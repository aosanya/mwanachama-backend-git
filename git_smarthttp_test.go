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

	"github.com/aosanya/mwanachama-backend-git/gormstore"
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

// TestSmartHTTP_IdenticalContentAtTwoPathsKeepsBothFiles pushes two files
// with byte-identical content at different paths. They share one blob SHA,
// so a push that keyed its reuse-or-create check on SHA alone gave the
// second path no Blob row at all and left it unreadable — path is not a
// property of the content a SHA identifies.
func TestSmartHTTP_IdenticalContentAtTwoPathsKeepsBothFiles(t *testing.T) {
	m := newTestManager(t)
	srv := httptest.NewServer(m.SmartHTTPHandler())
	defer srv.Close()

	workDir := t.TempDir()
	repo, err := gogit.PlainInit(workDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{srv.URL + "/widgets"},
	}); err != nil {
		t.Fatalf("CreateRemote: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, "docs"), 0o755); err != nil {
		t.Fatalf("MkdirAll docs: %v", err)
	}
	for _, p := range []string{"LICENSE", "docs/LICENSE"} {
		if err := os.WriteFile(filepath.Join(workDir, p), []byte("same bytes\n"), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", p, err)
		}
		if _, err := wt.Add(p); err != nil {
			t.Fatalf("Add %s: %v", p, err)
		}
	}
	if _, err := wt.Commit("two identical files", &gogit.CommitOptions{
		Author: &gogitobject.Signature{Name: "Test Author", Email: "author@example.com", When: time.Now()},
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if err := repo.Push(&gogit.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{config.RefSpec(head.Name().String() + ":refs/heads/main")},
	}); err != nil {
		t.Fatalf("Push: %v", err)
	}

	ctx := context.Background()
	repoModel, err := m.GetRepositoryByName(ctx, "widgets")
	if err != nil {
		t.Fatalf("GetRepositoryByName: %v", err)
	}
	branches, err := m.ListBranches(ctx, repoModel.ID)
	if err != nil || len(branches) != 1 {
		t.Fatalf("ListBranches: %+v, err=%v", branches, err)
	}

	for _, path := range []string{"LICENSE", "docs/LICENSE"} {
		blob, err := m.ReadFile(ctx, branches[0].ID, path)
		if err != nil {
			t.Errorf("ReadFile(%q): %v — both paths hold the same bytes and both must be readable", path, err)
			continue
		}
		if blob.Path != path {
			t.Errorf("ReadFile(%q) returned a blob at path %q", path, blob.Path)
		}
	}

	var blobPaths []string
	if err := m.db.WithContext(ctx).Table(m.tables.Blobs).Order("path").Pluck("path", &blobPaths).Error; err != nil {
		t.Fatalf("list blob paths: %v", err)
	}
	if len(blobPaths) != 2 {
		t.Fatalf("expected one Blob row per path, got %d: %v", len(blobPaths), blobPaths)
	}
}

// TestSmartHTTP_PushedHistoryIsWalkable pushes three commits and confirms
// Log walks the whole chain back from the tip. Log resolves history through
// gormstore.CommitChainIDs' recursive CTE over git_commit_parents, so a push
// that materialises Commit rows without linking them reports a one-commit
// history — the rows exist but nothing can reach them.
func TestSmartHTTP_PushedHistoryIsWalkable(t *testing.T) {
	m := newTestManager(t)
	srv := httptest.NewServer(m.SmartHTTPHandler())
	defer srv.Close()

	workDir := t.TempDir()
	pushOneCommit(t, srv.URL, "widgets", workDir, "a.txt", "first\n", "first commit")
	pushOneCommit(t, srv.URL, "widgets", workDir, "b.txt", "second\n", "second commit")
	tipSHA := pushOneCommit(t, srv.URL, "widgets", workDir, "c.txt", "third\n", "third commit")

	ctx := context.Background()
	repo, err := m.GetRepositoryByName(ctx, "widgets")
	if err != nil {
		t.Fatalf("GetRepositoryByName: %v", err)
	}
	branches, err := m.ListBranches(ctx, repo.ID)
	if err != nil || len(branches) != 1 {
		t.Fatalf("ListBranches: %+v, err=%v", branches, err)
	}

	history, err := m.Log(ctx, branches[0].ID, LogFilter{})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("expected all 3 pushed commits in the branch history, got %d: %+v", len(history), history)
	}
	if history[0].SHA != tipSHA {
		t.Errorf("history[0].SHA = %q, want the tip %q (Log returns newest-first)", history[0].SHA, tipSHA)
	}
	wantMessages := []string{"third commit", "second commit", "first commit"}
	for i, want := range wantMessages {
		if history[i].Message != want {
			t.Errorf("history[%d].Message = %q, want %q", i, history[i].Message, want)
		}
	}
}

// TestSmartHTTP_MergeCommitRecordsBothParentsInOrder pushes a merge and
// confirms both parents are linked with git's own parent order preserved —
// ParentIndex 0 is the first parent, 1 the merged-in branch.
func TestSmartHTTP_MergeCommitRecordsBothParentsInOrder(t *testing.T) {
	m := newTestManager(t)
	srv := httptest.NewServer(m.SmartHTTPHandler())
	defer srv.Close()

	workDir := t.TempDir()
	pushOneCommit(t, srv.URL, "widgets", workDir, "base.txt", "base\n", "base commit")

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
	baseHash := head.Hash()

	// A side commit off the base, then a merge commit on the main line with
	// (mainTip, sideTip) as its parents, in that order.
	sideRef := gogitplumbing.NewBranchReferenceName("side")
	if err := wt.Checkout(&gogit.CheckoutOptions{Hash: baseHash, Branch: sideRef, Create: true}); err != nil {
		t.Fatalf("Checkout side: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "side.txt"), []byte("side\n"), 0o644); err != nil {
		t.Fatalf("WriteFile side.txt: %v", err)
	}
	if _, err := wt.Add("side.txt"); err != nil {
		t.Fatalf("Add side.txt: %v", err)
	}
	sideSHA, err := wt.Commit("side commit", &gogit.CommitOptions{
		Author: &gogitobject.Signature{Name: "Test Author", Email: "author@example.com", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("Commit side: %v", err)
	}

	if err := wt.Checkout(&gogit.CheckoutOptions{Hash: baseHash}); err != nil {
		t.Fatalf("Checkout back to base: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "main.txt"), []byte("main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile main.txt: %v", err)
	}
	if _, err := wt.Add("main.txt"); err != nil {
		t.Fatalf("Add main.txt: %v", err)
	}
	mainSHA, err := wt.Commit("main commit", &gogit.CommitOptions{
		Author: &gogitobject.Signature{Name: "Test Author", Email: "author@example.com", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("Commit main: %v", err)
	}
	// The merge result itself — base + main.txt + side.txt. go-git refuses a
	// commit with a clean worktree, so the merge has to actually carry the
	// side branch's file, which is what merging it in means anyway.
	if err := os.WriteFile(filepath.Join(workDir, "side.txt"), []byte("side\n"), 0o644); err != nil {
		t.Fatalf("WriteFile side.txt for merge: %v", err)
	}
	if _, err := wt.Add("side.txt"); err != nil {
		t.Fatalf("Add side.txt for merge: %v", err)
	}
	mergeSHA, err := wt.Commit("merge side into main", &gogit.CommitOptions{
		Author:  &gogitobject.Signature{Name: "Test Author", Email: "author@example.com", When: time.Now()},
		Parents: []gogitplumbing.Hash{mainSHA, sideSHA},
	})
	if err != nil {
		t.Fatalf("Commit merge: %v", err)
	}
	if err := repo.Push(&gogit.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{config.RefSpec(gogitplumbing.NewHash(mergeSHA.String()).String() + ":refs/heads/main")},
	}); err != nil {
		t.Fatalf("push merge: %v", err)
	}

	ctx := context.Background()
	var mergeRow gormstore.CommitRow
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).
		Where("sha = ?", mergeSHA.String()).First(&mergeRow).Error; err != nil {
		t.Fatalf("find merge commit row: %v", err)
	}

	type parentJoin struct {
		SHA         string
		ParentIndex int
	}
	var parents []parentJoin
	if err := m.db.WithContext(ctx).Table(m.tables.CommitParents+" AS cp").
		Select("c.sha AS sha, cp.parent_index AS parent_index").
		Joins("JOIN "+m.tables.Commits+" AS c ON c.id = cp.parent_id").
		Where("cp.commit_id = ?", mergeRow.ID).
		Order("cp.parent_index").
		Scan(&parents).Error; err != nil {
		t.Fatalf("read merge parents: %v", err)
	}
	if len(parents) != 2 {
		t.Fatalf("expected the merge commit to have 2 linked parents, got %d: %+v", len(parents), parents)
	}
	if parents[0].SHA != mainSHA.String() {
		t.Errorf("first parent (ParentIndex 0) = %q, want the main-line tip %q", parents[0].SHA, mainSHA)
	}
	if parents[1].SHA != sideSHA.String() {
		t.Errorf("second parent (ParentIndex 1) = %q, want the merged-in side tip %q", parents[1].SHA, sideSHA)
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
