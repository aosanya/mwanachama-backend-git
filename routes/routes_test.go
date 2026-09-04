package routes_test

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	mwanachamagit "github.com/aosanya/mwanachama-backend-git"
	"github.com/aosanya/mwanachama-backend-git/routes"
)

func newTestManager(t *testing.T) mwanachamagit.GitManager {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	tables := mwanachamagit.DefaultTableNames("test")
	if err := mwanachamagit.Migrate(db, tables); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	gm, err := mwanachamagit.NewGitManager(db, tables, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewGitManager: %v", err)
	}
	return gm
}

func patterns(rts []routes.Route, prefix string) []string {
	out := make([]string, len(rts))
	for i, rt := range rts {
		out[i] = rt.Pattern(prefix)
	}
	return out
}

func TestRoutes_TotalCount(t *testing.T) {
	gm := newTestManager(t)
	all := routes.Routes(gm)
	// 5 repos + 5 branches + 4 tags + 6 merge-requests/rollback + 6 files/log/diff
	// + 3 import + 6 keywords + 2 edges + 3 graph + 2 fetch-branch + 1 blob-search
	want := 5 + 5 + 4 + 6 + 6 + 3 + 6 + 2 + 3 + 2 + 1
	if len(all) != want {
		t.Fatalf("got %d routes, want %d: %v", len(all), want, patterns(all, ""))
	}
}

func TestRoutes_NoDuplicatePatterns(t *testing.T) {
	gm := newTestManager(t)
	seen := make(map[string]bool)
	for _, p := range patterns(routes.Routes(gm), "") {
		if seen[p] {
			t.Errorf("duplicate route pattern: %s", p)
		}
		seen[p] = true
	}
}

func TestRepositoryRoutes(t *testing.T) {
	gm := newTestManager(t)
	rts := routes.RepositoryRoutes(gm)
	assertPatterns(t, rts, []string{
		"POST /repos",
		"GET /repos",
		"GET /repos/{repoID}",
		"DELETE /repos/{repoID}",
		"POST /repos/{repoID}/purge",
	})
}

func TestBranchRoutes(t *testing.T) {
	gm := newTestManager(t)
	rts := routes.BranchRoutes(gm)
	assertPatterns(t, rts, []string{
		"POST /repos/{repoID}/branches",
		"GET /repos/{repoID}/branches",
		"GET /branches/{branchID}",
		"DELETE /branches/{branchID}",
		"POST /branches/{branchID}/merge",
	})
}

func TestKeywordRoutes_TreeBeforeWildcard(t *testing.T) {
	gm := newTestManager(t)
	rts := routes.KeywordRoutes(gm)
	// GetKeywordTree's static "/keywords/tree" must be registered before
	// GetKeyword's "/keywords/{keywordID}" wildcard.
	var treeIdx, wildcardIdx = -1, -1
	for i, rt := range rts {
		switch rt.Pattern("") {
		case "GET /keywords/tree":
			treeIdx = i
		case "GET /keywords/{keywordID}":
			wildcardIdx = i
		}
	}
	if treeIdx == -1 || wildcardIdx == -1 {
		t.Fatalf("expected both routes present: %v", patterns(rts, ""))
	}
	if treeIdx > wildcardIdx {
		t.Fatalf("expected /keywords/tree registered before /keywords/{keywordID}")
	}
}

func TestRoute_PatternWithPrefix(t *testing.T) {
	gm := newTestManager(t)
	rts := routes.RepositoryRoutes(gm)
	if got := rts[0].Pattern("/v1/git"); got != "POST /v1/git/repos" {
		t.Fatalf("got %q", got)
	}
}

func assertPatterns(t *testing.T, got []routes.Route, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d routes, want %d: %v", len(got), len(want), patterns(got, ""))
	}
	for i, p := range patterns(got, "") {
		if p != want[i] {
			t.Fatalf("route %d: got %q, want %q", i, p, want[i])
		}
	}
}
