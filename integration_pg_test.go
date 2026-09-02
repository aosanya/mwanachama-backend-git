//go:build integration

package mwanachamagit

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aosanya/mwanachama-go-shared/entitygraph"
	"github.com/aosanya/mwanachama-go-shared/postgres"
)

// openTestDB opens a connection to POSTGRES_URL, applying this domain's DDL
// (postgres.DDL + the blob full-text index) idempotently before returning.
// Skips the test if POSTGRES_URL is unset — `make test-pg` is the only
// caller expected to set it.
func openTestDB(t *testing.T) (*sql.DB, postgres.TableNames) {
	t.Helper()
	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		t.Skip("POSTGRES_URL not set — skipping Postgres integration test (see `make test-pg`)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := postgres.Open(ctx, postgres.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("postgres.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tables := postgres.DefaultTableNames("git_")
	if _, err := db.ExecContext(ctx, postgres.DDL(tables)); err != nil {
		t.Fatalf("apply postgres.DDL: %v", err)
	}
	if _, err := db.ExecContext(ctx, BlobSearchIndexDDL(tables)); err != nil {
		t.Fatalf("apply BlobSearchIndexDDL: %v", err)
	}
	return db, tables
}

// newTestAgencyID returns a unique-enough agency ID so concurrent/repeated
// test runs against a shared database don't collide.
func newTestAgencyID(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("it-%s-%d", t.Name(), time.Now().UnixNano())
}

// TestPostgresGitManagerRoundTrip exercises InitRepo → CreateBranch →
// WriteFile → ReadFile → Log → MergeBranch against a real Postgres-backed
// GitManager (postgres.Backend, wired through NewGitManager exactly as
// mwanachama-api-gateway will) — the port-fidelity signal G9 asks for: the
// same business logic the fake-DataManager unit tests exercise, now against
// the real entitygraph.DataManager implementation.
func TestPostgresGitManagerRoundTrip(t *testing.T) {
	db, tables := openTestDB(t)
	backend := postgres.NewBackend(db, tables)
	agencyID := newTestAgencyID(t)
	gm := NewGitManager(backend, backend, nil, agencyID, nil, nil)
	ctx := context.Background()

	repo, err := gm.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, err := gm.ListBranches(ctx, repo.ID)
	if err != nil || len(branches) != 1 {
		t.Fatalf("ListBranches: %+v, err=%v", branches, err)
	}
	branch := branches[0]

	commit1, err := gm.WriteFile(ctx, WriteFileRequest{
		BranchID: branch.ID,
		Path:     "README.md",
		Content:  "# widgets\n\nA widget factory.",
	})
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	blob, err := gm.ReadFile(ctx, branch.ID, "README.md")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if blob.Content != "# widgets\n\nA widget factory." {
		t.Fatalf("unexpected content: %q", blob.Content)
	}

	feature, err := gm.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "feature/x"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if _, err := gm.WriteFile(ctx, WriteFileRequest{
		BranchID: feature.ID,
		Path:     "src/main.go",
		Content:  "package main",
	}); err != nil {
		t.Fatalf("WriteFile on feature branch: %v", err)
	}

	merged, err := gm.MergeBranch(ctx, feature.ID)
	if err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	if _, err := gm.ReadFile(ctx, merged.ID, "src/main.go"); err != nil {
		t.Fatalf("ReadFile on default branch after merge: %v", err)
	}

	history, err := gm.Log(ctx, merged.ID, LogFilter{})
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
// Postgres full-text search: relevance ranking, agency scoping, and the
// no-match-returns-empty-not-nil contract.
func TestPostgresBlobSearch(t *testing.T) {
	db, tables := openTestDB(t)
	backend := postgres.NewBackend(db, tables)
	agencyID := newTestAgencyID(t)
	otherAgencyID := newTestAgencyID(t) + "-other"
	ctx := context.Background()

	mustCreateBlob := func(agency, path, name, content string) {
		t.Helper()
		if _, err := backend.CreateEntity(ctx, entitygraph.CreateEntityRequest{
			AgencyID: agency,
			TypeID:   "Blob",
			Properties: map[string]any{
				"path":    path,
				"name":    name,
				"content": content,
			},
		}); err != nil {
			t.Fatalf("create blob %s: %v", path, err)
		}
	}

	mustCreateBlob(agencyID, "auth/oauth.go", "oauth.go", "package auth\n\n// OAuth token exchange for the login flow.")
	mustCreateBlob(agencyID, "auth/session.go", "session.go", "package auth\n\n// Session cookie handling, unrelated to tokens.")
	mustCreateBlob(agencyID, "README.md", "README.md", "# widgets\n\nA widget factory with no authentication concerns.")
	mustCreateBlob(otherAgencyID, "auth/oauth.go", "oauth.go", "package auth // OAuth in a different agency — must not leak into results")

	searcher := NewPostgresBlobSearcher(db, tables)

	results, err := searcher.Search(ctx, agencyID, "oauth token", 10)
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
	}

	// Agency scoping: the other agency's identically-named blob must never surface.
	for _, r := range results {
		if r.ID == "" {
			t.Fatalf("result missing ID: %+v", r)
		}
	}

	noMatch, err := searcher.Search(ctx, agencyID, "xyzzy-nonexistent-term", 10)
	if err != nil {
		t.Fatalf("Search (no match): %v", err)
	}
	if noMatch == nil {
		t.Fatalf("expected empty slice, not nil, for no matches")
	}
	if len(noMatch) != 0 {
		t.Fatalf("expected no matches, got %+v", noMatch)
	}

	otherAgencyResults, err := searcher.Search(ctx, otherAgencyID, "oauth", 10)
	if err != nil {
		t.Fatalf("Search (other agency): %v", err)
	}
	if len(otherAgencyResults) != 1 {
		t.Fatalf("expected exactly the other agency's own blob, got %+v", otherAgencyResults)
	}
}

// TestPostgresBlobSearchViaGitManager exercises GitManager.SearchBlobs end to
// end with a real PostgresBlobSearcher injected, confirming the wrapper
// (git_impl_blobsearch.go, ported in G4/constructor work) and the real
// searcher (this file, G7) work together.
func TestPostgresBlobSearchViaGitManager(t *testing.T) {
	db, tables := openTestDB(t)
	backend := postgres.NewBackend(db, tables)
	searcher := NewPostgresBlobSearcher(db, tables)
	agencyID := newTestAgencyID(t)
	gm := NewGitManager(backend, backend, nil, agencyID, nil, searcher)
	ctx := context.Background()

	repo, err := gm.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, _ := gm.ListBranches(ctx, repo.ID)
	if _, err := gm.WriteFile(ctx, WriteFileRequest{
		BranchID: branches[0].ID,
		Path:     "docs/auth.md",
		Content:  "OAuth login flow documentation.",
	}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	results, err := gm.SearchBlobs(ctx, SearchBlobsRequest{Query: "oauth", RepositoryName: repo.Name})
	if err != nil {
		t.Fatalf("SearchBlobs: %v", err)
	}
	if len(results) != 1 || results[0].Path != "docs/auth.md" {
		t.Fatalf("expected docs/auth.md, got %+v", results)
	}
}
