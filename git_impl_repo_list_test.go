package mwanachamagit

import (
	"context"
	"testing"
)

func TestListRepositorySummariesCarriesEachMainBranchID(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)

	alpha, err := m.InitRepo(ctx, CreateRepoRequest{Name: "alpha"})
	if err != nil {
		t.Fatalf("InitRepo alpha: %v", err)
	}
	beta, err := m.InitRepo(ctx, CreateRepoRequest{Name: "beta", DefaultBranch: "trunk"})
	if err != nil {
		t.Fatalf("InitRepo beta: %v", err)
	}
	if _, err := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: alpha.ID, Name: "feature/x"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	got, err := m.ListRepositorySummaries(ctx)
	if err != nil {
		t.Fatalf("ListRepositorySummaries: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 repositories, got %+v", got)
	}

	byName := map[string]string{}
	for _, s := range got {
		byName[s.Name] = s.DefaultBranchID
		if s.DefaultBranchID == "" {
			t.Errorf("%s has no default_branch_id", s.Name)
		}
	}
	wantAlpha, _ := m.GetBranchByName(ctx, alpha.ID, "main")
	wantBeta, _ := m.GetBranchByName(ctx, beta.ID, "trunk")
	if byName["alpha"] != wantAlpha.ID {
		t.Errorf("alpha default_branch_id = %q, want %q", byName["alpha"], wantAlpha.ID)
	}
	if byName["beta"] != wantBeta.ID {
		t.Errorf("beta default_branch_id = %q, want %q", byName["beta"], wantBeta.ID)
	}
	for _, s := range got {
		if s.Name == "beta" && s.DefaultBranch != "trunk" {
			t.Errorf("beta default_branch = %q, want trunk", s.DefaultBranch)
		}
	}
}

func TestListRepositorySummariesLeavesOutADeletedRepository(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)

	keep, err := m.InitRepo(ctx, CreateRepoRequest{Name: "keep"})
	if err != nil {
		t.Fatalf("InitRepo keep: %v", err)
	}
	gone, err := m.InitRepo(ctx, CreateRepoRequest{Name: "gone"})
	if err != nil {
		t.Fatalf("InitRepo gone: %v", err)
	}
	if err := m.DeleteRepo(ctx, gone.ID); err != nil {
		t.Fatalf("DeleteRepo: %v", err)
	}

	got, err := m.ListRepositorySummaries(ctx)
	if err != nil {
		t.Fatalf("ListRepositorySummaries: %v", err)
	}
	if len(got) != 1 || got[0].ID != keep.ID {
		t.Fatalf("want only keep, got %+v", got)
	}
}

func TestListRepositorySummariesIsEmptyOnAFreshCatalog(t *testing.T) {
	got, err := newTestManager(t).ListRepositorySummaries(context.Background())
	if err != nil {
		t.Fatalf("ListRepositorySummaries: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("want an empty non-nil list, got %#v", got)
	}
}
