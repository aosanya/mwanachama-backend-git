//go:build integration

// postgres_integration_test.go exercises GitManager against a real Postgres
// database, rather than the in-memory sqlite-backed manager the rest of this
// package's tests use.
//
// Skipped unless POSTGRES_URL is set. The unit tests elsewhere in this
// package already exhaustively cover GitManager's business logic; this
// file's job is narrower — prove the real Postgres wiring (GORM AutoMigrate,
// the blob full-text-search GIN index) works end-to-end.
package mwanachamagit

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/postgres"
)

// uniqueRepoName returns a repository name namespaced to the running test,
// so two tests in this file don't collide over InitRepo's global repository
// uniqueness check when both run against the same POSTGRES_URL database in
// one `go test -tags=integration` invocation.
func uniqueRepoName(t *testing.T, base string) string {
	t.Helper()
	return fmt.Sprintf("%s-%s-%d", base, t.Name(), time.Now().UnixNano())
}

// newPostgresGitManager opens POSTGRES_URL via
// mwanachama-backend-shared/postgres.Open (the same DSN parsing, pgx driver,
// and pooling every other repo already uses), wraps that connection with
// GORM's Postgres dialector, migrates a unique-enough table prefix, and
// returns a ready-to-use *gitManager. Skips the calling test if POSTGRES_URL
// is unset. Tables are dropped on cleanup.
func newPostgresGitManager(t *testing.T, searcher BlobSearcher) (*gitManager, TableNames) {
	t.Helper()
	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		t.Skip("POSTGRES_URL not set; skipping Postgres integration test")
	}

	ctx := context.Background()
	sqlDB, err := postgres.Open(ctx, postgres.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("postgres.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	db, err := gorm.Open(gormpostgres.New(gormpostgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}

	tables := DefaultTableNames("gitit")
	if err := Migrate(db, tables); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Migrator().DropTable(
			tables.BlobKeywordTags, tables.BlobReferences, tables.CommitParents,
			tables.TreeBlobs, tables.TreeSubtrees, tables.ImportJobs, tables.FetchBranchJobs,
			tables.Blobs, tables.Trees, tables.Commits, tables.Tags, tables.MergeRequests,
			tables.Branches, tables.Repositories, tables.Agencies,
		)
	})

	m := &gitManager{db: db, tables: tables, locker: &mutexLocker{}, searcher: searcher}
	return m, tables
}

// TestPostgresGitManagerRoundTrip exercises InitRepo -> CreateBranch ->
// WriteFile -> ReadFile -> Log -> MergeBranch against a real Postgres-backed
// GitManager — the port-fidelity signal: the same business logic the
// sqlite-backed unit tests exercise, now against real Postgres.
func TestPostgresGitManagerRoundTrip(t *testing.T) {
	m, _ := newPostgresGitManager(t, nil)
	ctx := context.Background()

	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: uniqueRepoName(t, "widgets")})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, err := m.ListBranches(ctx, repo.ID)
	if err != nil || len(branches) != 1 {
		t.Fatalf("ListBranches: %+v, err=%v", branches, err)
	}
	branch := branches[0]

	commit1, err := m.WriteFile(ctx, WriteFileRequest{
		BranchID: branch.ID,
		Path:     "README.md",
		Content:  "# widgets\n\nA widget factory.",
	})
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	blob, err := m.ReadFile(ctx, branch.ID, "README.md")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if blob.Content != "# widgets\n\nA widget factory." {
		t.Fatalf("unexpected content: %q", blob.Content)
	}

	feature, err := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "feature/x"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if _, err := m.WriteFile(ctx, WriteFileRequest{
		BranchID: feature.ID,
		Path:     "src/main.go",
		Content:  "package main",
	}); err != nil {
		t.Fatalf("WriteFile on feature branch: %v", err)
	}

	merged, err := m.MergeBranch(ctx, feature.ID)
	if err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	if _, err := m.ReadFile(ctx, merged.ID, "src/main.go"); err != nil {
		t.Fatalf("ReadFile on default branch after merge: %v", err)
	}

	history, err := m.Log(ctx, merged.ID, LogFilter{})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(history) < 1 || history[0].SHA == "" {
		t.Fatalf("expected at least one commit with a SHA in history, got %+v", history)
	}

	if commit1.SHA == "" {
		t.Fatalf("expected commit1 to have a real SHA")
	}
}

// TestPostgresBlobSearch exercises PostgresBlobSearcher against real
// Postgres full-text search: relevance ranking and the
// no-match-returns-empty-not-nil contract.
func TestPostgresBlobSearch(t *testing.T) {
	m, tables := newPostgresGitManager(t, nil)
	ctx := context.Background()

	mustCreateBlob := func(path, name, content string) {
		t.Helper()
		if err := m.db.WithContext(ctx).Table(tables.Blobs).Create(map[string]any{
			"id": mustUUID(), "path": path, "name": name, "content": content,
			"created_at": "2026-01-01T00:00:00.000000000Z",
		}).Error; err != nil {
			t.Fatalf("create blob %s: %v", path, err)
		}
	}

	mustCreateBlob("auth/oauth.go", "oauth.go", "package auth\n\n// OAuth token exchange for the login flow.")
	mustCreateBlob("auth/session.go", "session.go", "package auth\n\n// Session cookie handling, unrelated to tokens.")
	mustCreateBlob("README.md", "README.md", "# widgets\n\nA widget factory with no authentication concerns.")

	searcher := NewPostgresBlobSearcher(m.db, tables)

	results, err := searcher.Search(ctx, "oauth token", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected at least one match for 'oauth token'")
	}
	if results[0].Path != "auth/oauth.go" {
		t.Fatalf("expected oauth.go to rank first, got %+v", results)
	}
	for _, r := range results {
		if r.Path == "auth/oauth.go" && r.Snippet == "" {
			t.Fatalf("expected a non-empty snippet for the top match, got %+v", r)
		}
		if r.ID == "" {
			t.Fatalf("result missing ID: %+v", r)
		}
	}

	noMatch, err := searcher.Search(ctx, "xyzzy-nonexistent-term", 10)
	if err != nil {
		t.Fatalf("Search (no match): %v", err)
	}
	if noMatch == nil {
		t.Fatalf("expected empty slice, not nil, for no matches")
	}
	if len(noMatch) != 0 {
		t.Fatalf("expected no matches, got %+v", noMatch)
	}
}

// TestPostgresBlobSearchViaGitManager exercises GitManager.SearchBlobs end to
// end with a real PostgresBlobSearcher injected.
func TestPostgresBlobSearchViaGitManager(t *testing.T) {
	m, tables := newPostgresGitManager(t, nil)
	m.searcher = NewPostgresBlobSearcher(m.db, tables)
	ctx := context.Background()

	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: uniqueRepoName(t, "widgets")})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, _ := m.ListBranches(ctx, repo.ID)
	if _, err := m.WriteFile(ctx, WriteFileRequest{
		BranchID: branches[0].ID,
		Path:     "docs/auth.md",
		Content:  "OAuth login flow documentation.",
	}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	results, err := m.SearchBlobs(ctx, SearchBlobsRequest{Query: "oauth", RepositoryName: repo.Name})
	if err != nil {
		t.Fatalf("SearchBlobs: %v", err)
	}
	if len(results) != 1 || results[0].Path != "docs/auth.md" {
		t.Fatalf("expected docs/auth.md, got %+v", results)
	}
}

func mustUUID() string {
	// Minimal dependency-free unique id generator for this test file's raw
	// inserts — production code mints IDs via gormstore's BeforeCreate hooks
	// (uuid.NewString()); these raw-map inserts bypass hooks, so id is
	// supplied explicitly.
	return fmt.Sprintf("test-%d", time.Now().UnixNano())
}
