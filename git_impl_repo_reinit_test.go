package mwanachamagit

import (
	"context"
	"errors"
	"testing"
)

func TestSingleRepositoryInitRepoSucceedsAgainAfterDeleteRepo(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	m.singleRepository = true

	first, err := m.InitRepo(ctx, CreateRepoRequest{Name: "only"})
	if err != nil {
		t.Fatalf("InitRepo first: %v", err)
	}
	if err := m.DeleteRepo(ctx, first.ID); err != nil {
		t.Fatalf("DeleteRepo: %v", err)
	}

	second, err := m.InitRepo(ctx, CreateRepoRequest{Name: "only"})
	if err != nil {
		t.Fatalf("InitRepo again after delete: %v", err)
	}
	if second.ID == first.ID {
		t.Errorf("recreated repository reused id %s", first.ID)
	}

	listed, err := m.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != second.ID {
		t.Errorf("want exactly the recreated repository listed, got %+v", listed)
	}
}

func TestSingleRepositoryInitRepoStillRefusesASecondLiveRepository(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	m.singleRepository = true

	if _, err := m.InitRepo(ctx, CreateRepoRequest{Name: "only"}); err != nil {
		t.Fatalf("InitRepo first: %v", err)
	}
	if _, err := m.InitRepo(ctx, CreateRepoRequest{Name: "other"}); !errors.Is(err, ErrRepoAlreadyExists) {
		t.Fatalf("second live repository: got %v, want ErrRepoAlreadyExists", err)
	}
}
