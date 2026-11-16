package mwanachamagit

import (
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

type routedOpenMergeRequest struct {
	repo   models.Repository
	main   models.Branch
	source models.Branch
	mr     models.MergeRequest
}

func routedOpenMergeRequestIn(t *testing.T, f *routedGitFixture, repo models.Repository, main models.Branch, branchName, workflowRunID string) routedOpenMergeRequest {
	t.Helper()
	source, err := f.manager.CreateBranch(f.ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: branchName, FromBranchID: main.ID, WorkflowRunID: workflowRunID})
	if err != nil {
		t.Fatalf("CreateBranch %s: %v", branchName, err)
	}
	if _, err := f.manager.WriteFile(f.ctx, WriteFileRequest{BranchID: source.ID, Path: branchName + ".md", Content: branchName}); err != nil {
		t.Fatalf("WriteFile %s: %v", branchName, err)
	}
	mr, err := f.manager.CreateMergeRequest(f.ctx, CreateMergeRequestRequest{RepositoryID: repo.ID, SourceBranchID: source.ID, Title: branchName, WorkflowRunID: workflowRunID})
	if err != nil {
		t.Fatalf("CreateMergeRequest %s: %v", branchName, err)
	}
	return routedOpenMergeRequest{repo: repo, main: main, source: source, mr: mr}
}

func routedOpenMergeRequestInEachRepository(t *testing.T, f *routedGitFixture) (first, second routedOpenMergeRequest) {
	t.Helper()
	firstRepo, firstMain := f.initRepo(t, "aaa")
	secondRepo, secondMain := f.initRepo(t, "zzz")
	return routedOpenMergeRequestIn(t, f, firstRepo, firstMain, "work-aaa", ""), routedOpenMergeRequestIn(t, f, secondRepo, secondMain, "work-zzz", "")
}

func TestRoutedGitManager_AMergeRequestInTheSecondFileIsCompletedAndMergedIntoThatFilesDefaultBranchOnly(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	first, second := routedOpenMergeRequestInEachRepository(t, f)
	done, err := f.manager.CompleteMergeRequest(f.ctx, second.mr.ID, "merger")
	if err != nil {
		t.Fatalf("CompleteMergeRequest: %v", err)
	}
	if done.ID != second.mr.ID || done.Status != models.MergeRequestStatusMerged || done.MergedBy != "merger" {
		t.Fatalf("completed = %+v, want merged by merger", done)
	}
	blob, err := f.manager.ReadFile(f.ctx, second.main.ID, "work-zzz.md")
	if err != nil || blob.Content != "work-zzz" {
		t.Fatalf("second repository's default branch file = %+v, %v", blob, err)
	}
	if _, err := f.manager.ReadFile(f.ctx, first.main.ID, "work-zzz.md"); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("first repository reading the second's merged file = %v, want ErrFileNotFound", err)
	}
	stillOpen, err := f.manager.GetMergeRequest(f.ctx, first.mr.ID)
	if err != nil || stillOpen.Status != models.MergeRequestStatusOpen {
		t.Fatalf("first repository's merge request = %+v, %v; want open", stillOpen, err)
	}
}

func TestRoutedGitManager_AMergeRequestInTheFirstFileIsCompletedAndTheSecondFilesStaysOpen(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	first, second := routedOpenMergeRequestInEachRepository(t, f)
	done, err := f.manager.CompleteMergeRequest(f.ctx, first.mr.ID, "")
	if err != nil || done.ID != first.mr.ID || done.Status != models.MergeRequestStatusMerged {
		t.Fatalf("CompleteMergeRequest = %+v, %v", done, err)
	}
	blob, err := f.manager.ReadFile(f.ctx, first.main.ID, "work-aaa.md")
	if err != nil || blob.Content != "work-aaa" {
		t.Fatalf("first repository's default branch file = %+v, %v", blob, err)
	}
	stillOpen, err := f.manager.GetMergeRequest(f.ctx, second.mr.ID)
	if err != nil || stillOpen.Status != models.MergeRequestStatusOpen {
		t.Fatalf("second repository's merge request = %+v, %v; want open", stillOpen, err)
	}
}

func TestRoutedGitManager_CompletingAMergeRequestTwiceAnswersErrMergeRequestNotOpen(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	_, second := routedOpenMergeRequestInEachRepository(t, f)
	if _, err := f.manager.CompleteMergeRequest(f.ctx, second.mr.ID, ""); err != nil {
		t.Fatalf("CompleteMergeRequest: %v", err)
	}
	if _, err := f.manager.CompleteMergeRequest(f.ctx, second.mr.ID, ""); !errors.Is(err, ErrMergeRequestNotOpen) {
		t.Fatalf("second CompleteMergeRequest = %v, want ErrMergeRequestNotOpen", err)
	}
}

func TestRoutedGitManager_CompletingAnUnknownMergeRequestAnswersErrMergeRequestNotFound(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	routedOpenMergeRequestInEachRepository(t, f)
	if _, err := f.manager.CompleteMergeRequest(f.ctx, "no-such-mr", ""); !errors.Is(err, ErrMergeRequestNotFound) {
		t.Fatalf("CompleteMergeRequest unknown = %v, want ErrMergeRequestNotFound", err)
	}
}

func TestRoutedGitManager_ClosingAMergeRequestInTheSecondFileLeavesTheFirstFilesOpen(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	first, second := routedOpenMergeRequestInEachRepository(t, f)
	closed, err := f.manager.CloseMergeRequest(f.ctx, second.mr.ID)
	if err != nil || closed.Status != models.MergeRequestStatusClosed {
		t.Fatalf("CloseMergeRequest = %+v, %v", closed, err)
	}
	if _, err := f.manager.CompleteMergeRequest(f.ctx, second.mr.ID, ""); !errors.Is(err, ErrMergeRequestNotOpen) {
		t.Fatalf("CompleteMergeRequest of a closed one = %v, want ErrMergeRequestNotOpen", err)
	}
	stillOpen, err := f.manager.GetMergeRequest(f.ctx, first.mr.ID)
	if err != nil || stillOpen.Status != models.MergeRequestStatusOpen {
		t.Fatalf("first repository's merge request = %+v, %v; want open", stillOpen, err)
	}
}

func TestRoutedGitManager_MergeRequestsListedAcrossFilesAreCutToTheLimit(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	firstRepo, firstMain := f.initRepo(t, "aaa")
	secondRepo, secondMain := f.initRepo(t, "zzz")
	routedOpenMergeRequestIn(t, f, firstRepo, firstMain, "one", "")
	routedOpenMergeRequestIn(t, f, firstRepo, firstMain, "two", "")
	routedOpenMergeRequestIn(t, f, secondRepo, secondMain, "three", "")
	routedOpenMergeRequestIn(t, f, secondRepo, secondMain, "four", "")

	all, err := f.manager.ListMergeRequests(f.ctx, MergeRequestFilter{})
	if err != nil || len(all) != 4 {
		t.Fatalf("ListMergeRequests with no limit = %d, %v; want 4", len(all), err)
	}
	cut, err := f.manager.ListMergeRequests(f.ctx, MergeRequestFilter{Limit: 3})
	if err != nil || len(cut) != 3 {
		t.Fatalf("ListMergeRequests with limit 3 = %d, %v; want 3", len(cut), err)
	}
	reachesSecondFile := false
	for _, mr := range cut {
		reachesSecondFile = reachesSecondFile || mr.RepositoryID == secondRepo.ID
	}
	if !reachesSecondFile {
		t.Fatalf("limit 3 over 2+2 merge requests = %+v, want one from the second file", cut)
	}
	one, err := f.manager.ListMergeRequests(f.ctx, MergeRequestFilter{Limit: 1})
	if err != nil || len(one) != 1 {
		t.Fatalf("ListMergeRequests with limit 1 = %d, %v; want 1", len(one), err)
	}
}

func TestRoutedGitManager_MergeRequestsListedAcrossFilesCanBeFilteredByStatus(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	first, second := routedOpenMergeRequestInEachRepository(t, f)
	if _, err := f.manager.CompleteMergeRequest(f.ctx, second.mr.ID, ""); err != nil {
		t.Fatalf("CompleteMergeRequest: %v", err)
	}
	merged, err := f.manager.ListMergeRequests(f.ctx, MergeRequestFilter{Status: models.MergeRequestStatusMerged})
	if err != nil || len(merged) != 1 || merged[0].ID != second.mr.ID {
		t.Fatalf("merged merge requests = %+v, %v; want only the second's", merged, err)
	}
	open, err := f.manager.ListMergeRequests(f.ctx, MergeRequestFilter{Status: models.MergeRequestStatusOpen})
	if err != nil || len(open) != 1 || open[0].ID != first.mr.ID {
		t.Fatalf("open merge requests = %+v, %v; want only the first's", open, err)
	}
}
