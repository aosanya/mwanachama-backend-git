package mwanachamagit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func (f *routedGitFixture) workBranchIn(t *testing.T, repo models.Repository, main models.Branch) models.Branch {
	t.Helper()
	branch, err := f.manager.CreateBranch(f.ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "work", FromBranchID: main.ID})
	if err != nil {
		t.Fatalf("CreateBranch in %s: %v", repo.Name, err)
	}
	return branch
}

func (f *routedGitFixture) rememberedBranchOwner(branchID string) (string, bool) {
	return f.manager.owners.get("branch:" + branchID)
}

func TestRoutedGitManager_ABranchIdRemembersTheFileThatAnsweredFirst(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	f.initRepo(t, "aaa")
	zzz, zMain := f.initRepo(t, "zzz")
	work := f.workBranchIn(t, zzz, zMain)
	if _, remembered := f.rememberedBranchOwner(work.ID); remembered {
		t.Fatal("an owner is remembered before any call by branch id")
	}
	if _, err := f.manager.GetBranch(f.ctx, work.ID); err != nil {
		t.Fatalf("GetBranch: %v", err)
	}
	entry, err := f.catalog.Lookup(f.ctx, zzz.ID)
	if err != nil {
		t.Fatalf("catalog lookup: %v", err)
	}
	if owner, remembered := f.rememberedBranchOwner(work.ID); !remembered || owner != entry.FileName {
		t.Fatalf("remembered owner = %q, %v; want %q", owner, remembered, entry.FileName)
	}
}

func TestRoutedGitManager_ARememberedOwnerWhoseRepositoryMovedToAnotherFileIsForgottenAndTheFilesProbedAgain(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	f.initRepo(t, "aaa")
	zzz, zMain := f.initRepo(t, "zzz")
	work := f.workBranchIn(t, zzz, zMain)
	if _, err := f.manager.GetBranch(f.ctx, work.ID); err != nil {
		t.Fatalf("GetBranch before the move: %v", err)
	}
	entry, err := f.catalog.Lookup(f.ctx, zzz.ID)
	if err != nil {
		t.Fatalf("catalog lookup: %v", err)
	}

	movedName := "moved-" + entry.FileName
	if err := f.manager.files.Evict(entry.FileName); err != nil {
		t.Fatalf("Evict: %v", err)
	}
	if err := os.Rename(filepath.Join(f.folder, entry.FileName), filepath.Join(f.folder, movedName)); err != nil {
		t.Fatalf("rename file: %v", err)
	}
	if err := f.catalog.Remove(f.ctx, zzz.ID); err != nil {
		t.Fatalf("catalog remove: %v", err)
	}
	moved := entry
	moved.FileName = movedName
	if err := f.catalog.Register(f.ctx, moved); err != nil {
		t.Fatalf("catalog register: %v", err)
	}

	got, err := f.manager.GetBranch(f.ctx, work.ID)
	if err != nil || got.ID != work.ID {
		t.Fatalf("GetBranch after the move = %+v, %v; want the branch found in the moved file", got, err)
	}
	if owner, remembered := f.rememberedBranchOwner(work.ID); !remembered || owner != movedName {
		t.Fatalf("remembered owner after the move = %q, %v; want %q", owner, remembered, movedName)
	}
	if _, err := f.manager.WriteFile(f.ctx, WriteFileRequest{BranchID: work.ID, Path: "after-move.md", Content: "written after the move"}); err != nil {
		t.Fatalf("WriteFile after the move: %v", err)
	}
}

func TestRoutedGitManager_ARememberedOwnerWhoseCatalogEntryWasRemovedIsForgottenAndTheBranchIsNotFound(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	f.initRepo(t, "aaa")
	zzz, zMain := f.initRepo(t, "zzz")
	work := f.workBranchIn(t, zzz, zMain)
	if _, err := f.manager.GetBranch(f.ctx, work.ID); err != nil {
		t.Fatalf("GetBranch before the removal: %v", err)
	}
	if err := f.catalog.Remove(f.ctx, zzz.ID); err != nil {
		t.Fatalf("catalog remove: %v", err)
	}

	if _, err := f.manager.GetBranch(f.ctx, work.ID); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("GetBranch after the removal = %v, want ErrBranchNotFound", err)
	}
	if owner, remembered := f.rememberedBranchOwner(work.ID); remembered {
		t.Fatalf("owner %q is still remembered after its entry was removed", owner)
	}
}
