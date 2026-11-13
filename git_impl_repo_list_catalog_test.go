package mwanachamagit

import (
	"context"
	"testing"
)

func commitTo(t *testing.T, m *gitManager, repoID, path string) string {
	t.Helper()
	ctx := context.Background()
	branches, err := m.ListBranches(ctx, repoID)
	if err != nil || len(branches) == 0 {
		t.Fatalf("ListBranches: %v", err)
	}
	commit, err := m.WriteFile(ctx, WriteFileRequest{BranchID: branches[0].ID, Path: path, Content: "x"})
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return commit.CommittedAt
}

func TestListRepositoriesEqualsTheCatalogEntries(t *testing.T) {
	ctx := context.Background()
	m, catalog := newCataloguedManager(t)
	widgets, _ := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets", Description: "the widgets", DefaultBranch: "trunk"})
	gadgets, _ := m.InitRepo(ctx, CreateRepoRequest{Name: "gadgets"})
	committedAt := commitTo(t, m, widgets.ID, "README.md")

	got, err := m.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	entries, _ := catalog.List(ctx)
	if len(got) != len(entries) || len(got) != 2 {
		t.Fatalf("want %d repositories, got %+v", len(entries), got)
	}
	for i, entry := range entries {
		if got[i].ID != entry.ID || got[i].Name != entry.Name {
			t.Errorf("position %d: want %s/%s, got %s/%s", i, entry.ID, entry.Name, got[i].ID, got[i].Name)
		}
	}
	byID := map[string]int{got[0].ID: 0, got[1].ID: 1}
	w := got[byID[widgets.ID]]
	if w.UpdatedAt != committedAt || committedAt == "" {
		t.Errorf("widgets UpdatedAt = %q, want last commit %q", w.UpdatedAt, committedAt)
	}
	if w.Description != "the widgets" || w.DefaultBranch != "trunk" || w.CreatedAt != widgets.CreatedAt {
		t.Errorf("widgets row fields not carried: %+v", w)
	}
	g := got[byID[gadgets.ID]]
	if g.UpdatedAt != gadgets.UpdatedAt {
		t.Errorf("gadgets UpdatedAt = %q, want row time %q", g.UpdatedAt, gadgets.UpdatedAt)
	}
}

func TestListRepositoriesLeavesOutARowWithoutACatalogEntry(t *testing.T) {
	ctx := context.Background()
	m, catalog := newCataloguedManager(t)
	keep, _ := m.InitRepo(ctx, CreateRepoRequest{Name: "keep"})
	orphan, _ := m.InitRepo(ctx, CreateRepoRequest{Name: "orphan"})
	if err := catalog.Remove(ctx, orphan.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	got, err := m.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(got) != 1 || got[0].ID != keep.ID {
		t.Fatalf("want only keep, got %+v", got)
	}
}

func TestListRepositoriesLeavesOutARepositoryRemovedThroughDeleteRepo(t *testing.T) {
	ctx := context.Background()
	m, _ := newCataloguedManager(t)
	keep, _ := m.InitRepo(ctx, CreateRepoRequest{Name: "keep"})
	gone, _ := m.InitRepo(ctx, CreateRepoRequest{Name: "gone"})
	if err := m.DeleteRepo(ctx, gone.ID); err != nil {
		t.Fatalf("DeleteRepo: %v", err)
	}

	got, err := m.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(got) != 1 || got[0].ID != keep.ID {
		t.Fatalf("want only keep, got %+v", got)
	}
}

func TestListRepositoriesListsAnEntryWithoutARow(t *testing.T) {
	ctx := context.Background()
	m, catalog := newCataloguedManager(t)
	if err := catalog.Register(ctx, RepositoryCatalogEntry{ID: "ghost-id", Name: "ghost", LastCommitAt: "2026-01-02T03:04:05Z"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := m.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(got) != 1 || got[0].ID != "ghost-id" || got[0].Name != "ghost" || got[0].UpdatedAt != "2026-01-02T03:04:05Z" {
		t.Fatalf("unexpected listing %+v", got)
	}
	if got[0].DefaultBranch != "" {
		t.Errorf("want empty default branch, got %q", got[0].DefaultBranch)
	}
}

func TestListRepositorySummariesFollowTheCatalog(t *testing.T) {
	ctx := context.Background()
	m, catalog := newCataloguedManager(t)
	keep, _ := m.InitRepo(ctx, CreateRepoRequest{Name: "keep", DefaultBranch: "trunk"})
	orphan, _ := m.InitRepo(ctx, CreateRepoRequest{Name: "orphan"})
	gone, _ := m.InitRepo(ctx, CreateRepoRequest{Name: "gone"})
	if err := catalog.Remove(ctx, orphan.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := m.DeleteRepo(ctx, gone.ID); err != nil {
		t.Fatalf("DeleteRepo: %v", err)
	}
	if err := catalog.Register(ctx, RepositoryCatalogEntry{ID: "ghost-id", Name: "zzz-ghost"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := m.ListRepositorySummaries(ctx)
	if err != nil {
		t.Fatalf("ListRepositorySummaries: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want keep and ghost, got %+v", got)
	}
	trunk, _ := m.GetBranchByName(ctx, keep.ID, "trunk")
	if got[0].ID != keep.ID || got[0].DefaultBranch != "trunk" || got[0].DefaultBranchID != trunk.ID {
		t.Errorf("keep summary wrong: %+v", got[0])
	}
	if got[1].ID != "ghost-id" || got[1].DefaultBranch != "" || got[1].DefaultBranchID != "" {
		t.Errorf("ghost summary wrong: %+v", got[1])
	}
}
