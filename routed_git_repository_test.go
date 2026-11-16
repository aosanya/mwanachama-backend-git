package mwanachamagit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func TestRoutedGitManager_TwoRepositoriesAreBothListedFromTheCatalog(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	f.initRepo(t, "widgets")
	f.initRepo(t, "reports")
	listed, err := f.manager.ListRepositories(f.ctx)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(listed) != 2 || listed[0].Name != "reports" || listed[1].Name != "widgets" {
		t.Fatalf("ListRepositories = %+v, want reports then widgets", listed)
	}
	summaries, err := f.manager.ListRepositorySummaries(f.ctx)
	if err != nil {
		t.Fatalf("ListRepositorySummaries: %v", err)
	}
	if len(summaries) != 2 || summaries[0].Name != "reports" || summaries[0].DefaultBranchID == "" || summaries[1].DefaultBranchID == "" {
		t.Fatalf("ListRepositorySummaries = %+v, want both with a default branch id", summaries)
	}
	entries, err := f.catalog.List(f.ctx)
	if err != nil || len(entries) != 2 {
		t.Fatalf("catalog = %+v, %v; want two entries", entries, err)
	}
	for _, entry := range entries {
		if _, err := os.Stat(filepath.Join(f.folder, entry.FileName)); err != nil {
			t.Errorf("file of %s: %v", entry.Name, err)
		}
	}
}

func TestRoutedGitManager_RepositoriesAreFoundByIDAndByName(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	widgets, _ := f.initRepo(t, "widgets")
	f.initRepo(t, "reports")
	byID, err := f.manager.GetRepository(f.ctx, widgets.ID)
	if err != nil || byID.Name != "widgets" {
		t.Fatalf("GetRepository = %+v, %v", byID, err)
	}
	byName, err := f.manager.GetRepositoryByName(f.ctx, "widgets")
	if err != nil || byName.ID != widgets.ID {
		t.Fatalf("GetRepositoryByName = %+v, %v", byName, err)
	}
	if _, err := f.manager.GetRepository(f.ctx, "no-such-id"); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("unknown id = %v, want ErrRepoNotInitialised", err)
	}
	if _, err := f.manager.GetRepositoryByName(f.ctx, "no-such-name"); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("unknown name = %v, want ErrRepoNotInitialised", err)
	}
}

func TestRoutedGitManager_ASecondRepositoryWithTheSameNameIsRefused(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	f.initRepo(t, "widgets")
	if _, err := f.manager.InitRepo(f.ctx, CreateRepoRequest{Name: "widgets"}); !errors.Is(err, ErrRepoAlreadyExists) {
		t.Fatalf("duplicate InitRepo = %v, want ErrRepoAlreadyExists", err)
	}
}

func TestRoutedGitManager_ReopeningOverTheSameFolderAndCatalogKeepsTheFile(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	repo, main := f.initRepo(t, "widgets")
	f.writeAndMerge(t, repo, main, "plan.md", "kept")
	again := f.reopened(t)
	blob, err := again.ReadFile(f.ctx, main.ID, "plan.md")
	if err != nil || blob.Content != "kept" {
		t.Fatalf("ReadFile after reopen = %+v, %v", blob, err)
	}
	listed, err := again.ListRepositories(f.ctx)
	if err != nil || len(listed) != 1 || listed[0].Name != "widgets" {
		t.Fatalf("ListRepositories after reopen = %+v, %v", listed, err)
	}
}

func TestRoutedGitManager_AFileCopiedIntoTheFolderWithNoCatalogEntryIsIgnored(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	repo, main := f.initRepo(t, "widgets")
	f.writeAndMerge(t, repo, main, "plan.md", "real")
	entry, err := f.catalog.Lookup(f.ctx, repo.ID)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if err := f.manager.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(f.folder, entry.FileName))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(f.folder, "stray.db"), raw, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	again := f.reopened(t)
	listed, err := again.ListRepositories(f.ctx)
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListRepositories = %+v, %v; want only the catalogued repository", listed, err)
	}
	opened := again.files.With(f.ctx, "stray.db", func(*RepoFile) error { return nil })
	if !errors.Is(opened, ErrRepositoryNotRegistered) {
		t.Fatalf("opening the stray file = %v, want ErrRepositoryNotRegistered", opened)
	}
}

func TestRoutedGitManager_DeletingARepositoryRemovesItsFileAndItsEntry(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	repo, main := f.initRepo(t, "widgets")
	f.initRepo(t, "reports")
	f.writeAndMerge(t, repo, main, "plan.md", "gone soon")
	entry, err := f.catalog.Lookup(f.ctx, repo.ID)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if err := f.manager.DeleteRepo(f.ctx, repo.ID); err != nil {
		t.Fatalf("DeleteRepo: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.folder, entry.FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file after delete: %v, want not exist", err)
	}
	if _, err := f.catalog.Lookup(f.ctx, repo.ID); !errors.Is(err, ErrRepositoryNotCatalogued) {
		t.Fatalf("entry after delete: %v, want not catalogued", err)
	}
	listed, err := f.manager.ListRepositories(f.ctx)
	if err != nil || len(listed) != 1 || listed[0].Name != "reports" {
		t.Fatalf("ListRepositories = %+v, %v; want only reports", listed, err)
	}
	if _, err := f.manager.GetBranch(f.ctx, main.ID); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("GetBranch of the deleted repository = %v, want ErrBranchNotFound", err)
	}
	if err := f.manager.DeleteRepo(f.ctx, repo.ID); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("second DeleteRepo = %v, want ErrRepoNotInitialised", err)
	}
	if err := f.manager.PurgeRepo(f.ctx, "no-such-id"); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("PurgeRepo of an unknown id = %v, want ErrRepoNotInitialised", err)
	}
}

func TestRoutedGitManager_ACommitUpdatesTheCatalogEntrySizeAndLastCommitTime(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	repo, main := f.initRepo(t, "widgets")
	before, err := f.catalog.Lookup(f.ctx, repo.ID)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if before.LastCommitAt != "" {
		t.Fatalf("a fresh repository has last commit %q, want none", before.LastCommitAt)
	}
	work, err := f.manager.CreateBranch(f.ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "work", FromBranchID: main.ID})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	big := make([]byte, 200000)
	for i := range big {
		big[i] = byte('a' + i%23)
	}
	if _, err := f.manager.WriteFile(f.ctx, WriteFileRequest{BranchID: work.ID, Path: "big.txt", Content: string(big)}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	after, err := f.catalog.Lookup(f.ctx, repo.ID)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if after.LastCommitAt == "" {
		t.Fatal("last commit time not recorded after a commit")
	}
	if after.SizeBytes <= before.SizeBytes {
		t.Fatalf("size %d after the commit, %d before; want it to grow", after.SizeBytes, before.SizeBytes)
	}
	listed, err := f.manager.ListRepositories(f.ctx)
	if err != nil || len(listed) != 1 || listed[0].UpdatedAt != after.LastCommitAt {
		t.Fatalf("ListRepositories = %+v, %v; want UpdatedAt %q", listed, err, after.LastCommitAt)
	}
}

func TestRoutedGitManager_ManyRepositoriesShareAFewOpenFiles(t *testing.T) {
	f := newRoutedGitFixture(t, 1)
	names := []string{"one", "two", "three", "four"}
	branches := map[string]models.Branch{}
	for _, name := range names {
		repo, main := f.initRepo(t, name)
		f.writeAndMerge(t, repo, main, name+".md", name)
		branches[name] = main
	}
	for round := 0; round < 2; round++ {
		for _, name := range names {
			blob, err := f.manager.ReadFile(f.ctx, branches[name].ID, name+".md")
			if err != nil || blob.Content != name {
				t.Fatalf("ReadFile %s round %d = %+v, %v", name, round, blob, err)
			}
			if open := f.manager.openFiles(); open > 1 {
				t.Fatalf("%d files open, want at most 1", open)
			}
		}
	}
}
