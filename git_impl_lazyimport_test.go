// git_impl_lazyimport_test.go — G9 fidelity port of CodeValdGit's
// git_lazyimport_test.go: Lazy Import v2 (GIT-023h).
//
//   - ReadFile lazy path: ErrBlobContentUnavailable when the recorded local
//     clone path doesn't exist on disk.
//   - GetFetchBranchStatus: ErrImportJobNotFound for unknown job ID.
//   - ImportRepo end-to-end against a real local git repository (go-git can
//     clone a filesystem path directly, so this needs no network) — proves
//     the real gogit.PlainCloneContext + FetchBranch pulled forward in G5
//     actually completes and stub-imports correctly, not just that the
//     guard clauses return the right sentinel errors.
package mwanachamagit

import (
	"context"
	"errors"
	"testing"
	"time"

	gogitosfs "github.com/go-git/go-billy/v5/osfs"
	gogit "github.com/go-git/go-git/v5"
	gogitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/aosanya/mwanachama-backend-git/gormstore"
	"github.com/aosanya/mwanachama-backend-git/models"
)

// TestReadFile_LazyLoad_NoLocalClone verifies that ReadFile returns
// ErrBlobContentUnavailable when the Blob row exists (metadata only,
// content field empty) but the Repository's bare_clone_path doesn't exist
// on disk.
func TestReadFile_LazyLoad_NoLocalClone(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	now := models.NowRFC3339()

	repoRow := gormstore.RepositoryToRow(models.Repository{
		Name: "lazy-repo", DefaultBranch: "main", SourceURL: "file:///nonexistent",
		CreatedAt: now, UpdatedAt: now,
	})
	repoRow.BareClonePath = "/this/path/does/not/exist/on/disk"
	if err := m.db.WithContext(ctx).Table(m.tables.Repositories).Create(&repoRow).Error; err != nil {
		t.Fatalf("create Repository: %v", err)
	}

	branchRow := gormstore.BranchToRow(models.Branch{Name: "main", IsDefault: true, CreatedAt: now, UpdatedAt: now})
	branchRow.RepositoryID = gormstore.StringToNullable(repoRow.ID)
	branchRow.Status = "fetched"
	if err := m.db.WithContext(ctx).Table(m.tables.Branches).Create(&branchRow).Error; err != nil {
		t.Fatalf("create Branch: %v", err)
	}

	blobRow := gormstore.BlobToRow(models.Blob{
		SHA: "dddddddddddddddddddddddddddddddddddddddd", Path: "file.txt", Name: "file.txt",
		Extension: "txt", Size: 10, Encoding: "utf-8", Content: "", CreatedAt: now,
	})
	if err := m.db.WithContext(ctx).Table(m.tables.Blobs).Create(&blobRow).Error; err != nil {
		t.Fatalf("create Blob: %v", err)
	}

	treeRow := gormstore.TreeToRow(models.Tree{SHA: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", Path: "", CreatedAt: now})
	if err := m.db.WithContext(ctx).Table(m.tables.Trees).Create(&treeRow).Error; err != nil {
		t.Fatalf("create Tree: %v", err)
	}
	if err := m.db.WithContext(ctx).Table(m.tables.TreeBlobs).
		Create(&gormstore.TreeBlobRow{TreeID: treeRow.ID, BlobID: blobRow.ID}).Error; err != nil {
		t.Fatalf("link tree_blobs: %v", err)
	}

	commitRow := gormstore.CommitToRow(models.Commit{SHA: "ffffffffffffffffffffffffffffffffffffffff", Message: "stub commit", CreatedAt: now})
	commitRow.TreeID = gormstore.StringToNullable(treeRow.ID)
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).Create(&commitRow).Error; err != nil {
		t.Fatalf("create Commit: %v", err)
	}

	if err := m.db.WithContext(ctx).Table(m.tables.Branches).Where("id = ?", branchRow.ID).
		Update("head_commit_id", commitRow.ID).Error; err != nil {
		t.Fatalf("update branch HEAD: %v", err)
	}

	_, err := m.ReadFile(ctx, branchRow.ID, "file.txt")
	if !errors.Is(err, ErrBlobContentUnavailable) {
		t.Errorf("ReadFile (no local clone): got %v, want ErrBlobContentUnavailable", err)
	}
}

func TestGetFetchBranchStatus_NotFound(t *testing.T) {
	m := newTestManager(t)
	_, err := m.GetFetchBranchStatus(context.Background(), "nonexistent-job-id")
	if !errors.Is(err, ErrImportJobNotFound) {
		t.Errorf("GetFetchBranchStatus unknown ID: got %v, want ErrImportJobNotFound", err)
	}
}

// makeLocalGitSource creates a non-bare git repository in a temp directory
// with one commit (README.md), then pushes it into a bare repo (go-git
// shallow clone needs a real remote-shaped repo). Returns the bare repo's
// directory, which go-git can clone via a plain local path — no network,
// no file:// prefix needed.
func makeLocalGitSource(t *testing.T) string {
	t.Helper()
	srcDir := t.TempDir()

	repo, err := gogit.PlainInit(srcDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	srcFS := gogitosfs.New(srcDir)
	f, err := srcFS.Create("README.md")
	if err != nil {
		t.Fatalf("create README.md: %v", err)
	}
	if _, err := f.Write([]byte("# test")); err != nil {
		_ = f.Close()
		t.Fatalf("write README.md: %v", err)
	}
	_ = f.Close()
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := wt.Commit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{Name: "tester", Email: "t@t.com", When: time.Now()},
	}); err != nil {
		t.Fatalf("commit: %v", err)
	}

	bareDir := t.TempDir()
	bareRepo, err := gogit.PlainInit(bareDir, true)
	if err != nil {
		t.Fatalf("bare PlainInit: %v", err)
	}
	if _, err := bareRepo.CreateRemote(&gogitconfig.RemoteConfig{Name: "origin", URLs: []string{srcDir}}); err != nil {
		t.Fatalf("CreateRemote: %v", err)
	}
	if err := bareRepo.Fetch(&gogit.FetchOptions{
		RemoteName: "origin",
		RefSpecs:   []gogitconfig.RefSpec{"+refs/heads/*:refs/heads/*"},
	}); err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		t.Fatalf("Fetch into bare: %v", err)
	}
	return bareDir
}

// TestImportRepo_LocalGitSource_EndToEnd verifies, against a real local git
// repository (no network): ImportRepo completes within 10s, only a stub
// Branch row is written by Phase 1 (no Commit/Tree/Blob yet — those come
// from the auto-triggered FetchBranch, which then completes and materialises
// them), a completion event fires, and the imported file is readable.
func TestImportRepo_LocalGitSource_EndToEnd(t *testing.T) {
	bareDir := makeLocalGitSource(t)
	ctx := context.Background()
	m, pub := newTestManagerWithPublisher(t)

	start := time.Now()
	// gogit.PlainInit defaults new repos to branch "master", not "main" —
	// match makeLocalGitSource's actual default branch.
	job, err := m.ImportRepo(ctx, ImportRepoRequest{Name: "stub-test", SourceURL: bareDir, DefaultBranch: "master"})
	if err != nil {
		t.Fatalf("ImportRepo: %v", err)
	}
	if job.ID == "" {
		t.Fatal("ImportRepo returned empty job ID")
	}

	deadline := time.After(10 * time.Second)
	var final models.ImportJob
poll:
	for {
		select {
		case <-deadline:
			t.Fatalf("ImportRepo did not complete within 10s (last status: %q)", final.Status)
		case <-time.After(50 * time.Millisecond):
			j, err := m.GetImportStatus(ctx, job.ID)
			if err != nil {
				t.Fatalf("GetImportStatus: %v", err)
			}
			final = j
			if j.Status == importStatusCompleted || j.Status == importStatusFailed {
				break poll
			}
		}
	}
	if elapsed := time.Since(start); elapsed >= 10*time.Second {
		t.Errorf("ImportRepo took %v, want < 10s", elapsed)
	}
	if final.Status != importStatusCompleted {
		t.Fatalf("Import status = %q (err: %q), want completed", final.Status, final.ErrorMessage)
	}

	if !hasTopic(pub.published(), TopicRepoImported) {
		t.Errorf("expected TopicRepoImported published, got %v", pub.published())
	}

	var branches []gormstore.BranchRow
	if err := m.db.WithContext(ctx).Table(m.tables.Branches).Find(&branches).Error; err != nil || len(branches) == 0 {
		t.Fatalf("no Branch rows found after import (err=%v)", err)
	}

	// The default branch's auto-fetch was triggered synchronously by
	// runImport; poll until it clears "fetching"/"stub" (background
	// goroutine) so we can assert the README is actually readable.
	var repos []gormstore.RepositoryRow
	if err := m.db.WithContext(ctx).Table(m.tables.Repositories).Find(&repos).Error; err != nil || len(repos) != 1 {
		t.Fatalf("expected exactly one Repository, got %d (err=%v)", len(repos), err)
	}
	repoID := repos[0].ID

	fetchDeadline := time.After(10 * time.Second)
	var mainBranch models.Branch
fetchPoll:
	for {
		select {
		case <-fetchDeadline:
			t.Fatalf("default branch never reached status=fetched (last=%q)", mainBranch.Name)
		case <-time.After(50 * time.Millisecond):
			bs, err := m.ListBranches(ctx, repoID)
			if err != nil {
				t.Fatalf("ListBranches: %v", err)
			}
			for _, b := range bs {
				if b.Name != "master" {
					continue
				}
				mainBranch = b
				var branchRow gormstore.BranchRow
				if err := m.db.WithContext(ctx).Table(m.tables.Branches).Where("id = ?", b.ID).First(&branchRow).Error; err != nil {
					continue
				}
				if branchRow.Status == branchStatusFetched {
					break fetchPoll
				}
				if branchRow.Status == branchStatusFetchFailed {
					t.Fatalf("branch fetch failed: %v", branchRow.ErrorMessage)
				}
			}
		}
	}

	blob, err := m.ReadFile(ctx, mainBranch.ID, "README.md")
	if err != nil {
		t.Fatalf("ReadFile README.md after fetch: %v", err)
	}
	if blob.Path != "README.md" {
		t.Errorf("blob.Path = %q, want README.md", blob.Path)
	}
}
