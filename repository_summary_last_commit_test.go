package mwanachamagit

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func TestRepositorySummaryJSONCarriesLastCommitAtAndOmitsItWhenEmpty(t *testing.T) {
	with, err := json.Marshal(models.RepositorySummary{ID: "r1", Name: "widgets", LastCommitAt: "2026-11-13T10:00:00Z"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(with), `"last_commit_at":"2026-11-13T10:00:00Z"`) {
		t.Fatalf("summary JSON = %s, want last_commit_at", with)
	}

	without, err := json.Marshal(models.RepositorySummary{ID: "r1", Name: "widgets"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(without), "last_commit_at") {
		t.Fatalf("summary JSON = %s, want no last_commit_at for a repository with no commit", without)
	}
}

func TestListRepositorySummariesFromTheSharedDatabaseLeavesLastCommitAtEmpty(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	if _, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"}); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	got, err := m.ListRepositorySummaries(ctx)
	if err != nil || len(got) != 1 {
		t.Fatalf("ListRepositorySummaries = %+v, %v; want one", got, err)
	}
	if got[0].LastCommitAt != "" {
		t.Fatalf("LastCommitAt = %q, want empty from the shared-database manager", got[0].LastCommitAt)
	}
}
