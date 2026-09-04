// git_impl_rollback_extra_test.go — G9 fidelity port of scenarios from
// CodeValdGit's git_rollback_test.go not already covered by
// TestRollbackByWorkflowRun in git_impl_test.go: event-count assertions,
// merged-SHA preservation across rollback, the no-op-but-still-published
// case, and default-branch preservation when it's (unexpectedly) tagged
// with the rolled-back run.
package mwanachamagit

import (
	"context"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

// TestRollbackByWorkflowRun_PublishesExpectedEvents verifies that rolling
// back a run with 2 branches/MRs fires exactly 2 TopicMergeRolledBack events
// (one per MR) plus exactly 1 TopicWorkflowRunRolledBack summary event, and
// that a branch/MR from a different run is left untouched.
func TestRollbackByWorkflowRun_PublishesExpectedEvents(t *testing.T) {
	const targetRun = "wfr_rollback_target"
	const otherRun = "wfr_rollback_other"
	ctx := context.Background()
	m, pub := newTestManagerWithPublisher(t)
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "r"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	target1, err := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "feature/target-1", WorkflowRunID: targetRun})
	if err != nil {
		t.Fatalf("CreateBranch target-1: %v", err)
	}
	target2, err := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "feature/target-2", WorkflowRunID: targetRun})
	if err != nil {
		t.Fatalf("CreateBranch target-2: %v", err)
	}
	otherBranch, err := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "feature/other", WorkflowRunID: otherRun})
	if err != nil {
		t.Fatalf("CreateBranch other: %v", err)
	}

	target1MR, err := m.CreateMergeRequest(ctx, CreateMergeRequestRequest{RepositoryID: repo.ID, Title: "target-1", SourceBranchID: target1.ID, WorkflowRunID: targetRun})
	if err != nil {
		t.Fatalf("CreateMergeRequest target-1: %v", err)
	}
	target2MR, err := m.CreateMergeRequest(ctx, CreateMergeRequestRequest{RepositoryID: repo.ID, Title: "target-2", SourceBranchID: target2.ID, WorkflowRunID: targetRun})
	if err != nil {
		t.Fatalf("CreateMergeRequest target-2: %v", err)
	}
	otherMR, err := m.CreateMergeRequest(ctx, CreateMergeRequestRequest{RepositoryID: repo.ID, Title: "other", SourceBranchID: otherBranch.ID, WorkflowRunID: otherRun})
	if err != nil {
		t.Fatalf("CreateMergeRequest other: %v", err)
	}

	result, err := m.RollbackByWorkflowRun(ctx, targetRun)
	if err != nil {
		t.Fatalf("RollbackByWorkflowRun: %v", err)
	}
	if result.BranchesDeleted != 2 || result.MergeRequestsRolledBack != 2 {
		t.Errorf("result = %+v, want 2 branches + 2 MRs", result)
	}

	gotOther, err := m.GetMergeRequest(ctx, otherMR.ID)
	if err != nil {
		t.Fatalf("GetMergeRequest other: %v", err)
	}
	if gotOther.Status != models.MergeRequestStatusOpen {
		t.Errorf("other MR status = %q, want %q (untouched)", gotOther.Status, models.MergeRequestStatusOpen)
	}
	_ = target1MR
	_ = target2MR

	events := pub.published()
	mrEvents := countByTopic(events, TopicMergeRolledBack)
	summary := countByTopic(events, TopicWorkflowRunRolledBack)
	if mrEvents != 2 {
		t.Errorf("TopicMergeRolledBack count = %d, want 2", mrEvents)
	}
	if summary != 1 {
		t.Errorf("TopicWorkflowRunRolledBack count = %d, want 1", summary)
	}
}

// TestRollbackByWorkflowRun_PreservesMergedSHA verifies that MergeRequests
// previously marked "merged" keep their merged_commit_sha after being
// flipped to "rolled_back" — the value is audit-state, not lifecycle.
func TestRollbackByWorkflowRun_PreservesMergedSHA(t *testing.T) {
	const runID = "wfr_rollback_merged"
	ctx := context.Background()
	m := newTestManager(t)
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "r"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	source, err := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "feature/already-merged", WorkflowRunID: runID})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	mr, err := m.CreateMergeRequest(ctx, CreateMergeRequestRequest{RepositoryID: repo.ID, Title: "Merged MR", SourceBranchID: source.ID, WorkflowRunID: runID})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}

	// Directly transition to "merged" with a recorded SHA — skip MergeBranch,
	// which isn't the behavior under test here.
	if err := m.db.WithContext(ctx).Table(m.tables.MergeRequests).Where("id = ?", mr.ID).Updates(map[string]any{
		"status":            models.MergeRequestStatusMerged,
		"merged_commit_sha": "deadbeefdeadbeefdeadbeef",
	}).Error; err != nil {
		t.Fatalf("fake merge transition: %v", err)
	}

	if _, err := m.RollbackByWorkflowRun(ctx, runID); err != nil {
		t.Fatalf("RollbackByWorkflowRun: %v", err)
	}

	got, err := m.GetMergeRequest(ctx, mr.ID)
	if err != nil {
		t.Fatalf("GetMergeRequest: %v", err)
	}
	if got.Status != models.MergeRequestStatusRolledBack {
		t.Errorf("status = %q, want %q", got.Status, models.MergeRequestStatusRolledBack)
	}
	if got.MergedCommitSHA != "deadbeefdeadbeefdeadbeef" {
		t.Errorf("merged_commit_sha = %q, want preserved", got.MergedCommitSHA)
	}
}

// TestRollbackByWorkflowRun_NoOpWhenRunProducedNothing verifies that a run
// with no Git artifacts produces a zero-counter result and still fires the
// summary event (so the cross-service coordinator can mark the Git leg
// complete).
func TestRollbackByWorkflowRun_NoOpWhenRunProducedNothing(t *testing.T) {
	ctx := context.Background()
	m, pub := newTestManagerWithPublisher(t)
	if _, err := m.InitRepo(ctx, CreateRepoRequest{Name: "r"}); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	result, err := m.RollbackByWorkflowRun(ctx, "wfr_no_artifacts")
	if err != nil {
		t.Fatalf("RollbackByWorkflowRun: %v", err)
	}
	if result.BranchesDeleted != 0 || result.MergeRequestsRolledBack != 0 {
		t.Errorf("expected zero counters, got %+v", result)
	}
	events := pub.published()
	if countByTopic(events, TopicWorkflowRunRolledBack) != 1 {
		t.Errorf("expected summary event even on no-op; got %v", events)
	}
	if countByTopic(events, TopicMergeRolledBack) != 0 {
		t.Errorf("expected no TopicMergeRolledBack on no-op; got %v", events)
	}
}

// TestRollbackByWorkflowRun_PreservesDefaultBranch verifies that a default
// branch tagged with the rollback run-id (an unexpected but possible state)
// is preserved and surfaced via DefaultBranchesSkipped rather than deleted.
func TestRollbackByWorkflowRun_PreservesDefaultBranch(t *testing.T) {
	const runID = "wfr_rollback_default"
	ctx := context.Background()
	m := newTestManager(t)
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "r"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, _ := m.ListBranches(ctx, repo.ID)
	defaultBranch := branches[0]

	if err := m.db.WithContext(ctx).Table(m.tables.Branches).Where("id = ?", defaultBranch.ID).
		Update("workflow_run_id", runID).Error; err != nil {
		t.Fatalf("backfill default branch run_id: %v", err)
	}

	result, err := m.RollbackByWorkflowRun(ctx, runID)
	if err != nil {
		t.Fatalf("RollbackByWorkflowRun: %v", err)
	}
	if result.DefaultBranchesSkipped != 1 {
		t.Errorf("DefaultBranchesSkipped = %d, want 1", result.DefaultBranchesSkipped)
	}
	if result.BranchesDeleted != 0 {
		t.Errorf("BranchesDeleted = %d, want 0", result.BranchesDeleted)
	}
	if _, err := m.GetBranch(ctx, defaultBranch.ID); err != nil {
		t.Errorf("default branch deleted unexpectedly: %v", err)
	}
}
