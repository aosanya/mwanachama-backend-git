package mwanachamagit

import (
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

type routedEditedFile struct {
	main  models.Branch
	first models.Commit
	last  models.Commit
}

func routedEditAFileTwiceIn(t *testing.T, f *routedGitFixture, repoName string) routedEditedFile {
	t.Helper()
	_, main := f.initRepo(t, repoName)
	first, err := f.manager.WriteFile(f.ctx, WriteFileRequest{BranchID: main.ID, Path: repoName + ".txt", Content: "one"})
	if err != nil {
		t.Fatalf("WriteFile %s one: %v", repoName, err)
	}
	last, err := f.manager.WriteFile(f.ctx, WriteFileRequest{BranchID: main.ID, Path: repoName + ".txt", Content: "two"})
	if err != nil {
		t.Fatalf("WriteFile %s two: %v", repoName, err)
	}
	return routedEditedFile{main: main, first: first, last: last}
}

func TestRoutedGitManager_ADiffOfCommitsOfTheSecondFileIsFoundAfterTheFirstFileDoesNotKnowThem(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	routedEditAFileTwiceIn(t, f, "aaa")
	second := routedEditAFileTwiceIn(t, f, "zzz")
	diffs, err := f.manager.Diff(f.ctx, second.first.ID, second.last.ID)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(diffs) != 1 || diffs[0].Path != "zzz.txt" || diffs[0].Operation != "modified" {
		t.Fatalf("Diff = %+v, want zzz.txt modified", diffs)
	}
}

func TestRoutedGitManager_ADiffOfCommitsOfTheFirstFileIsAnsweredByTheFirstFile(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	first := routedEditAFileTwiceIn(t, f, "aaa")
	routedEditAFileTwiceIn(t, f, "zzz")
	diffs, err := f.manager.Diff(f.ctx, first.first.ID, first.last.ID)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(diffs) != 1 || diffs[0].Path != "aaa.txt" || diffs[0].Operation != "modified" {
		t.Fatalf("Diff = %+v, want aaa.txt modified", diffs)
	}
}

func TestRoutedGitManager_ADiffFromACommitToABranchIdIsFoundInTheFileOfBoth(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	routedEditAFileTwiceIn(t, f, "aaa")
	second := routedEditAFileTwiceIn(t, f, "zzz")
	if _, err := f.manager.WriteFile(f.ctx, WriteFileRequest{BranchID: second.main.ID, Path: "added.txt", Content: "new"}); err != nil {
		t.Fatalf("WriteFile added: %v", err)
	}
	diffs, err := f.manager.Diff(f.ctx, second.first.ID, second.main.ID)
	if err != nil || len(diffs) != 2 {
		t.Fatalf("Diff to the branch id = %+v, %v; want zzz.txt modified and added.txt added", diffs, err)
	}
}

func TestRoutedGitManager_ADiffBetweenARefOfOneFileAndARefOfAnotherAnswersErrRefNotFound(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	first := routedEditAFileTwiceIn(t, f, "aaa")
	second := routedEditAFileTwiceIn(t, f, "zzz")
	if _, err := f.manager.Diff(f.ctx, first.first.ID, second.last.ID); !errors.Is(err, ErrRefNotFound) {
		t.Fatalf("Diff across files = %v, want ErrRefNotFound", err)
	}
	if _, err := f.manager.Diff(f.ctx, second.first.ID, first.last.ID); !errors.Is(err, ErrRefNotFound) {
		t.Fatalf("Diff across files reversed = %v, want ErrRefNotFound", err)
	}
}

func TestRoutedGitManager_ADiffOfUnknownRefsAnswersErrRefNotFound(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	second := routedEditAFileTwiceIn(t, f, "zzz")
	if _, err := f.manager.Diff(f.ctx, "no-such-ref", second.last.ID); !errors.Is(err, ErrRefNotFound) {
		t.Fatalf("Diff from an unknown ref = %v, want ErrRefNotFound", err)
	}
	if _, err := f.manager.Diff(f.ctx, second.first.ID, "no-such-ref"); !errors.Is(err, ErrRefNotFound) {
		t.Fatalf("Diff to an unknown ref = %v, want ErrRefNotFound", err)
	}
}

func TestRoutedGitManager_ADiffWithNoRepositoryAtAllAnswersErrRefNotFound(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	if _, err := f.manager.Diff(f.ctx, "a", "b"); !errors.Is(err, ErrRefNotFound) {
		t.Fatalf("Diff with no repository = %v, want ErrRefNotFound", err)
	}
}
