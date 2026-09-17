// git_bug_g12_history_test.go pins board row G12 (todo.md): FetchBranch's
// (and by extension ImportRepo's, which auto-triggers FetchBranch on the
// default branch) commit walk creates Commit rows via walkCommitsOnly, but
// never writes the accompanying git_commit_parents join rows the way
// git_impl_push.go's linkCommitParents does for the push path. Log resolves
// history solely through gormstore.CommitChainIDs' recursive CTE over that
// join table, so an unlinked tip reports a one-commit history however many
// commits were actually indexed.
//
// This test builds a real 3-commit linear history (via the real git object
// model, go-git PlainInit/Commit — not fixture rows), imports it through
// the real ImportRepo → auto-FetchBranch path, and asserts the CURRENT
// (broken) behaviour: Log returns only 1 commit even though 3 were
// indexed. Once G12 is fixed (by reusing linkCommitParents from
// git_impl_push.go inside walkCommitsOnly), this assertion should change to
// len(history) == 3, newest-first, matching the fixed
// TestSmartHTTP_PushedHistoryIsWalkable test for the push path.
package mwanachamagit

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	gogitosfs "github.com/go-git/go-billy/v5/osfs"
	gogit "github.com/go-git/go-git/v5"
	gogitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/aosanya/mwanachama-backend-git/gormstore"
)

// makeLocalGitSourceWithCommits is [makeLocalGitSource] extended to write n
// sequential commits (file-1.txt, file-2.txt, ...) forming a real linear
// parent chain, then bare-clone the result the same way makeLocalGitSource
// does — go-git can clone a filesystem path directly, no network needed.
func makeLocalGitSourceWithCommits(t *testing.T, n int) string {
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
	for i := 1; i <= n; i++ {
		fname := "file-" + strconv.Itoa(i) + ".txt"
		f, err := srcFS.Create(fname)
		if err != nil {
			t.Fatalf("create %s: %v", fname, err)
		}
		if _, err := f.Write([]byte("content " + strconv.Itoa(i))); err != nil {
			_ = f.Close()
			t.Fatalf("write %s: %v", fname, err)
		}
		_ = f.Close()
		if _, err := wt.Add(fname); err != nil {
			t.Fatalf("git add %s: %v", fname, err)
		}
		if _, err := wt.Commit("commit "+strconv.Itoa(i), &gogit.CommitOptions{
			Author: &object.Signature{Name: "tester", Email: "t@t.com", When: time.Now()},
		}); err != nil {
			t.Fatalf("commit %d: %v", i, err)
		}
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

// TestFetchBranch_PinG12_HistoryUnreachableAfterImport pins the CURRENT
// broken behaviour: a real 3-commit history, imported end-to-end, reports
// only 1 commit via Log because walkCommitsOnly never links
// git_commit_parents. See this file's package doc for the fixed
// assertion.
func TestFetchBranch_PinG12_HistoryUnreachableAfterImport(t *testing.T) {
	bareDir := makeLocalGitSourceWithCommits(t, 3)
	ctx := context.Background()
	m, pub := newTestManagerWithPublisher(t)

	job, err := m.ImportRepo(ctx, ImportRepoRequest{Name: "g12-repo", SourceURL: bareDir, DefaultBranch: "master"})
	if err != nil {
		t.Fatalf("ImportRepo: %v", err)
	}

	deadline := time.After(10 * time.Second)
	var final = job
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
	if final.Status != importStatusCompleted {
		t.Fatalf("Import status = %q (err: %q), want completed", final.Status, final.ErrorMessage)
	}
	if !hasTopic(pub.published(), TopicRepoImported) {
		t.Errorf("expected TopicRepoImported published, got %v", pub.published())
	}

	repo, err := m.GetRepositoryByName(ctx, "g12-repo")
	if err != nil {
		t.Fatalf("GetRepositoryByName: %v", err)
	}

	// Poll until the default branch's auto-triggered fetch (runImport calls
	// FetchBranch synchronously, but the walk itself runs in a background
	// goroutine) reaches status=fetched.
	fetchDeadline := time.After(10 * time.Second)
	var branchID string
fetchPoll:
	for {
		select {
		case <-fetchDeadline:
			t.Fatalf("default branch never reached status=fetched")
		case <-time.After(50 * time.Millisecond):
			var branchRow gormstore.BranchRow
			if err := m.db.WithContext(ctx).Table(m.tables.Branches).
				Where("repository_id = ? AND name = ?", repo.ID, "master").First(&branchRow).Error; err != nil {
				continue
			}
			branchID = branchRow.ID
			if branchRow.Status == branchStatusFetched {
				break fetchPoll
			}
			if branchRow.Status == branchStatusFetchFailed {
				t.Fatalf("branch fetch failed: %v", branchRow.ErrorMessage)
			}
		}
	}

	// Confirm the walk actually reached and indexed all 3 commits as rows —
	// the bug is specifically that they're unlinked, not that they're
	// missing.
	var commitCount int64
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).Count(&commitCount).Error; err != nil {
		t.Fatalf("count commits: %v", err)
	}
	if commitCount != 3 {
		t.Fatalf("expected walkCommitsOnly to have created 3 Commit rows, got %d — fixture problem, not G12 itself", commitCount)
	}

	// The actual G12 assertion: Log only resolves history through
	// git_commit_parents, which walkCommitsOnly never populates.
	history, err := m.Log(ctx, branchID, LogFilter{})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("G12 REGRESSION-OR-FIX DETECTED: Log returned %d commits, want 1 (the pinned CURRENT broken behaviour). "+
			"If this is now 3, G12 has been fixed (walkCommitsOnly now links git_commit_parents, e.g. via "+
			"git_impl_push.go's linkCommitParents) — update this test to assert len(history) == 3, newest-first, "+
			"parents linked, and close G12 on the board.", len(history))
	}
	t.Logf("G12 pinned: real history has 3 commits, but Log(%s) reports only %d — git_commit_parents was never linked by walkCommitsOnly", branchID, len(history))
}
