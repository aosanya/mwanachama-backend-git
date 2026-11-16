package mwanachamagit

import (
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func (f *routedGitFixture) inFileOf(t *testing.T, repo models.Repository, use func(file *RepoFile, manager *gitManager)) {
	t.Helper()
	entry, err := f.catalog.Lookup(f.ctx, repo.ID)
	if err != nil {
		t.Fatalf("catalog lookup of %s: %v", repo.Name, err)
	}
	err = f.manager.files.With(f.ctx, entry.FileName, func(file *RepoFile) error {
		manager, ok := file.GitManager.(*gitManager)
		if !ok {
			t.Fatalf("file of %s holds a %T, want *gitManager", repo.Name, file.GitManager)
		}
		use(file, manager)
		return nil
	})
	if err != nil {
		t.Fatalf("open file of %s: %v", repo.Name, err)
	}
}

func (f *routedGitFixture) seedImportJob(t *testing.T, repo models.Repository) models.ImportJob {
	t.Helper()
	now := models.NowRFC3339()
	job := models.ImportJob{
		Name: repo.Name, SourceURL: "https://example.invalid/" + repo.Name + ".git", DefaultBranch: "main",
		Status: models.ImportStatusPending, CreatedAt: now, UpdatedAt: now,
	}
	ensureID(&job)
	f.inFileOf(t, repo, func(_ *RepoFile, manager *gitManager) {
		if err := manager.db.WithContext(f.ctx).Table(manager.tables.ImportJobs).Create(&job).Error; err != nil {
			t.Fatalf("seed import job in %s: %v", repo.Name, err)
		}
	})
	return job
}

func TestRoutedGitManager_AnImportJobIsFoundInWhicheverFileHoldsItInEitherOrder(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	aaa, _ := f.initRepo(t, "aaa")
	zzz, _ := f.initRepo(t, "zzz")
	inA := f.seedImportJob(t, aaa)
	inZ := f.seedImportJob(t, zzz)
	for _, want := range []models.ImportJob{inZ, inA, inZ, inA} {
		got, err := f.manager.GetImportStatus(f.ctx, want.ID)
		if err != nil || got.ID != want.ID || got.SourceURL != want.SourceURL {
			t.Fatalf("GetImportStatus(%s) = %+v, %v", want.ID, got, err)
		}
	}
}

func TestRoutedGitManager_AnUnknownImportJobIdIsNotFoundInAnyFile(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	f.initRepo(t, "aaa")
	f.initRepo(t, "zzz")
	if _, err := f.manager.GetImportStatus(f.ctx, "no-such-job"); !errors.Is(err, ErrImportJobNotFound) {
		t.Fatalf("GetImportStatus of an unknown job = %v, want ErrImportJobNotFound", err)
	}
	if err := f.manager.CancelImport(f.ctx, "no-such-job"); !errors.Is(err, ErrImportJobNotFound) {
		t.Fatalf("CancelImport of an unknown job = %v, want ErrImportJobNotFound", err)
	}
}

func TestRoutedGitManager_CancellingAnImportJobReachesTheFileThatHoldsItAndLeavesTheOtherAlone(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	aaa, _ := f.initRepo(t, "aaa")
	zzz, _ := f.initRepo(t, "zzz")
	inA := f.seedImportJob(t, aaa)
	inZ := f.seedImportJob(t, zzz)
	if err := f.manager.CancelImport(f.ctx, inZ.ID); err != nil {
		t.Fatalf("CancelImport in the second file: %v", err)
	}
	cancelled, err := f.manager.GetImportStatus(f.ctx, inZ.ID)
	if err != nil || cancelled.Status != models.ImportStatusCancelled {
		t.Fatalf("job of the second file after cancel = %+v, %v; want cancelled", cancelled, err)
	}
	untouched, err := f.manager.GetImportStatus(f.ctx, inA.ID)
	if err != nil || untouched.Status != models.ImportStatusPending {
		t.Fatalf("job of the first file after the other cancel = %+v, %v; want pending", untouched, err)
	}
	if err := f.manager.CancelImport(f.ctx, inZ.ID); !errors.Is(err, ErrImportJobNotCancellable) {
		t.Fatalf("second CancelImport = %v, want ErrImportJobNotCancellable", err)
	}
	if err := f.manager.CancelImport(f.ctx, inA.ID); err != nil {
		t.Fatalf("CancelImport in the first file: %v", err)
	}
}

func TestRoutedGitManager_AFetchJobIsFoundInWhicheverFileHoldsItInEitherOrder(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	aaa, aMain := f.initRepo(t, "aaa")
	zzz, zMain := f.initRepo(t, "zzz")
	f.writeAndMerge(t, aaa, aMain, "a.md", "a")
	f.writeAndMerge(t, zzz, zMain, "z.md", "z")
	inA, err := f.manager.FetchBranch(f.ctx, FetchBranchRequest{RepoID: aaa.ID, BranchID: aMain.ID})
	if err != nil {
		t.Fatalf("FetchBranch in the first file: %v", err)
	}
	inZ, err := f.manager.FetchBranch(f.ctx, FetchBranchRequest{RepoID: zzz.ID, BranchID: zMain.ID})
	if err != nil {
		t.Fatalf("FetchBranch in the second file: %v", err)
	}
	for _, want := range []models.FetchBranchJob{inZ, inA, inZ, inA} {
		got, err := f.manager.GetFetchBranchStatus(f.ctx, want.ID)
		if err != nil || got.ID != want.ID || got.RepoID != want.RepoID {
			t.Fatalf("GetFetchBranchStatus(%s) = %+v, %v", want.ID, got, err)
		}
	}
}

func TestRoutedGitManager_AnUnknownFetchJobIdIsNotFoundInAnyFile(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	f.initRepo(t, "aaa")
	f.initRepo(t, "zzz")
	if _, err := f.manager.GetFetchBranchStatus(f.ctx, "no-such-job"); !errors.Is(err, ErrImportJobNotFound) {
		t.Fatalf("GetFetchBranchStatus of an unknown job = %v, want ErrImportJobNotFound", err)
	}
}
