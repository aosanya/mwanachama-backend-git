package mwanachamagit

import (
	"context"
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func openMergeRequestForGate(t *testing.T, m *gitManager) models.MergeRequest {
	t.Helper()
	ctx := context.Background()
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "gated"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	feature, err := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "feature/g"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	commit := createCommit(t, m, repo.ID, "feedface")
	if _, err := m.advanceBranchHead(ctx, feature.ID, commit.ID, ""); err != nil {
		t.Fatalf("advanceBranchHead: %v", err)
	}
	mr, err := m.CreateMergeRequest(ctx, CreateMergeRequestRequest{RepositoryID: repo.ID, Title: "t", SourceBranchID: feature.ID})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}
	return mr
}

func TestAMergeGateRefusesACallerItDoesNotAllowAndLeavesTheRequestOpen(t *testing.T) {
	m := newTestManager(t)
	var asked string
	m.merging = func(_ context.Context, caller, _, _ string) (bool, error) {
		asked = caller
		return caller == "accountable", nil
	}
	mr := openMergeRequestForGate(t, m)
	ctx := context.Background()

	if _, err := m.CompleteMergeRequest(ctx, mr.ID, "someone-else"); !errors.Is(err, ErrMergeNotPermitted) {
		t.Fatalf("want ErrMergeNotPermitted, got %v", err)
	}
	if asked != "someone-else" {
		t.Fatalf("gate asked about %q", asked)
	}
	still, err := m.GetMergeRequest(ctx, mr.ID)
	if err != nil || still.Status != models.MergeRequestStatusOpen {
		t.Fatalf("request should stay open, got %q %v", still.Status, err)
	}

	done, err := m.CompleteMergeRequest(ctx, mr.ID, "accountable")
	if err != nil {
		t.Fatalf("CompleteMergeRequest: %v", err)
	}
	if done.Status != models.MergeRequestStatusMerged || done.MergedBy != "accountable" {
		t.Fatalf("got status %q merged_by %q", done.Status, done.MergedBy)
	}
}

func TestAMergeGateIsAskedByADirectBranchMerge(t *testing.T) {
	m := newTestManager(t)
	m.merging = func(context.Context, string, string, string) (bool, error) { return false, nil }
	mr := openMergeRequestForGate(t, m)
	if _, err := m.MergeBranch(context.Background(), mr.SourceBranchID); !errors.Is(err, ErrMergeNotPermitted) {
		t.Fatalf("want ErrMergeNotPermitted, got %v", err)
	}
}

func TestNoMergeGateLeavesMergingOpen(t *testing.T) {
	m := newTestManager(t)
	mr := openMergeRequestForGate(t, m)
	done, err := m.CompleteMergeRequest(context.Background(), mr.ID, "anyone")
	if err != nil || done.Status != models.MergeRequestStatusMerged {
		t.Fatalf("got %q %v", done.Status, err)
	}
}
