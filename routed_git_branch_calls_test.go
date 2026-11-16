package mwanachamagit

import (
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func TestRoutedGitManager_ABranchIsDeletedInTheFileThatHoldsItInEitherOrder(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	aaa, aMain := f.initRepo(t, "aaa")
	zzz, zMain := f.initRepo(t, "zzz")
	inA, err := f.manager.CreateBranch(f.ctx, CreateBranchRequest{RepositoryID: aaa.ID, Name: "gone", FromBranchID: aMain.ID})
	if err != nil {
		t.Fatalf("CreateBranch in the first file: %v", err)
	}
	inZ, err := f.manager.CreateBranch(f.ctx, CreateBranchRequest{RepositoryID: zzz.ID, Name: "gone", FromBranchID: zMain.ID})
	if err != nil {
		t.Fatalf("CreateBranch in the second file: %v", err)
	}
	if err := f.manager.DeleteBranch(f.ctx, inZ.ID); err != nil {
		t.Fatalf("DeleteBranch in the second file: %v", err)
	}
	if _, err := f.manager.GetBranch(f.ctx, inZ.ID); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("GetBranch of the deleted branch = %v, want ErrBranchNotFound", err)
	}
	if kept, err := f.manager.GetBranch(f.ctx, inA.ID); err != nil || kept.ID != inA.ID {
		t.Fatalf("GetBranch of the same-named branch in the first file = %+v, %v; want it kept", kept, err)
	}
	if err := f.manager.DeleteBranch(f.ctx, inA.ID); err != nil {
		t.Fatalf("DeleteBranch in the first file: %v", err)
	}
	if err := f.manager.DeleteBranch(f.ctx, "no-such-branch"); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("DeleteBranch of an unknown branch = %v, want ErrBranchNotFound", err)
	}
}

func TestRoutedGitManager_TheDefaultBranchOfTheSecondFileCannotBeDeleted(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	f.initRepo(t, "aaa")
	_, zMain := f.initRepo(t, "zzz")
	if err := f.manager.DeleteBranch(f.ctx, zMain.ID); !errors.Is(err, ErrDefaultBranchDeleteForbidden) {
		t.Fatalf("DeleteBranch of the default branch = %v, want ErrDefaultBranchDeleteForbidden", err)
	}
}

func TestRoutedGitManager_AFileIsDeletedOnTheBranchInTheFileThatHoldsItInEitherOrder(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	aaa, aMain := f.initRepo(t, "aaa")
	zzz, zMain := f.initRepo(t, "zzz")
	branches := map[string]models.Branch{}
	for name, pair := range map[string]struct {
		repo models.Repository
		main models.Branch
	}{"aaa": {aaa, aMain}, "zzz": {zzz, zMain}} {
		branch, err := f.manager.CreateBranch(f.ctx, CreateBranchRequest{RepositoryID: pair.repo.ID, Name: "edit", FromBranchID: pair.main.ID})
		if err != nil {
			t.Fatalf("CreateBranch in %s: %v", name, err)
		}
		for _, path := range []string{"keep.md", "drop.md"} {
			if _, err := f.manager.WriteFile(f.ctx, WriteFileRequest{BranchID: branch.ID, Path: path, Content: path + " in " + name}); err != nil {
				t.Fatalf("WriteFile %s in %s: %v", path, name, err)
			}
		}
		branches[name] = branch
	}
	for _, name := range []string{"zzz", "aaa"} {
		branch := branches[name]
		commit, err := f.manager.DeleteFile(f.ctx, DeleteFileRequest{BranchID: branch.ID, Path: "drop.md"})
		if err != nil || commit.ID == "" {
			t.Fatalf("DeleteFile in %s = %+v, %v", name, commit, err)
		}
		if _, err := f.manager.ReadFile(f.ctx, branch.ID, "drop.md"); !errors.Is(err, ErrFileNotFound) {
			t.Fatalf("ReadFile of the deleted file in %s = %v, want ErrFileNotFound", name, err)
		}
		kept, err := f.manager.ReadFile(f.ctx, branch.ID, "keep.md")
		if err != nil || kept.Content != "keep.md in "+name {
			t.Fatalf("ReadFile of the kept file in %s = %+v, %v", name, kept, err)
		}
	}
	if _, err := f.manager.DeleteFile(f.ctx, DeleteFileRequest{BranchID: "no-such-branch", Path: "drop.md"}); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("DeleteFile on an unknown branch = %v, want ErrBranchNotFound", err)
	}
}

func TestRoutedGitManager_BranchesAreListedAndFilteredByWorkflowRunFromTheRepositoryFile(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	aaa, aMain := f.initRepo(t, "aaa")
	zzz, zMain := f.initRepo(t, "zzz")
	for _, pair := range []struct {
		repo models.Repository
		main models.Branch
	}{{zzz, zMain}, {aaa, aMain}} {
		for name, run := range map[string]string{"in-run": "run-1", "other-run": "run-2", "no-run": ""} {
			if _, err := f.manager.CreateBranch(f.ctx, CreateBranchRequest{RepositoryID: pair.repo.ID, Name: name, FromBranchID: pair.main.ID, WorkflowRunID: run}); err != nil {
				t.Fatalf("CreateBranch %s in %s: %v", name, pair.repo.Name, err)
			}
		}
	}
	for _, repo := range []models.Repository{zzz, aaa, zzz} {
		filtered, err := f.manager.ListBranchesFiltered(f.ctx, repo.ID, BranchFilter{WorkflowRunID: "run-1"})
		if err != nil || len(filtered) != 1 || filtered[0].Name != "in-run" || filtered[0].RepositoryID != repo.ID {
			t.Fatalf("ListBranchesFiltered of %s = %+v, %v; want only its own in-run branch", repo.Name, filtered, err)
		}
		everything, err := f.manager.ListBranchesFiltered(f.ctx, repo.ID, BranchFilter{})
		if err != nil || len(everything) != 4 {
			t.Fatalf("ListBranchesFiltered of %s with no filter = %+v, %v; want its four branches", repo.Name, everything, err)
		}
	}
	if _, err := f.manager.ListBranchesFiltered(f.ctx, "no-such-id", BranchFilter{}); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("ListBranchesFiltered of an unknown repository = %v, want ErrRepoNotInitialised", err)
	}
}
