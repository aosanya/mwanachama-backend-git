package mwanachamagit

import (
	"context"
	"testing"
)

type growingRepositoryFile struct {
	calls int64
}

func (f *growingRepositoryFile) file(repoID string) (string, int64, error) {
	f.calls++
	return repoID + ".sqlite", f.calls * 1000, nil
}

func newCataloguedManager(t *testing.T) (*gitManager, *MemoryRepositoryCatalog) {
	t.Helper()
	m := newTestManager(t)
	catalog := NewMemoryRepositoryCatalog()
	WithRepositoryCatalog(catalog, (&growingRepositoryFile{}).file)(m)
	return m, catalog
}

func TestInitRepoRegistersCatalogEntry(t *testing.T) {
	ctx := context.Background()
	m, catalog := newCataloguedManager(t)

	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	entries, err := catalog.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry.ID != repo.ID || entry.Name != "widgets" {
		t.Fatalf("unexpected identity: %+v", entry)
	}
	if entry.FileName != repo.ID+".sqlite" {
		t.Fatalf("unexpected file name %q", entry.FileName)
	}
	if entry.SizeBytes != 1000 {
		t.Fatalf("unexpected size %d", entry.SizeBytes)
	}
	if entry.LastCommitAt != "" {
		t.Fatalf("expected empty last commit time, got %q", entry.LastCommitAt)
	}
}

func TestWriteFileUpdatesCatalogEntry(t *testing.T) {
	ctx := context.Background()
	m, catalog := newCataloguedManager(t)

	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	registered, err := catalog.Lookup(ctx, repo.ID)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	branches, _ := m.ListBranches(ctx, repo.ID)

	commit, err := m.WriteFile(ctx, WriteFileRequest{BranchID: branches[0].ID, Path: "README.md", Content: "# widgets"})
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	updated, err := catalog.Lookup(ctx, repo.ID)
	if err != nil {
		t.Fatalf("Lookup after commit: %v", err)
	}
	if updated.SizeBytes <= registered.SizeBytes {
		t.Fatalf("expected size to grow from %d, got %d", registered.SizeBytes, updated.SizeBytes)
	}
	if commit.CommittedAt == "" || updated.LastCommitAt != commit.CommittedAt {
		t.Fatalf("expected last commit %q, got %q", commit.CommittedAt, updated.LastCommitAt)
	}
}

func TestCommitLeavesOtherRepositoryEntryUntouched(t *testing.T) {
	ctx := context.Background()
	m, catalog := newCataloguedManager(t)

	first, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo first: %v", err)
	}
	second, err := m.InitRepo(ctx, CreateRepoRequest{Name: "gadgets"})
	if err != nil {
		t.Fatalf("InitRepo second: %v", err)
	}
	before, err := catalog.Lookup(ctx, second.ID)
	if err != nil {
		t.Fatalf("Lookup before: %v", err)
	}
	branches, _ := m.ListBranches(ctx, first.ID)

	if _, err := m.WriteFile(ctx, WriteFileRequest{BranchID: branches[0].ID, Path: "a.txt", Content: "one"}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	after, err := catalog.Lookup(ctx, second.ID)
	if err != nil {
		t.Fatalf("Lookup after: %v", err)
	}
	if after != before {
		t.Fatalf("second entry changed: %+v -> %+v", before, after)
	}
}

func TestDeleteRepoRemovesCatalogEntry(t *testing.T) {
	ctx := context.Background()
	m, catalog := newCataloguedManager(t)

	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	if err := m.DeleteRepo(ctx, repo.ID); err != nil {
		t.Fatalf("DeleteRepo: %v", err)
	}

	entries, err := catalog.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries, got %+v", entries)
	}
}

func TestDeleteRepoToleratesMissingCatalogEntry(t *testing.T) {
	ctx := context.Background()
	m, catalog := newCataloguedManager(t)

	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	if err := catalog.Remove(ctx, repo.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := m.DeleteRepo(ctx, repo.ID); err != nil {
		t.Fatalf("DeleteRepo: %v", err)
	}
}

func TestRepositoryAndFileWorkWithoutCatalog(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)

	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, _ := m.ListBranches(ctx, repo.ID)
	if _, err := m.WriteFile(ctx, WriteFileRequest{BranchID: branches[0].ID, Path: "a.txt", Content: "one"}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := m.DeleteRepo(ctx, repo.ID); err != nil {
		t.Fatalf("DeleteRepo: %v", err)
	}
}
