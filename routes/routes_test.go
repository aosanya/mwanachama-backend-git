package routes_test

import (
	"path/filepath"
	"sort"
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

func patterns(rts []routes.Route, prefix string) []string {
	out := make([]string, len(rts))
	for i, rt := range rts {
		out[i] = rt.Pattern(prefix)
	}
	return out
}

// The forty-three addresses this package answered before the route table was
// declared. The conversion is only correct if the set is identical, so it is
// written out rather than counted.
var addresses = []string{
	"DELETE /branches/{branchID}",
	"DELETE /branches/{branchID}/files",
	"DELETE /edges",
	"DELETE /keywords/{keywordID}",
	"DELETE /repos/{repoID}",
	"DELETE /tags/{tagID}",
	"GET /branches/{branchID}",
	"GET /branches/{branchID}/directory",
	"GET /branches/{branchID}/files",
	"GET /branches/{branchID}/log",
	"GET /branches/{branchID}/neighborhood/{entityID}",
	"GET /diff",
	"GET /fetch-jobs/{jobID}",
	"GET /imports/{jobID}",
	"GET /keywords",
	"GET /keywords/tree",
	"GET /keywords/{keywordID}",
	"GET /merge-requests",
	"GET /merge-requests/{mrID}",
	"GET /repos",
	"GET /repos/{repoID}",
	"GET /repos/{repoID}/branches",
	"GET /repos/{repoID}/tags",
	"GET /tags/{tagID}",
	"POST /branches/{branchID}/fetch",
	"POST /branches/{branchID}/files",
	"POST /branches/{branchID}/merge",
	"POST /edges",
	"POST /graph/query",
	"POST /imports",
	"POST /imports/{jobID}/cancel",
	"POST /keywords",
	"POST /merge-requests",
	"POST /merge-requests/{mrID}/close",
	"POST /merge-requests/{mrID}/complete",
	"POST /repos",
	"POST /repos/{repoID}/branches",
	"POST /repos/{repoID}/purge",
	"POST /repos/{repoID}/tags",
	"POST /search/blobs",
	"POST /search/keywords",
	"POST /workflow-runs/{workflowRunID}/rollback",
	"PUT /keywords/{keywordID}",
}

func TestTheDeclaredTableAnswersExactlyTheAddressesItAlwaysDid(t *testing.T) {
	gm := newTestManager(t)
	got := patterns(routes.Routes(gm), "")
	sort.Strings(got)

	want := append([]string(nil), addresses...)
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("got %d addresses, want %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("address %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNoDuplicatePatterns(t *testing.T) {
	gm := newTestManager(t)
	seen := map[string]bool{}
	for _, p := range patterns(routes.Routes(gm), "") {
		if seen[p] {
			t.Errorf("duplicate route pattern: %s", p)
		}
		seen[p] = true
	}
}

func TestPatternTakesThePrefix(t *testing.T) {
	gm := newTestManager(t)
	for _, rt := range routes.Routes(gm) {
		if rt.Pattern("/v1/git") != rt.Method+" /v1/git"+rt.Path {
			t.Fatalf("prefix not applied to %s %s", rt.Method, rt.Path)
		}
	}
}

func TestEverySentinelTheSpecNeverMapsIsReported(t *testing.T) {
	unmapped, err := routes.Table.UnmappedSentinels()
	if err != nil {
		t.Fatalf("unmapped: %v", err)
	}
	if len(unmapped) > 0 {
		t.Errorf("these sentinels are supplied but mapped to no status, so each is redacted to a 500: %v", unmapped)
	}
}

func TestEveryAnonymousActionIsARealAction(t *testing.T) {
	unknown, err := routes.Table.UnknownAnonymousActions()
	if err != nil {
		t.Fatalf("unknown: %v", err)
	}
	if len(unknown) > 0 {
		t.Errorf("AnonymousActions names actions no operation declares: %v", unknown)
	}
}
