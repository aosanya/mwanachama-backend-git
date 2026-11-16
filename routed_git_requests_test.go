package mwanachamagit

import (
	"errors"
	"testing"
)

func TestRoutedGitManager_MergeRequestsAndTagsAreRoutedToTheirFile(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	_, _ = f.initRepo(t, "aaa")
	repo, main := f.initRepo(t, "zzz")
	work, err := f.manager.CreateBranch(f.ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "work", FromBranchID: main.ID})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if _, err := f.manager.WriteFile(f.ctx, WriteFileRequest{BranchID: work.ID, Path: "a.md", Content: "a"}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mr, err := f.manager.CreateMergeRequest(f.ctx, CreateMergeRequestRequest{RepositoryID: repo.ID, SourceBranchID: work.ID, Title: "work"})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}
	got, err := f.manager.GetMergeRequest(f.ctx, mr.ID)
	if err != nil || got.ID != mr.ID {
		t.Fatalf("GetMergeRequest = %+v, %v", got, err)
	}
	all, err := f.manager.ListMergeRequests(f.ctx, MergeRequestFilter{})
	if err != nil || len(all) != 1 {
		t.Fatalf("ListMergeRequests across files = %+v, %v; want the one", all, err)
	}
	scoped, err := f.manager.ListMergeRequests(f.ctx, MergeRequestFilter{RepositoryID: repo.ID})
	if err != nil || len(scoped) != 1 {
		t.Fatalf("ListMergeRequests of the repository = %+v, %v; want the one", scoped, err)
	}
	if _, err := f.manager.GetMergeRequest(f.ctx, "no-such-mr"); !errors.Is(err, ErrMergeRequestNotFound) {
		t.Fatalf("unknown merge request = %v, want ErrMergeRequestNotFound", err)
	}
	if _, err := f.manager.CloseMergeRequest(f.ctx, mr.ID); err != nil {
		t.Fatalf("CloseMergeRequest: %v", err)
	}
}
