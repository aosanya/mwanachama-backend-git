package mwanachamagit

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	gogitplumbing "github.com/go-git/go-git/v5/plumbing"
	gogitobject "github.com/go-git/go-git/v5/plumbing/object"
)

// pushOneCommit clones srvURL/repoName into a fresh temp working copy (or
// reuses workDir if it already has one, for a second push in the same
// test), writes path=content, commits it, and pushes refs/heads/main.
// Returns the new commit's SHA.
func pushOneCommit(t *testing.T, srvURL, repoName, workDir, path, content, message string) string {
	t.Helper()

	var repo *gogit.Repository
	if _, err := os.Stat(filepath.Join(workDir, ".git")); err == nil {
		var openErr error
		repo, openErr = gogit.PlainOpen(workDir)
		if openErr != nil {
			t.Fatalf("PlainOpen %s: %v", workDir, openErr)
		}
	} else {
		var initErr error
		repo, initErr = gogit.PlainInit(workDir, false)
		if initErr != nil {
			t.Fatalf("PlainInit %s: %v", workDir, initErr)
		}
		if _, err := repo.CreateRemote(&config.RemoteConfig{
			Name: "origin",
			URLs: []string{srvURL + "/" + repoName},
		}); err != nil {
			t.Fatalf("CreateRemote: %v", err)
		}
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, path), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
	if _, err := wt.Add(path); err != nil {
		t.Fatalf("Add %s: %v", path, err)
	}
	sha, err := wt.Commit(message, &gogit.CommitOptions{
		Author: &gogitobject.Signature{Name: "Test Author", Email: "author@example.com", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	refSpec := config.RefSpec(head.Name().String() + ":refs/heads/main")
	if err := repo.Push(&gogit.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{refSpec},
	}); err != nil {
		t.Fatalf("Push (refspec %s): %v", refSpec, err)
	}
	return sha.String()
}

// TestSmartHTTP_PushIndexesCommitTreeAndBlob drives a real git push (go-git
// client-side, over the real wire protocol) against SmartHTTPHandler and
// confirms IndexPushedBranch materialised a Commit row, a Branch row
// pointing at it, and a Blob row for the pushed file.
func TestSmartHTTP_PushIndexesCommitTreeAndBlob(t *testing.T) {
	m := newTestManager(t)
	srv := httptest.NewServer(m.SmartHTTPHandler())
	defer srv.Close()

	workDir := t.TempDir()
	sha := pushOneCommit(t, srv.URL, "widgets", workDir, "README.md", "hello\n", "first commit")

	ctx := context.Background()
	var commitCount int64
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).Where("sha = ?", sha).Count(&commitCount).Error; err != nil {
		t.Fatalf("count commits: %v", err)
	}
	if commitCount != 1 {
		t.Fatalf("expected exactly one Commit row for %s, got %d", sha, commitCount)
	}

	repo, err := m.GetRepositoryByName(ctx, "widgets")
	if err != nil {
		t.Fatalf("GetRepositoryByName: %v (repo should have been auto-created on first push)", err)
	}

	branches, err := m.ListBranches(ctx, repo.ID)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(branches) != 1 {
		t.Fatalf("expected exactly one branch (main, created by the push), got %d: %+v", len(branches), branches)
	}
	if branches[0].Name != "main" {
		t.Fatalf("expected branch named %q, got %q", "main", branches[0].Name)
	}
	if branches[0].SHA != sha {
		t.Fatalf("branch tip SHA = %q, want %q", branches[0].SHA, sha)
	}

	var blobCount int64
	if err := m.db.WithContext(ctx).Table(m.tables.Blobs).Where("path = ?", "README.md").Count(&blobCount).Error; err != nil {
		t.Fatalf("count blobs: %v", err)
	}
	if blobCount != 1 {
		t.Fatalf("expected exactly one Blob row for README.md, got %d", blobCount)
	}
}

// TestSmartHTTP_SecondPushIsIdempotentForUnchangedTree pushes a second
// commit that only adds a new file, and confirms the *unchanged* file from
// the first commit did not get a duplicate Blob row — proving
// upsertTreeIdempotent actually reuses rows rather than re-creating the
// whole tree on every push.
func TestSmartHTTP_SecondPushIsIdempotentForUnchangedTree(t *testing.T) {
	m := newTestManager(t)
	srv := httptest.NewServer(m.SmartHTTPHandler())
	defer srv.Close()

	workDir := t.TempDir()
	firstSHA := pushOneCommit(t, srv.URL, "widgets", workDir, "README.md", "hello\n", "first commit")
	secondSHA := pushOneCommit(t, srv.URL, "widgets", workDir, "second.md", "second file\n", "second commit")
	if firstSHA == secondSHA {
		t.Fatal("expected two distinct commits")
	}

	ctx := context.Background()
	var commitCount int64
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).Count(&commitCount).Error; err != nil {
		t.Fatalf("count commits: %v", err)
	}
	if commitCount != 2 {
		t.Fatalf("expected exactly two Commit rows total, got %d", commitCount)
	}

	var readmeBlobCount int64
	if err := m.db.WithContext(ctx).Table(m.tables.Blobs).Where("path = ?", "README.md").Count(&readmeBlobCount).Error; err != nil {
		t.Fatalf("count README blobs: %v", err)
	}
	if readmeBlobCount != 1 {
		t.Fatalf("README.md unchanged between pushes — expected exactly 1 Blob row, got %d (tree walk is not idempotent)", readmeBlobCount)
	}

	var secondBlobCount int64
	if err := m.db.WithContext(ctx).Table(m.tables.Blobs).Where("path = ?", "second.md").Count(&secondBlobCount).Error; err != nil {
		t.Fatalf("count second.md blobs: %v", err)
	}
	if secondBlobCount != 1 {
		t.Fatalf("expected exactly one Blob row for the newly-added second.md, got %d", secondBlobCount)
	}

	repo, err := m.GetRepositoryByName(ctx, "widgets")
	if err != nil {
		t.Fatalf("GetRepositoryByName: %v", err)
	}
	branches, err := m.ListBranches(ctx, repo.ID)
	if err != nil || len(branches) != 1 {
		t.Fatalf("ListBranches: %+v, err=%v", branches, err)
	}
	if branches[0].SHA != secondSHA {
		t.Fatalf("branch tip SHA = %q after second push, want %q", branches[0].SHA, secondSHA)
	}
}

// TestSmartHTTP_DeleteOnlyPushRemovesBranch pushes a non-default branch,
// then deletes it with `git push --delete` (a delete-only receive-pack,
// which carries no packfile at all — go-git's own ReceivePack fails on
// that, so this exercises receivePackDeletes specifically) and confirms
// the Branch row is gone.
func TestSmartHTTP_DeleteOnlyPushRemovesBranch(t *testing.T) {
	m := newTestManager(t)
	srv := httptest.NewServer(m.SmartHTTPHandler())
	defer srv.Close()

	workDir := t.TempDir()
	pushOneCommit(t, srv.URL, "widgets", workDir, "README.md", "hi\n", "first commit on main")

	repo, err := gogit.PlainOpen(workDir)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	featureRef := gogitplumbing.NewBranchReferenceName("feature")
	if err := wt.Checkout(&gogit.CheckoutOptions{Hash: head.Hash(), Branch: featureRef, Create: true}); err != nil {
		t.Fatalf("Checkout feature: %v", err)
	}
	if err := repo.Push(&gogit.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{config.RefSpec(featureRef + ":" + featureRef)},
	}); err != nil {
		t.Fatalf("push feature: %v", err)
	}

	ctx := context.Background()
	repoModel, err := m.GetRepositoryByName(ctx, "widgets")
	if err != nil {
		t.Fatalf("GetRepositoryByName: %v", err)
	}
	branches, err := m.ListBranches(ctx, repoModel.ID)
	if err != nil || len(branches) != 2 {
		t.Fatalf("expected 2 branches (main, feature) after the feature push, got %d: %+v (err=%v)", len(branches), branches, err)
	}

	// `git push --delete origin feature` — an empty source, deleting the
	// remote ref.
	if err := repo.Push(&gogit.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{config.RefSpec(":" + featureRef)},
	}); err != nil {
		t.Fatalf("delete push: %v", err)
	}

	branches, err = m.ListBranches(ctx, repoModel.ID)
	if err != nil {
		t.Fatalf("ListBranches after delete: %v", err)
	}
	if len(branches) != 1 || branches[0].Name != "main" {
		t.Fatalf("expected only 'main' to remain after deleting 'feature', got %+v", branches)
	}
}
