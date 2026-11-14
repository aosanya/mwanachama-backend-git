package mwanachamagit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryFolderPurge_LeavesNothingOfTheRepositoryBehind(t *testing.T) {
	ctx := context.Background()
	f := newRepositoryFolderFixture(t)
	files, err := NewRepoFiles(f.folder, 4, RegistrationCheckFromCatalog(f.catalog), f.open)
	if err != nil {
		t.Fatalf("NewRepoFiles: %v", err)
	}
	t.Cleanup(func() { _ = files.Close() })
	folder := f.repositoryFolder(WithRepoFiles(files))
	repo := f.create(t, folder, "widgets")
	fileName := f.entries(t)[0].FileName
	dbPath := filepath.Join(f.folder, fileName)
	journalPath := dbPath + "-journal"

	err = files.With(ctx, fileName, func(rf *RepoFile) error {
		branches, err := rf.ListBranches(ctx, repo.ID)
		if err != nil {
			return err
		}
		_, err = rf.WriteFile(ctx, WriteFileRequest{BranchID: branches[0].ID, Path: "a.txt", Content: "hello", Message: "add a"})
		return err
	})
	if err != nil {
		t.Fatalf("write through the cache: %v", err)
	}
	if err := os.WriteFile(journalPath, []byte("left by a crash"), 0o644); err != nil {
		t.Fatalf("plant journal: %v", err)
	}
	if files.Open() != 1 {
		t.Fatalf("open files before Purge = %d, want 1", files.Open())
	}

	if err := folder.Purge(ctx, repo.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("Stat of the .db after Purge = %v, want it gone", err)
	}
	if _, err := os.Stat(journalPath); !os.IsNotExist(err) {
		t.Fatalf("Stat of the -journal after Purge = %v, want it gone", err)
	}
	if entries := f.entries(t); len(entries) != 0 {
		t.Fatalf("catalog entries after Purge = %+v, want none", entries)
	}
	if files.Open() != 0 {
		t.Fatalf("open files after Purge = %d, want none held for the purged repository", files.Open())
	}
	again, err := folder.Create(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("Create of the same name after Purge: %v", err)
	}
	if again.ID == repo.ID {
		t.Fatalf("new repository reuses the purged id %q", repo.ID)
	}
}
