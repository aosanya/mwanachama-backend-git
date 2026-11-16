package routes_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	mwanachamagit "github.com/aosanya/mwanachama-backend-git"
	"github.com/aosanya/mwanachama-backend-git/routes"
)

func newGitServer(t *testing.T) *httptest.Server {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	s, err := mwanachamagit.LoadSpec(filepath.Join("..", "spec", "examples", "engineering.git.json"))
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if err := mwanachamagit.Provision(db, s); err != nil {
		t.Fatalf("Provision: %v", err)
	}
	gm, err := mwanachamagit.NewGitManager(db, s, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewGitManager: %v", err)
	}
	mux := http.NewServeMux()
	for _, rt := range routes.Routes(gm) {
		mux.HandleFunc(rt.Pattern(""), rt.Handler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func send(t *testing.T, srv *httptest.Server, method, path, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
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

func TestTheRepositoryListAnswersEachLiveRepositoryWithItsMainBranchID(t *testing.T) {
	srv := newGitServer(t)
	send(t, srv, "POST", "/repos", `{"name":"widgets"}`)
	code, body := send(t, srv, "POST", "/repos", `{"name":"gadgets","default_branch":"trunk"}`)
	if code != http.StatusCreated {
		t.Fatalf("create: got %d: %s", code, body)
	}
	var gadgets map[string]any
	if err := json.Unmarshal(body, &gadgets); err != nil {
		t.Fatalf("decode: %v", err)
	}
	send(t, srv, "DELETE", "/repos/"+gadgets["id"].(string), "")

	code, body = send(t, srv, "GET", "/repositories", "")
	if code != http.StatusOK {
		t.Fatalf("list: got %d: %s", code, body)
	}
	var rows []map[string]string
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("decode list: %v: %s", err, body)
	}
	if len(rows) != 1 || rows[0]["name"] != "widgets" {
		t.Fatalf("want only widgets, got %s", body)
	}
	for _, key := range []string{"id", "name", "default_branch", "default_branch_id"} {
		if rows[0][key] == "" {
			t.Errorf("%s missing from %s", key, body)
		}
	}
	if code, body := send(t, srv, "GET", "/branches/"+rows[0]["default_branch_id"]+"/directory", ""); code != http.StatusOK {
		t.Errorf("default_branch_id must open the tree, got %d: %s", code, body)
	}
}
