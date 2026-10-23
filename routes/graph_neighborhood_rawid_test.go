package routes_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/routes"
)

func neighborhoodNodeIDs(t *testing.T, result any) []string {
	t.Helper()
	nodes, _ := result.(map[string]any)["nodes"].([]any)
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.(map[string]any)["id"].(string))
	}
	return ids
}

func TestG22_NeighborhoodByRawIDStaysInsideTheCallersBranchHead(t *testing.T) {
	gm := newTestManager(t)
	mux := http.NewServeMux()
	for _, rt := range routes.Routes(gm) {
		mux.Handle(rt.Pattern(""), rt.Handler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := srv.Client()

	_, victimBranch := createRepoAndDefaultBranch(t, client, srv.URL, "g22-victim-repo")
	writeProbeFile(t, client, srv.URL, victimBranch, "secrets.txt", "REPO-A-SECRET-CONTENT")

	_, attackerBranch := createRepoAndDefaultBranch(t, client, srv.URL, "g22-attacker-repo")
	writeProbeFile(t, client, srv.URL, attackerBranch, "mine.txt", "first version")

	status, victimView := jsonDo(t, client, http.MethodGet,
		fmt.Sprintf("%s/branches/%s/neighborhood/secrets.txt?depth=3", srv.URL, victimBranch), nil)
	if status != http.StatusOK {
		t.Fatalf("victim's own neighbourhood: status %d, body %v", status, victimView)
	}
	victimIDs := neighborhoodNodeIDs(t, victimView)
	if len(victimIDs) < 3 {
		t.Fatalf("victim's neighbourhood should hold the blob, its tree and its commit: %v", victimView)
	}
	for _, id := range victimIDs {
		status, result := jsonDo(t, client, http.MethodGet,
			fmt.Sprintf("%s/branches/%s/neighborhood/%s?depth=3", srv.URL, attackerBranch, id), nil)
		if status != http.StatusNotFound {
			t.Fatalf("GetNeighborhood by the victim's raw id %s from the attacker's branch: status %d, want 404, body %v", id, status, result)
		}
	}

	status, before := jsonDo(t, client, http.MethodGet,
		fmt.Sprintf("%s/branches/%s/neighborhood/mine.txt?depth=3", srv.URL, attackerBranch), nil)
	if status != http.StatusOK {
		t.Fatalf("caller's own path: status %d, body %v", status, before)
	}
	oldBlob := blobIDByPath(t, before, "mine.txt")
	for _, id := range neighborhoodNodeIDs(t, before) {
		for _, victimID := range victimIDs {
			if id == victimID {
				t.Fatalf("the caller's own walk stepped into the victim's repository at %s: %v", id, before)
			}
		}
	}

	writeProbeFile(t, client, srv.URL, attackerBranch, "mine.txt", "second version")

	status, result := jsonDo(t, client, http.MethodGet,
		fmt.Sprintf("%s/branches/%s/neighborhood/%s?depth=1", srv.URL, attackerBranch, oldBlob), nil)
	if status != http.StatusNotFound {
		t.Fatalf("GetNeighborhood by a blob id no longer in the head's tree: status %d, want 404, body %v", status, result)
	}

	status, after := jsonDo(t, client, http.MethodGet,
		fmt.Sprintf("%s/branches/%s/neighborhood/mine.txt?depth=1", srv.URL, attackerBranch), nil)
	if status != http.StatusOK {
		t.Fatalf("caller's own path after the edit: status %d, body %v", status, after)
	}
	newBlob := blobIDByPath(t, after, "mine.txt")
	status, own := jsonDo(t, client, http.MethodGet,
		fmt.Sprintf("%s/branches/%s/neighborhood/%s?depth=1", srv.URL, attackerBranch, newBlob), nil)
	if status != http.StatusOK {
		t.Fatalf("caller's own raw blob id at head: status %d, body %v", status, own)
	}
}
