package routes_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/routes"
)

// TestG17_NeighborhoodByRealIDLeaksBlobsAcrossRepositories pins a known
// defect (board row G17, todo.md), a second and more direct hole in the
// same method G16 already flags: GetNeighborhood's only real scoping is
// that branch_id must name an EXISTING branch (git_impl_graph.go, ~line
// 44) — the traversal itself is never checked against that branch.
//
// G16's own filed fix ("resolveEntityID's path fallback must scope the
// Blob lookup") only touches the entityID-as-path branch of
// resolveEntityID. This pins the OTHER branch: when entityID is already a
// real row id (gormstore.ResolveNodeType, ~queries.go:136, checks Repository/
// Branch/MergeRequest/Tag/Commit/Tree/Blob/Keyword tables globally, with no
// repo or branch predicate at all), resolveEntityID returns it unchanged
// and traverseNeighborhood (gormstore.NeighborhoodEdges) then walks from it
// with the same lack of scoping — so a caller who has learned ANY real id
// from an unrelated repository (via G15's own leak, a log line, a webhook
// payload, or any other channel) can pull that repository's full subgraph,
// including Blob content, through their OWN unrelated branch. G16's stated
// fix would NOT close this: this path never reaches the Blob-by-path
// lookup at all.
//
// Driven through the real aggregator + http.ServeMux + http.Client, same
// shape as G15/G16's own tests. Setup: repo A ("victim") writes
// "id-secrets.txt" = "REPO-A-ID-SECRET"; its real blob id is read back via
// a QueryGraph call scoped to the victim's OWN branch (standing in for
// however an attacker actually learned that id — G15's own leak is one
// such channel already filed on this board). Repo B ("attacker", no
// relationship to A) then calls GetNeighborhood scoped to ITS OWN branch,
// passing the victim's real blob id directly — no path guess, no branch
// or repo id belonging to the victim. Real observed response:
//
//	nodes:[... map[content:REPO-A-ID-SECRET ... path:id-secrets.txt] ...]
//
// Fix: both gormstore.ResolveNodeType and gormstore.NeighborhoodEdges (or
// traverseNeighborhood's caller) need the given branch's reachable-node set
// as a filter, not just a branch-exists check up front — the same
// reachability join ReadFile/ListDirectory/Log already use, per this
// repo's own CLAUDE.md. This test's "leaked content is present" assertion
// must invert into a refusal/not-found once that lands.
func TestG17_NeighborhoodByRealIDLeaksBlobsAcrossRepositories(t *testing.T) {
	gm := newTestManager(t)
	mux := http.NewServeMux()
	for _, rt := range routes.Routes(gm) {
		mux.Handle(rt.Pattern(""), rt.Handler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := srv.Client()

	_, victimBranch := createRepoAndDefaultBranch(t, client, srv.URL, "g17-victim-repo")
	writeProbeFile(t, client, srv.URL, victimBranch, "id-secrets.txt", "REPO-A-ID-SECRET")

	status, victimGraph := jsonDo(t, client, http.MethodPost, srv.URL+"/graph/query", map[string]any{
		"branch_id": victimBranch, "limit": 10,
	})
	if status != http.StatusOK {
		t.Fatalf("QueryGraph(victim): status %d body %v", status, victimGraph)
	}
	victimBlobID := blobIDByPath(t, victimGraph, "id-secrets.txt")

	_, attackerBranch := createRepoAndDefaultBranch(t, client, srv.URL, "g17-attacker-repo")
	writeProbeFile(t, client, srv.URL, attackerBranch, "mine.txt", "just my own file")

	// The attacker scopes the call to THEIR OWN branch and supplies the
	// victim's real blob id directly — never a guessed path, never a
	// branch or repo id belonging to the victim.
	status, result := jsonDo(t, client, http.MethodGet,
		fmt.Sprintf("%s/branches/%s/neighborhood/%s?depth=1", srv.URL, attackerBranch, victimBlobID), nil)
	if status != http.StatusOK {
		t.Fatalf("GetNeighborhood: status %d, body %v", status, result)
	}

	sawVictimSecret := false
	nodes, _ := result.(map[string]any)["nodes"].([]any)
	for _, n := range nodes {
		node := n.(map[string]any)
		props, _ := node["properties"].(map[string]any)
		if props["path"] == "id-secrets.txt" && props["content"] == "REPO-A-ID-SECRET" {
			sawVictimSecret = true
		}
	}
	if !sawVictimSecret {
		t.Fatalf("G17 regression: GetNeighborhood scoped to the attacker's own branch (%s), given the victim's real blob id directly, no longer returns the victim repository's secret content (nodes seen: %v) — this pin needs inverting into a real branch-reachability assertion, see board row G17", attackerBranch, nodes)
	}
}
