package routes_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/routes"
)

func serve(t *testing.T) *httptest.Server {
	t.Helper()
	gm := newTestManager(t)
	mux := http.NewServeMux()
	for _, rt := range routes.Routes(gm) {
		mux.HandleFunc(rt.Pattern(""), rt.Handler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func call(t *testing.T, srv *httptest.Server, method, path, body string) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, srv.URL+path, r)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func TestTheDeclaredTableServesTheStatusesItAlwaysDid(t *testing.T) {
	srv := serve(t)

	code, body := call(t, srv, "POST", "/repos", `{"name":"widgets","description":"a factory"}`)
	if code != http.StatusCreated {
		t.Fatalf("create repository: got %d, want 201; body %s", code, body)
	}
	var repo map[string]any
	if err := json.Unmarshal(body, &repo); err != nil {
		t.Fatalf("decode repository: %v", err)
	}
	repoID, _ := repo["id"].(string)
	if repoID == "" {
		t.Fatalf("a created repository must carry an id, got %s", body)
	}

	if code, body := call(t, srv, "POST", "/repos", `{"name":"widgets"}`); code != http.StatusConflict {
		t.Errorf("a duplicate name must be 409, got %d: %s", code, body)
	}
	if code, body := call(t, srv, "GET", "/repos/"+repoID, ""); code != http.StatusOK {
		t.Errorf("read repository: got %d: %s", code, body)
	}
	if code, _ := call(t, srv, "GET", "/repos/nope", ""); code != http.StatusNotFound {
		t.Errorf("an unknown repository must be 404, got %d", code)
	}

	code, body = call(t, srv, "GET", "/repos?name=widgets", "")
	if code != http.StatusOK {
		t.Fatalf("get by name: got %d: %s", code, body)
	}
	if !bytes.Contains(body, []byte(repoID)) {
		t.Errorf("get by name must answer the same repository, got %s", body)
	}

	code, body = call(t, srv, "GET", "/repos/"+repoID+"/branches", "")
	if code != http.StatusOK {
		t.Fatalf("list branches: got %d: %s", code, body)
	}
	var branches []map[string]any
	if err := json.Unmarshal(body, &branches); err != nil || len(branches) == 0 {
		t.Fatalf("a new repository has a default branch, got %s (%v)", body, err)
	}
	branchID, _ := branches[0]["id"].(string)

	code, body = call(t, srv, "POST", "/branches/"+branchID+"/files",
		`{"path":"README.md","content":"# widgets","message":"first","author_name":"amos"}`)
	if code != http.StatusCreated {
		t.Fatalf("write file: got %d, want 201: %s", code, body)
	}

	code, body = call(t, srv, "GET", "/branches/"+branchID+"/directory?path=", "")
	if code != http.StatusOK {
		t.Fatalf("list directory: got %d: %s", code, body)
	}
	var entries []map[string]any
	if err := json.Unmarshal(body, &entries); err != nil {
		t.Fatalf("decode directory: %v (%s)", err, body)
	}
	if len(entries) == 0 {
		t.Fatalf("the directory must list the file just written, got %s", body)
	}
	for _, key := range []string{"name", "path", "is_dir", "size"} {
		if _, ok := entries[0][key]; !ok {
			t.Errorf("a directory entry must carry %q in snake case, got %s", key, body)
		}
	}

	code, body = call(t, srv, "GET", "/branches/"+branchID+"/log", "")
	if code != http.StatusOK {
		t.Fatalf("log: got %d: %s", code, body)
	}
	var commits []map[string]any
	if err := json.Unmarshal(body, &commits); err != nil {
		t.Fatalf("decode log: %v (%s)", err, body)
	}
	if len(commits) == 0 {
		t.Fatalf("the log must carry the commit just written, got %s", body)
	}
	for _, key := range []string{"sha", "author", "message", "timestamp"} {
		if _, ok := commits[0][key]; !ok {
			t.Errorf("a log entry must carry %q, got %s", key, body)
		}
	}
	if ts, _ := commits[0]["timestamp"].(string); strings.Contains(ts, ".") {
		t.Errorf("a log timestamp is RFC3339 without fractional seconds, got %q", ts)
	}

	if code, body := call(t, srv, "DELETE", "/repos/"+repoID, ""); code != http.StatusNoContent {
		t.Errorf("delete repository: got %d, want 204: %s", code, body)
	}
}

func TestAnUndeclaredRelationshipIsFourHundred(t *testing.T) {
	srv := serve(t)
	code, body := call(t, srv, "POST", "/edges",
		`{"branch_id":"b","from_id":"x","to_id":"y","relationship_name":"supersedes"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("an undeclared relationship must be 400, got %d: %s", code, body)
	}
}

func TestAKeywordTreeIsNotSwallowedByTheWildcard(t *testing.T) {
	srv := serve(t)
	if code, body := call(t, srv, "GET", "/keywords/tree", ""); code != http.StatusOK {
		t.Fatalf("GET /keywords/tree must reach the tree, not the {keywordID} wildcard; got %d: %s", code, body)
	}
}
