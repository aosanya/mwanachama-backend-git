package routes_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/routes"
)

// TestG16_NeighborhoodByPathLeaksBlobsAcrossRepositories pins a known defect
// (board row G16, todo.md) in the same graph-query family as G15, same file
// (git_impl_graph.go), one method over: GetNeighborhood confirms its
// branch_id path parameter exists (git_impl_graph.go's GetNeighborhood,
// ~line 44) but resolveEntityID (~line 123) resolves a non-id entityID by a
// bare, GLOBAL, unscoped `path = ?` lookup against every Blob in the
// deployment — not just blobs reachable from that branch. Worse than G15:
// an attacker needs no prior knowledge at all, not even a discoverable
// keyword id — a guessed common filename ("secrets.txt", ".env",
// "README.md") is enough.
//
// Driven through the real aggregator + http.ServeMux + http.Client. Setup:
// repo A ("victim") gets a file "secrets.txt" with content
// "REPO-A-SECRET-CONTENT"; repo B ("attacker", no relationship to A) gets
// its own unrelated file. The attacker calls
// GET /branches/{their own branch}/neighborhood/secrets.txt and receives
// the victim's blob content verbatim, real observed response:
//
//	nodes:[map[... path:secrets.txt content:REPO-A-SECRET-CONTENT ...] ...]
//
// Fix: resolveEntityID's path fallback must scope the Blob lookup to blobs
// reachable from the given branch (the same join GetNeighborhood's sibling
// handlers ReadFile/ListDirectory/Log already use, per this repo's own
// CLAUDE.md) rather than querying git_tables.Blobs globally by path alone.
// This test's "leaked content is present" assertion must invert into a
// refusal/not-found once that lands.
func TestG16_NeighborhoodByPathLeaksBlobsAcrossRepositories(t *testing.T) {
	gm := newTestManager(t)
	mux := http.NewServeMux()
	for _, rt := range routes.Routes(gm) {
		mux.Handle(rt.Pattern(""), rt.Handler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := srv.Client()

	_, victimBranch := createRepoAndDefaultBranch(t, client, srv.URL, "g16-victim-repo")
	writeProbeFile(t, client, srv.URL, victimBranch, "secrets.txt", "REPO-A-SECRET-CONTENT")

	_, attackerBranch := createRepoAndDefaultBranch(t, client, srv.URL, "g16-attacker-repo")
	writeProbeFile(t, client, srv.URL, attackerBranch, "mine.txt", "just my own file")

	// The attacker scopes the call to THEIR OWN branch and guesses a common
	// filename — never learns the victim's branch id, repo id, or blob id.
	status, result := jsonDo(t, client, http.MethodGet,
		fmt.Sprintf("%s/branches/%s/neighborhood/secrets.txt?depth=1", srv.URL, attackerBranch), nil)
	if status != http.StatusOK {
		t.Fatalf("GetNeighborhood: status %d, body %v", status, result)
	}

	sawVictimSecret := false
	nodes, _ := result.(map[string]any)["nodes"].([]any)
	for _, n := range nodes {
		node := n.(map[string]any)
		props, _ := node["properties"].(map[string]any)
		if props["path"] == "secrets.txt" && props["content"] == "REPO-A-SECRET-CONTENT" {
			sawVictimSecret = true
		}
	}
	if !sawVictimSecret {
		t.Fatalf("G16 regression: GetNeighborhood scoped to the attacker's own branch (%s), resolving a guessed path, no longer returns the victim repository's secrets.txt content (nodes seen: %v) — this pin needs inverting into a real branch-scoping assertion, see board row G16", attackerBranch, nodes)
	}
}
