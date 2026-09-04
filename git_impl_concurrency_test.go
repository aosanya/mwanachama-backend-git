// git_impl_concurrency_test.go — G9 fidelity port of CodeValdGit's
// git_concurrency_test.go and git_011_internal_test.go: GIT-011's
// advanceBranchHead CAS guard and RefLocker contract.
package mwanachamagit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/gormstore"
	"github.com/aosanya/mwanachama-backend-git/models"
)

// TestGIT011_ConcurrentMerges verifies that two goroutines merging different
// task branches concurrently both succeed with no lost update — the
// in-process mutexLocker serialises the two advance-head calls.
func TestGIT011_ConcurrentMerges(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "r"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branchA, err := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "concurrent-a"})
	if err != nil {
		t.Fatalf("CreateBranch a: %v", err)
	}
	branchB, err := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "concurrent-b"})
	if err != nil {
		t.Fatalf("CreateBranch b: %v", err)
	}
	writeTestFile(t, m, branchA.ID, "a.txt", "branch-a content")
	writeTestFile(t, m, branchB.ID, "b.txt", "branch-b content")

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, id := range []string{branchA.ID, branchB.ID} {
		wg.Add(1)
		go func(idx int, branchID string) {
			defer wg.Done()
			_, errs[idx] = m.MergeBranch(ctx, branchID)
		}(i, id)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d MergeBranch failed: %v", i, err)
		}
	}
}

// TestBUG09020_ConcurrentWriteFilesAllLand verifies the BUG-09-020 fix: when
// N goroutines fire WriteFile against the same branch in parallel, the
// RefLocker serialises them so every commit chains onto the
// previous one and every file ends up reachable from the branch HEAD.
// Without the lock, each goroutine reads the same parent HEAD, builds a
// sibling commit, and the unsynchronised advanceBranchHead leaves the
// branch tip pointing at only the last writer's commit — losing the rest.
func TestBUG09020_ConcurrentWriteFilesAllLand(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "r"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branch, err := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "bug09020-concurrent-writes"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	const n = 10
	paths := make([]string, n)
	for i := range paths {
		paths[i] = fmt.Sprintf("dir/file_%02d.txt", i)
	}

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = m.WriteFile(ctx, WriteFileRequest{
				BranchID: branch.ID, Path: paths[idx], Content: fmt.Sprintf("content-%02d", idx), AuthorName: "test-author",
			})
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("WriteFile %d failed: %v", i, err)
		}
	}

	for _, p := range paths {
		blob, err := m.ReadFile(ctx, branch.ID, p)
		if err != nil {
			t.Errorf("ReadFile %q after concurrent writes: %v", p, err)
			continue
		}
		if blob.Path != p {
			t.Errorf("ReadFile %q returned blob with Path=%q", p, blob.Path)
		}
	}
}

// TestGIT011_MergeLockRespectsContextCancellation verifies that
// WithMergeLock returns ctx.Err() when the context is cancelled before fn
// runs.
func TestGIT011_MergeLockRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	m := newTestManager(t)
	repo, err := m.InitRepo(context.Background(), CreateRepoRequest{Name: "r"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branch, err := m.CreateBranch(context.Background(), CreateBranchRequest{RepositoryID: repo.ID, Name: "ctx-cancel-001"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	writeTestFile(t, m, branch.ID, "file.txt", "content")

	_, err = m.MergeBranch(ctx, branch.ID)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// TestGIT011_AdvanceBranchHead_StaleHeadReturnsConflict verifies that
// advanceBranchHead returns ErrMergeConcurrencyConflict when the supplied
// expectedHeadCommitID does not match the branch's current head_commit_id.
func TestGIT011_AdvanceBranchHead_StaleHeadReturnsConflict(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	const actualHead = "commit-current"

	branchRow := gormstore.BranchToRow(models.Branch{Name: "stale-head", HeadCommitID: actualHead})
	if err := m.db.WithContext(ctx).Table(m.tables.Branches).Create(&branchRow).Error; err != nil {
		t.Fatalf("seed branch: %v", err)
	}
	commitRow := gormstore.CommitToRow(models.Commit{SHA: "abc123"})
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).Create(&commitRow).Error; err != nil {
		t.Fatalf("seed commit: %v", err)
	}

	_, err := m.advanceBranchHead(ctx, branchRow.ID, commitRow.ID, "commit-stale")
	if !errors.Is(err, ErrMergeConcurrencyConflict) {
		t.Fatalf("expected ErrMergeConcurrencyConflict, got %v", err)
	}
}

// TestGIT011_AdvanceBranchHead_MatchingHeadSucceeds verifies that
// advanceBranchHead succeeds when the supplied expectedHeadCommitID matches
// the branch's current head_commit_id.
func TestGIT011_AdvanceBranchHead_MatchingHeadSucceeds(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	const currentHead = "commit-current2"

	branchRow := gormstore.BranchToRow(models.Branch{Name: "matching-head", HeadCommitID: currentHead})
	if err := m.db.WithContext(ctx).Table(m.tables.Branches).Create(&branchRow).Error; err != nil {
		t.Fatalf("seed branch: %v", err)
	}
	commitRow := gormstore.CommitToRow(models.Commit{SHA: "def456"})
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).Create(&commitRow).Error; err != nil {
		t.Fatalf("seed commit: %v", err)
	}

	if _, err := m.advanceBranchHead(ctx, branchRow.ID, commitRow.ID, currentHead); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}
