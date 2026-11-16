package routes_test

import (
	"context"
	"encoding/json"
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

func decodeObject(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode object: %v: %s", err, raw)
	}
	return out
}

func decodeList(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode list: %v: %s", err, raw)
	}
	return out
}

func newGitManager(t *testing.T) mwanachamagit.GitManager {
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
	return gm
}

func serveRoutes(t *testing.T, rs []routes.Route) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for _, rt := range rs {
		mux.HandleFunc(rt.Pattern(""), rt.Handler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func createRepo(t *testing.T, srv *httptest.Server, name string) map[string]any {
	t.Helper()
	code, body := send(t, srv, "POST", "/repos", `{"name":"`+name+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("create %s: %d %s", name, code, body)
	}
	return decodeObject(t, body)
}

func TestListingRepositoriesWithoutANameReturnsEveryOne(t *testing.T) {
	srv := newGitServer(t)
	createRepo(t, srv, "alpha")
	createRepo(t, srv, "beta")
	code, body := send(t, srv, "GET", "/repos", "")
	if code != http.StatusOK {
		t.Fatalf("got %d: %s", code, body)
	}
	names := map[string]bool{}
	for _, r := range decodeList(t, body) {
		names[r["name"].(string)] = true
	}
	if len(names) != 2 || !names["alpha"] || !names["beta"] {
		t.Fatalf("want alpha and beta, got %s", body)
	}
}

func TestAskingForARepositoryByNameReturnsOnlyThatOne(t *testing.T) {
	srv := newGitServer(t)
	createRepo(t, srv, "alpha")
	beta := createRepo(t, srv, "beta")
	code, body := send(t, srv, "GET", "/repos?name=beta", "")
	if code != http.StatusOK {
		t.Fatalf("got %d: %s", code, body)
	}
	got := decodeObject(t, body)
	if got["id"] != beta["id"] || got["name"] != "beta" {
		t.Fatalf("want beta, got %s", body)
	}
}

func TestAskingForAnUnknownRepositoryNameIsNotFound(t *testing.T) {
	srv := newGitServer(t)
	createRepo(t, srv, "alpha")
	code, body := send(t, srv, "GET", "/repos?name=ghost", "")
	if code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", code, body)
	}
	if !strings.Contains(string(body), "error") {
		t.Fatalf("want an error body, got %s", body)
	}
}

func createBranch(t *testing.T, srv *httptest.Server, repoID, name string) map[string]any {
	t.Helper()
	code, body := send(t, srv, "POST", "/repos/"+repoID+"/branches", `{"name":"`+name+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("create branch %s: %d %s", name, code, body)
	}
	return decodeObject(t, body)
}

func TestListingBranchesReturnsTheDefaultAndEveryCreatedBranch(t *testing.T) {
	srv := newGitServer(t)
	repo := createRepo(t, srv, "alpha")
	createBranch(t, srv, repo["id"].(string), "feature-a")
	code, body := send(t, srv, "GET", "/repos/"+repo["id"].(string)+"/branches", "")
	if code != http.StatusOK {
		t.Fatalf("got %d: %s", code, body)
	}
	names := map[string]bool{}
	for _, b := range decodeList(t, body) {
		names[b["name"].(string)] = true
	}
	if !names["feature-a"] || len(names) != 2 {
		t.Fatalf("want default plus feature-a, got %s", body)
	}
}

func TestAskingForABranchByNameReturnsOnlyThatBranch(t *testing.T) {
	srv := newGitServer(t)
	repo := createRepo(t, srv, "alpha")
	feature := createBranch(t, srv, repo["id"].(string), "feature-a")
	createBranch(t, srv, repo["id"].(string), "feature-b")
	code, body := send(t, srv, "GET", "/repos/"+repo["id"].(string)+"/branches?name=feature-a", "")
	if code != http.StatusOK {
		t.Fatalf("got %d: %s", code, body)
	}
	if got := decodeObject(t, body); got["id"] != feature["id"] {
		t.Fatalf("want feature-a, got %s", body)
	}
}

func TestAskingForAnUnknownBranchNameIsNotFound(t *testing.T) {
	srv := newGitServer(t)
	repo := createRepo(t, srv, "alpha")
	code, body := send(t, srv, "GET", "/repos/"+repo["id"].(string)+"/branches?name=ghost", "")
	if code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", code, body)
	}
}

func TestBranchNameLookupWinsOverTheWorkflowRunFilter(t *testing.T) {
	srv := newGitServer(t)
	repo := createRepo(t, srv, "alpha")
	feature := createBranch(t, srv, repo["id"].(string), "feature-a")
	code, body := send(t, srv, "GET", "/repos/"+repo["id"].(string)+"/branches?name=feature-a&workflow_run_id=nope", "")
	if code != http.StatusOK {
		t.Fatalf("got %d: %s", code, body)
	}
	if got := decodeObject(t, body); got["id"] != feature["id"] {
		t.Fatalf("want feature-a, got %s", body)
	}
}

func TestFilteringBranchesByAWorkflowRunWithNoBranchesIsEmpty(t *testing.T) {
	srv := newGitServer(t)
	repo := createRepo(t, srv, "alpha")
	createBranch(t, srv, repo["id"].(string), "feature-a")
	code, body := send(t, srv, "GET", "/repos/"+repo["id"].(string)+"/branches?workflow_run_id=run-none", "")
	if code != http.StatusOK {
		t.Fatalf("got %d: %s", code, body)
	}
	if rows := decodeList(t, body); len(rows) != 0 {
		t.Fatalf("want no branches for an unknown run, got %s", body)
	}
}

func TestMergingABranchWithNoCommitsAnswersTheDefaultBranch(t *testing.T) {
	srv := newGitServer(t)
	repo := createRepo(t, srv, "alpha")
	feature := createBranch(t, srv, repo["id"].(string), "feature-a")
	code, body := send(t, srv, "POST", "/branches/"+feature["id"].(string)+"/merge", "")
	if code != http.StatusOK {
		t.Fatalf("got %d: %s", code, body)
	}
	got := decodeObject(t, body)
	if got["id"] == feature["id"] || got["repository_id"] != repo["id"] {
		t.Fatalf("want the repository's default branch, got %s", body)
	}
}

func TestMergingAnUnknownBranchIsNotFound(t *testing.T) {
	srv := newGitServer(t)
	code, body := send(t, srv, "POST", "/branches/no-such-branch/merge", "")
	if code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", code, body)
	}
}

func TestEveryRouteBuildReturnsDeclaredAndHandWrittenAddresses(t *testing.T) {
	gm := newGitManager(t)
	built, err := routes.Build(gm)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	seen := map[string]bool{}
	for _, rt := range built {
		seen[rt.Method+" "+rt.Path] = true
	}
	for _, want := range []string{"GET /repos", "GET /repos/{repoID}/branches", "POST /branches/{branchID}/merge", "POST /repos", "GET /repositories"} {
		if !seen[want] {
			t.Errorf("missing %s", want)
		}
	}
	if len(built) != len(routes.Routes(gm)) {
		t.Errorf("Build and Routes disagree: %d vs %d", len(built), len(routes.Routes(gm)))
	}
}

func TestSplitPutsTheHandWrittenAddressesBehindTheGateAndNothingIsPublic(t *testing.T) {
	m := newGitManager(t)
	split := routes.Split(m, routes.Mount{})
	if len(split.Anonymous) != 0 || len(routes.PublicRoutes(m)) != 0 {
		t.Fatalf("nothing may be anonymous, got %d", len(split.Anonymous))
	}
	gated := map[string]bool{}
	for _, rt := range split.Gated {
		gated[rt.Method+" "+rt.Path] = true
	}
	for _, want := range []string{"GET /repos", "GET /repos/{repoID}/branches", "POST /branches/{branchID}/merge"} {
		if !gated[want] {
			t.Errorf("%s must be gated", want)
		}
	}
	if len(routes.OperatorRoutes(m, routes.Mount{})) != len(split.Gated) {
		t.Errorf("OperatorRoutes must be the gated half")
	}
}

func TestADeniedActionRefusesTheDeclaredRoutesWith403(t *testing.T) {
	gm := newGitManager(t)
	deny := func(ctx context.Context, action string) error {
		return context.Canceled
	}
	srv := serveRoutes(t, routes.RoutesFor(gm, routes.Mount{Authorize: deny}))
	code, body := send(t, srv, "POST", "/repos", `{"name":"alpha"}`)
	if code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", code, body)
	}
	if code, body := send(t, srv, "GET", "/repositories", ""); code != http.StatusForbidden {
		t.Fatalf("list must be refused too, got %d: %s", code, body)
	}
}
