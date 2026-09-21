package routes_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/routes"
)

// jsonDo drives the real http.ServeMux built from routes.Routes over a real
// http.Client — never the http.HandlerFunc directly — so these tests catch
// what only an actual caller (wrong route registered, wrong status code,
// wrong response shape) would see.
func jsonDo(t *testing.T, client *http.Client, method, url string, body any) (int, any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func createRepoAndDefaultBranch(t *testing.T, client *http.Client, baseURL, name string) (repoID, branchID string) {
	t.Helper()
	_, repo := jsonDo(t, client, http.MethodPost, baseURL+"/repos", map[string]any{"name": name})
	repoID = repo.(map[string]any)["id"].(string)
	_, branches := jsonDo(t, client, http.MethodGet, baseURL+"/repos/"+repoID+"/branches", nil)
	branchID = branches.([]any)[0].(map[string]any)["id"].(string)
	return repoID, branchID
}

func writeProbeFile(t *testing.T, client *http.Client, baseURL, branchID, path, content string) string {
	t.Helper()
	status, out := jsonDo(t, client, http.MethodPost, baseURL+"/branches/"+branchID+"/files", map[string]any{
		"branch_id": branchID, "path": path, "content": content, "author_name": "test-actor", "message": "add " + path,
	})
	if status != http.StatusCreated {
		t.Fatalf("WriteFile(%s): status %d, body %v", path, status, out)
	}
	return branchID
}

// blobIDByPath scans a QueryGraph/SearchByKeywords-shaped {"nodes": [...]}
// response for the node whose properties.path matches, and fails the test
// if it isn't found rather than returning a zero value silently.
func blobIDByPath(t *testing.T, result any, path string) string {
	t.Helper()
	nodes, _ := result.(map[string]any)["nodes"].([]any)
	for _, n := range nodes {
		node := n.(map[string]any)
		props, _ := node["properties"].(map[string]any)
		if props["path"] == path {
			return node["id"].(string)
		}
	}
	t.Fatalf("no node with path %q in result %v", path, result)
	return ""
}

func nodePaths(result any) []string {
	nodes, _ := result.(map[string]any)["nodes"].([]any)
	var paths []string
	for _, n := range nodes {
		node := n.(map[string]any)
		props, _ := node["properties"].(map[string]any)
		if p, ok := props["path"].(string); ok {
			paths = append(paths, p)
		}
	}
	return paths
}

// G15_QueryGraphLeaksBlobsAcrossRepositories pins a known defect (board row
// G15, todo.md): POST /graph/query only checks that the given branch_id
// exists (git_impl_graph.go's QueryGraph, ~line 455); it never filters the
// actual Blob query by that branch or its repository. Driven through the
// real aggregator + http.ServeMux + http.Client (not the manager directly):
// an attacker who owns nothing but their own tiny repository can see every
// Blob's full content anywhere in the deployment, including a completely
// unrelated repository's private file, just by calling this route scoped to
// their own branch. A fixed QueryGraph must restrict results to Blobs
// reachable from the given branch's own repository/history, and this test's
// "leaked path is present" assertion must invert once that lands.
func TestG15_QueryGraphLeaksBlobsAcrossRepositories(t *testing.T) {
	gm := newTestManager(t)
	mux := http.NewServeMux()
	for _, rt := range routes.Routes(gm) {
		mux.Handle(rt.Pattern(""), rt.Handler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := srv.Client()

	_, victimBranch := createRepoAndDefaultBranch(t, client, srv.URL, "g15-victim-repo")
	writeProbeFile(t, client, srv.URL, victimBranch, "secrets.txt", "REPO-A-SECRET-CONTENT")

	_, attackerBranch := createRepoAndDefaultBranch(t, client, srv.URL, "g15-attacker-repo")
	writeProbeFile(t, client, srv.URL, attackerBranch, "mine.txt", "just my own file")

	status, result := jsonDo(t, client, http.MethodPost, srv.URL+"/graph/query", map[string]any{"branch_id": attackerBranch})
	if status != http.StatusOK {
		t.Fatalf("QueryGraph: status %d, body %v", status, result)
	}

	paths := nodePaths(result)
	sawVictimSecret := false
	for _, p := range paths {
		if p == "secrets.txt" {
			sawVictimSecret = true
		}
	}
	if !sawVictimSecret {
		t.Fatalf("G15 regression: QueryGraph scoped to the attacker's own branch (%s) no longer returns the victim repository's secrets.txt (paths seen: %v) — this pin needs inverting into a real branch-scoping assertion, see board row G15", attackerBranch, paths)
	}
}

// G15_SearchByKeywordsLeaksBlobsAcrossRepositories widens G15's finding to
// the sibling handler in the same file (git_impl_graph.go's
// SearchByKeywords, ~line 294): its BranchID field carries the identical
// doc-comment promise ("Only entities reachable from this branch are
// considered") and the identical bug — taggedBlobsForKeyword queries
// git_blob_keyword_tags by keyword id alone, with no branch/repository
// filter at all. An attacker who merely knows a keyword id (keywords are a
// flat, ungated GET /keywords list) and their own branch id can retrieve a
// blob from a repository they have no relationship to.
func TestG15_SearchByKeywordsLeaksBlobsAcrossRepositories(t *testing.T) {
	gm := newTestManager(t)
	mux := http.NewServeMux()
	for _, rt := range routes.Routes(gm) {
		mux.Handle(rt.Pattern(""), rt.Handler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := srv.Client()

	_, victimBranch := createRepoAndDefaultBranch(t, client, srv.URL, "g15-kw-victim-repo")
	writeProbeFile(t, client, srv.URL, victimBranch, "secrets.txt", "REPO-A-SECRET-CONTENT")
	_, queryResult := jsonDo(t, client, http.MethodPost, srv.URL+"/graph/query", map[string]any{"branch_id": victimBranch})
	secretBlobID := blobIDByPath(t, queryResult, "secrets.txt")

	_, attackerBranch := createRepoAndDefaultBranch(t, client, srv.URL, "g15-kw-attacker-repo")

	status, kw := jsonDo(t, client, http.MethodPost, srv.URL+"/keywords", map[string]any{"name": "g15-victim-only-tag", "scope": "test"})
	if status != http.StatusCreated {
		t.Fatalf("CreateKeyword: status %d, body %v", status, kw)
	}
	kwID := kw.(map[string]any)["id"].(string)

	status, edgeOut := jsonDo(t, client, http.MethodPost, srv.URL+"/edges", map[string]any{
		"branch_id": victimBranch, "from_entity_id": secretBlobID,
		"relationship_name": "tagged_with", "to_entity_id": kwID,
	})
	if status != http.StatusCreated {
		t.Fatalf("CreateEdge(tagged_with): status %d, body %v", status, edgeOut)
	}

	status, searchResult := jsonDo(t, client, http.MethodPost, srv.URL+"/search/keywords", map[string]any{
		"branch_id": attackerBranch, "keywords": []string{kwID},
	})
	if status != http.StatusOK {
		t.Fatalf("SearchByKeywords: status %d, body %v", status, searchResult)
	}

	paths := nodePaths(searchResult)
	sawVictimSecret := false
	for _, p := range paths {
		if p == "secrets.txt" {
			sawVictimSecret = true
		}
	}
	if !sawVictimSecret {
		t.Fatalf("G15 regression: SearchByKeywords scoped to the attacker's own branch (%s) no longer returns the victim repository's secrets.txt (paths seen: %v) — this pin needs inverting once branch scoping is fixed, see board row G15", attackerBranch, paths)
	}
}
