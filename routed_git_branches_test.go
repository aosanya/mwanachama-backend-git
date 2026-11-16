package mwanachamagit

import (
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func TestRoutedGitManager_AFileWrittenOnABranchIsReadBackFromTheDefaultBranchAfterTheMerge(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	repo, main := f.initRepo(t, "widgets")
	f.writeAndMerge(t, repo, main, "notes/plan.md", "first plan")
	blob, err := f.manager.ReadFile(f.ctx, main.ID, "notes/plan.md")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if blob.Content != "first plan" {
		t.Fatalf("content = %q, want first plan", blob.Content)
	}
	log, err := f.manager.Log(f.ctx, main.ID, LogFilter{})
	if err != nil || len(log) == 0 {
		t.Fatalf("Log = %+v, %v; want the commit", log, err)
	}
	dir, err := f.manager.ListDirectory(f.ctx, main.ID, "notes")
	if err != nil || len(dir) != 1 {
		t.Fatalf("ListDirectory = %+v, %v; want plan.md", dir, err)
	}
}

func TestRoutedGitManager_TheSecondRepositoryDoesNotSeeTheFirstOnesFile(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	widgets, widgetsMain := f.initRepo(t, "widgets")
	_, reportsMain := f.initRepo(t, "reports")
	f.writeAndMerge(t, widgets, widgetsMain, "secret.md", "widgets only")
	if _, err := f.manager.ReadFile(f.ctx, reportsMain.ID, "secret.md"); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("reports reading widgets' file = %v, want ErrFileNotFound", err)
	}
	if _, err := f.manager.ReadFile(f.ctx, widgetsMain.ID, "secret.md"); err != nil {
		t.Fatalf("widgets reading its own file: %v", err)
	}
}

func TestRoutedGitManager_AnIdIsFoundInWhicheverFileOwnsItInEitherOrder(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	_, aMain := f.initRepo(t, "aaa")
	_, zMain := f.initRepo(t, "zzz")
	for _, want := range []models.Branch{zMain, aMain, zMain, aMain} {
		got, err := f.manager.GetBranch(f.ctx, want.ID)
		if err != nil || got.ID != want.ID || got.RepositoryID != want.RepositoryID {
			t.Fatalf("GetBranch(%s) = %+v, %v", want.ID, got, err)
		}
	}
	if _, err := f.manager.GetBranch(f.ctx, "no-such-branch"); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("unknown branch = %v, want ErrBranchNotFound", err)
	}
}
