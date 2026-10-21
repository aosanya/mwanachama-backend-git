package routes_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/routes"
)

func TestG16_NeighborhoodByPathStaysInsideTheCallersBranch(t *testing.T) {
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

	status, result := jsonDo(t, client, http.MethodGet,
		fmt.Sprintf("%s/branches/%s/neighborhood/secrets.txt?depth=1", srv.URL, attackerBranch), nil)
	if status != http.StatusNotFound {
		t.Fatalf("GetNeighborhood by a path only the victim's branch holds, from the attacker's branch: status %d, want 404, body %v", status, result)
	}

	status, own := jsonDo(t, client, http.MethodGet,
		fmt.Sprintf("%s/branches/%s/neighborhood/mine.txt?depth=1", srv.URL, attackerBranch), nil)
	if status != http.StatusOK {
		t.Fatalf("GetNeighborhood by the caller's own path: status %d, body %v", status, own)
	}
	if got := blobIDByPath(t, own, "mine.txt"); got == "" {
		t.Fatalf("GetNeighborhood by the caller's own path did not return it: %v", own)
	}
}
