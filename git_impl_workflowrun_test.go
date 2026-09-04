// git_impl_workflowrun_test.go — G9 fidelity port of CodeValdGit's
// git_workflowrun_test.go: workflow_run_id persistence on Branch, the
// MergeRequest lifecycle, and the associated git.merge.* event publishing.
package mwanachamagit

import (
	"context"
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func TestGitManager_CreateBranch_PersistsWorkflowRunID(t *testing.T) {
	const runID = "wfr_branch_001"
	ctx := context.Background()
	m := newTestManager(t)
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "r"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	created, err := m.CreateBranch(ctx, CreateBranchRequest{
		RepositoryID: repo.ID, Name: "feature/run-001", WorkflowRunID: runID,
	})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if created.WorkflowRunID != runID {
		t.Errorf("created.WorkflowRunID = %q, want %q", created.WorkflowRunID, runID)
	}

	got, err := m.GetBranch(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetBranch: %v", err)
	}
	if got.WorkflowRunID != runID {
		t.Errorf("GetBranch.WorkflowRunID = %q, want %q", got.WorkflowRunID, runID)
	}
}

func TestGitManager_ListBranchesFiltered_ByWorkflowRunID(t *testing.T) {
	const runA = "wfr_filter_A"
	const runB = "wfr_filter_B"
	ctx := context.Background()
	m := newTestManager(t)
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "r"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	for _, b := range []struct{ name, runID string }{
		{"feature/a-1", runA}, {"feature/a-2", runA}, {"feature/b-1", runB}, {"feature/no-run", ""},
	} {
		if _, err := m.CreateBranch(ctx, CreateBranchRequest{
			RepositoryID: repo.ID, Name: b.name, WorkflowRunID: b.runID,
		}); err != nil {
			t.Fatalf("CreateBranch(%s): %v", b.name, err)
		}
	}

	got, err := m.ListBranchesFiltered(ctx, repo.ID, BranchFilter{WorkflowRunID: runA})
	if err != nil {
		t.Fatalf("ListBranchesFiltered: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len(filtered) = %d, want 2", len(got))
	}
	for _, b := range got {
		if b.WorkflowRunID != runA {
			t.Errorf("filtered branch %q has WorkflowRunID = %q, want %q", b.Name, b.WorkflowRunID, runA)
		}
	}

	all, err := m.ListBranchesFiltered(ctx, repo.ID, BranchFilter{})
	if err != nil {
		t.Fatalf("ListBranchesFiltered (no filter): %v", err)
	}
	if len(all) != 5 { // default + 4 created above
		t.Errorf("len(all) = %d, want 5", len(all))
	}
}

func TestGitManager_CreateMergeRequest_PublishesAndPersists(t *testing.T) {
	const runID = "wfr_mr_001"
	ctx := context.Background()
	m, pub := newTestManagerWithPublisher(t)
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "r"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	source, err := m.CreateBranch(ctx, CreateBranchRequest{
		RepositoryID: repo.ID, Name: "feature/mr-source", WorkflowRunID: runID,
	})
	if err != nil {
		t.Fatalf("CreateBranch source: %v", err)
	}

	mr, err := m.CreateMergeRequest(ctx, CreateMergeRequestRequest{
		RepositoryID: repo.ID, Title: "Add widget", SourceBranchID: source.ID, WorkflowRunID: runID,
	})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}
	if mr.Status != models.MergeRequestStatusOpen {
		t.Errorf("mr.Status = %q, want %q", mr.Status, models.MergeRequestStatusOpen)
	}
	if mr.WorkflowRunID != runID {
		t.Errorf("mr.WorkflowRunID = %q, want %q", mr.WorkflowRunID, runID)
	}

	got, err := m.GetMergeRequest(ctx, mr.ID)
	if err != nil {
		t.Fatalf("GetMergeRequest: %v", err)
	}
	if got.Title != "Add widget" || got.WorkflowRunID != runID {
		t.Errorf("GetMergeRequest = %+v, want title=Add widget run_id=%s", got, runID)
	}

	if !hasTopic(pub.published(), TopicMergeRequested) {
		t.Errorf("expected %q published, got %v", TopicMergeRequested, pub.published())
	}
}

func TestGitManager_ListMergeRequests_FilterByWorkflowRunID(t *testing.T) {
	const runA = "wfr_listmr_A"
	const runB = "wfr_listmr_B"
	ctx := context.Background()
	m := newTestManager(t)
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "r"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	sourceA, _ := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "feature/list-A"})
	sourceB, _ := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "feature/list-B"})

	if _, err := m.CreateMergeRequest(ctx, CreateMergeRequestRequest{
		RepositoryID: repo.ID, Title: "A", SourceBranchID: sourceA.ID, WorkflowRunID: runA,
	}); err != nil {
		t.Fatalf("CreateMergeRequest A: %v", err)
	}
	if _, err := m.CreateMergeRequest(ctx, CreateMergeRequestRequest{
		RepositoryID: repo.ID, Title: "B", SourceBranchID: sourceB.ID, WorkflowRunID: runB,
	}); err != nil {
		t.Fatalf("CreateMergeRequest B: %v", err)
	}

	got, err := m.ListMergeRequests(ctx, MergeRequestFilter{WorkflowRunID: runA})
	if err != nil {
		t.Fatalf("ListMergeRequests: %v", err)
	}
	if len(got) != 1 || got[0].WorkflowRunID != runA {
		t.Errorf("filtered MRs = %+v, want one MR with run %q", got, runA)
	}
}

func TestGitManager_CloseMergeRequest_TransitionsToClosed(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "r"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	source, _ := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "feature/close-me"})
	mr, err := m.CreateMergeRequest(ctx, CreateMergeRequestRequest{
		RepositoryID: repo.ID, Title: "Close test", SourceBranchID: source.ID,
	})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}

	closed, err := m.CloseMergeRequest(ctx, mr.ID)
	if err != nil {
		t.Fatalf("CloseMergeRequest: %v", err)
	}
	if closed.Status != models.MergeRequestStatusClosed {
		t.Errorf("closed.Status = %q, want %q", closed.Status, models.MergeRequestStatusClosed)
	}

	if _, err := m.CloseMergeRequest(ctx, mr.ID); !errors.Is(err, ErrMergeRequestNotOpen) {
		t.Errorf("second CloseMergeRequest: got %v, want ErrMergeRequestNotOpen", err)
	}
}

// TestGitManager_Publisher verifies InitRepo publishes TopicRepoCreated —
// the event side of an operation the fake-DataManager tests otherwise only
// check via return values.
func TestGitManager_Publisher(t *testing.T) {
	m, pub := newTestManagerWithPublisher(t)

	if _, err := m.InitRepo(context.Background(), CreateRepoRequest{Name: "test-repo"}); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	events := pub.published()
	if len(events) != 1 {
		t.Fatalf("published events: got %d, want 1 (%+v)", len(events), events)
	}
	if events[0].topic != TopicRepoCreated {
		t.Errorf("event.topic = %q, want %q", events[0].topic, TopicRepoCreated)
	}
}
