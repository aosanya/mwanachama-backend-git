package mwanachamagit

import (
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

const (
	routedRolledBackRun = "wfr-rolled-back"
	routedOtherRun      = "wfr-other"
)

type routedRunFixture struct {
	firstMR   routedOpenMergeRequest
	secondMR  routedOpenMergeRequest
	firstOwn  models.Branch
	otherRun  models.Branch
	otherMain models.Branch
}

func routedRunInBothRepositories(t *testing.T, f *routedGitFixture) routedRunFixture {
	t.Helper()
	firstRepo, firstMain := f.initRepo(t, "aaa")
	secondRepo, secondMain := f.initRepo(t, "zzz")
	run := routedRunFixture{
		firstMR:   routedOpenMergeRequestIn(t, f, firstRepo, firstMain, "run-aaa", routedRolledBackRun),
		secondMR:  routedOpenMergeRequestIn(t, f, secondRepo, secondMain, "run-zzz", routedRolledBackRun),
		otherMain: secondMain,
	}
	var err error
	run.firstOwn, err = f.manager.CreateBranch(f.ctx, CreateBranchRequest{RepositoryID: firstRepo.ID, Name: "run-aaa-extra", FromBranchID: firstMain.ID, WorkflowRunID: routedRolledBackRun})
	if err != nil {
		t.Fatalf("CreateBranch run-aaa-extra: %v", err)
	}
	run.otherRun, err = f.manager.CreateBranch(f.ctx, CreateBranchRequest{RepositoryID: secondRepo.ID, Name: "other-zzz", FromBranchID: secondMain.ID, WorkflowRunID: routedOtherRun})
	if err != nil {
		t.Fatalf("CreateBranch other-zzz: %v", err)
	}
	return run
}

func TestRoutedGitManager_RollingBackAWorkflowRunSumsTheCountsOfEveryFile(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	routedRunInBothRepositories(t, f)
	got, err := f.manager.RollbackByWorkflowRun(f.ctx, routedRolledBackRun)
	if err != nil {
		t.Fatalf("RollbackByWorkflowRun: %v", err)
	}
	want := RollbackResult{WorkflowRunID: routedRolledBackRun, BranchesDeleted: 3, MergeRequestsRolledBack: 2, DefaultBranchesSkipped: 0}
	if got != want {
		t.Fatalf("RollbackByWorkflowRun = %+v, want %+v", got, want)
	}
}

func TestRoutedGitManager_RollingBackAWorkflowRunRemovesItsBranchesAndRollsBackItsMergeRequestsInBothFiles(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	run := routedRunInBothRepositories(t, f)
	if _, err := f.manager.RollbackByWorkflowRun(f.ctx, routedRolledBackRun); err != nil {
		t.Fatalf("RollbackByWorkflowRun: %v", err)
	}
	for _, branch := range []models.Branch{run.firstMR.source, run.secondMR.source, run.firstOwn} {
		if _, err := f.manager.GetBranch(f.ctx, branch.ID); !errors.Is(err, ErrBranchNotFound) {
			t.Errorf("GetBranch %s after rollback = %v, want ErrBranchNotFound", branch.Name, err)
		}
	}
	for _, mr := range []models.MergeRequest{run.firstMR.mr, run.secondMR.mr} {
		got, err := f.manager.GetMergeRequest(f.ctx, mr.ID)
		if err != nil || got.Status != models.MergeRequestStatusRolledBack {
			t.Errorf("merge request %s after rollback = %+v, %v; want rolled_back", mr.Title, got, err)
		}
	}
}

func TestRoutedGitManager_RollingBackAWorkflowRunLeavesOtherRunsAndDefaultBranchesAlone(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	run := routedRunInBothRepositories(t, f)
	if _, err := f.manager.RollbackByWorkflowRun(f.ctx, routedRolledBackRun); err != nil {
		t.Fatalf("RollbackByWorkflowRun: %v", err)
	}
	for _, branch := range []models.Branch{run.otherRun, run.otherMain, run.firstMR.main} {
		if got, err := f.manager.GetBranch(f.ctx, branch.ID); err != nil || got.ID != branch.ID {
			t.Errorf("GetBranch %s after rollback = %+v, %v; want it kept", branch.Name, got, err)
		}
	}
}

func TestRoutedGitManager_RollingBackTheSameWorkflowRunAgainChangesNothing(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	routedRunInBothRepositories(t, f)
	if _, err := f.manager.RollbackByWorkflowRun(f.ctx, routedRolledBackRun); err != nil {
		t.Fatalf("RollbackByWorkflowRun: %v", err)
	}
	again, err := f.manager.RollbackByWorkflowRun(f.ctx, routedRolledBackRun)
	if err != nil {
		t.Fatalf("second RollbackByWorkflowRun: %v", err)
	}
	if want := (RollbackResult{WorkflowRunID: routedRolledBackRun}); again != want {
		t.Fatalf("second RollbackByWorkflowRun = %+v, want %+v", again, want)
	}
}

func TestRoutedGitManager_RollingBackARunWithNoBranchesInAnyFileAnswersZeroCounts(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	routedRunInBothRepositories(t, f)
	got, err := f.manager.RollbackByWorkflowRun(f.ctx, "wfr-unknown")
	if err != nil {
		t.Fatalf("RollbackByWorkflowRun: %v", err)
	}
	if want := (RollbackResult{WorkflowRunID: "wfr-unknown"}); got != want {
		t.Fatalf("RollbackByWorkflowRun = %+v, want %+v", got, want)
	}
}

func TestRoutedGitManager_RollingBackWithNoWorkflowRunIdAnswersErrWorkflowRunIDRequired(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	routedRunInBothRepositories(t, f)
	if _, err := f.manager.RollbackByWorkflowRun(f.ctx, ""); !errors.Is(err, ErrWorkflowRunIDRequired) {
		t.Fatalf("RollbackByWorkflowRun with no id = %v, want ErrWorkflowRunIDRequired", err)
	}
}
