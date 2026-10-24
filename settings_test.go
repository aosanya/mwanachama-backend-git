package mwanachamagit

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
	"github.com/aosanya/mwanachama-backend-shared/spec"
)

func TestDocumentStoreSettingsDefaultToNone(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	var settings DocumentStoreSettings = m

	repos, err := settings.Repositories(ctx)
	if err != nil || len(repos) != 0 {
		t.Fatalf("Repositories = %v, %v; want none", repos, err)
	}
	patterns, err := settings.TagPatterns(ctx)
	if err != nil || len(patterns) != 0 {
		t.Fatalf("TagPatterns = %v, %v; want none", patterns, err)
	}
}

func TestDocumentStoreSettingsResolveThroughTheStoredLayers(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	s := m.store.Spec()

	if err := spec.PutDefinitionSettings(m.db, s, map[string]any{
		"repositories": []any{"contracts", "minutes"},
		"tag_patterns": []any{"v*"},
	}); err != nil {
		t.Fatalf("PutDefinitionSettings: %v", err)
	}
	repos, err := m.Repositories(ctx)
	if err != nil || !reflect.DeepEqual(repos, []string{"contracts", "minutes"}) {
		t.Fatalf("Repositories = %v, %v", repos, err)
	}

	if _, err := spec.StoreOverride(m.db, s, "tag_patterns", []any{"release-*"}, "owner", models.NowRFC3339()); err != nil {
		t.Fatalf("StoreOverride: %v", err)
	}
	patterns, err := m.TagPatterns(ctx)
	if err != nil || !reflect.DeepEqual(patterns, []string{"release-*"}) {
		t.Fatalf("TagPatterns = %v, %v; want the override", patterns, err)
	}
}

func TestCreateTagIsHeldToTheDeclaredPatterns(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	commit := createCommit(t, m, repo.ID, "abcdef0")

	if err := spec.PutDefinitionSettings(m.db, m.store.Spec(), map[string]any{
		"tag_patterns": []any{"v*", "release-*"},
	}); err != nil {
		t.Fatalf("PutDefinitionSettings: %v", err)
	}

	if _, err := m.CreateTag(ctx, CreateTagRequest{RepositoryID: repo.ID, Name: "draft-3", CommitID: commit.ID}); !errors.Is(err, ErrTagNameNotAllowed) {
		t.Fatalf("CreateTag(draft-3) = %v; want ErrTagNameNotAllowed", err)
	}
	if _, err := m.CreateTag(ctx, CreateTagRequest{RepositoryID: repo.ID, Name: "release-2026", CommitID: commit.ID}); err != nil {
		t.Fatalf("CreateTag(release-2026): %v", err)
	}
}

func TestCreateTagAcceptsAnyNameWithNoPatterns(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	commit := createCommit(t, m, repo.ID, "abcdef1")
	if _, err := m.CreateTag(ctx, CreateTagRequest{RepositoryID: repo.ID, Name: "anything at all", CommitID: commit.ID}); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
}

func TestParseStringListRefusesANonName(t *testing.T) {
	if _, err := ParseStringList("repositories", []any{"a", 3.0}); err == nil {
		t.Fatal("a number was accepted as a repository name")
	}
	if _, err := ParseStringList("repositories", "a"); err == nil {
		t.Fatal("a bare string was accepted as a list")
	}
}

func TestTagNameAllowedRefusesABadPattern(t *testing.T) {
	if _, err := TagNameAllowed([]string{"["}, "v1"); err == nil {
		t.Fatal("a malformed pattern was accepted")
	}
}
