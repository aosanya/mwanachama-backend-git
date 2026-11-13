package mwanachamagit

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

const (
	conflictPath = "notes/shared.txt"
	conflictBase = "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\n"
)

func conflictOpen(t *testing.T, pub *fakePublisher) (*RepoFile, models.Repository, models.Branch) {
	t.Helper()
	s, err := LoadSpec(filepath.Join("spec", "examples", "engineering.git.json"))
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	var publisher interface {
		Publish(ctx context.Context, topic string, payload any) error
	}
	if pub != nil {
		publisher = pub
	}
	rf, err := OpenRepoFile(filepath.Join(t.TempDir(), "conflict.db"), s, publisher, nil, nil)
	if err != nil {
		t.Fatalf("OpenRepoFile: %v", err)
	}
	t.Cleanup(func() { _ = rf.Close() })
	ctx := context.Background()
	repo, err := rf.InitRepo(ctx, CreateRepoRequest{Name: featureRepoName})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, err := rf.ListBranches(ctx, repo.ID)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(branches) == 0 {
		t.Fatalf("repository %s has no default branch", repo.Name)
	}
	return rf, repo, branches[0]
}

func conflictBranchFrom(t *testing.T, rf *RepoFile, repoID, fromID, name string) models.Branch {
	t.Helper()
	branch, err := rf.CreateBranch(context.Background(), CreateBranchRequest{
		RepositoryID: repoID,
		Name:         name,
		FromBranchID: fromID,
	})
	if err != nil {
		t.Fatalf("CreateBranch %s: %v", name, err)
	}
	return branch
}

func conflictWrite(t *testing.T, rf *RepoFile, branchID, path, content string) {
	t.Helper()
	writeFeatureFiles(t, rf, branchID, []featureFile{{Path: path, Content: content}})
}

func conflictRead(t *testing.T, rf *RepoFile, branchID, path string) string {
	t.Helper()
	blob, err := rf.ReadFile(context.Background(), branchID, path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	return blob.Content
}

func conflictHead(t *testing.T, rf *RepoFile, branchID string) string {
	t.Helper()
	branch, err := rf.GetBranch(context.Background(), branchID)
	if err != nil {
		t.Fatalf("GetBranch: %v", err)
	}
	return branch.HeadCommitID
}

func conflictReplaceLine(content string, line int, text string) string {
	lines := strings.Split(content, "\n")
	lines[line-1] = text
	return strings.Join(lines, "\n")
}

func conflictDiverged(t *testing.T, pub *fakePublisher) (*RepoFile, models.Repository, models.Branch, models.Branch) {
	t.Helper()
	rf, repo, def := conflictOpen(t, pub)
	conflictWrite(t, rf, def.ID, conflictPath, conflictBase)
	feature := conflictBranchFrom(t, rf, repo.ID, def.ID, "feature/a")
	conflictWrite(t, rf, feature.ID, conflictPath, conflictReplaceLine(conflictBase, 2, "from branch"))
	conflictWrite(t, rf, def.ID, conflictPath, conflictReplaceLine(conflictBase, 2, "from default"))
	return rf, repo, def, feature
}

func conflictFiles(t *testing.T, err error) []string {
	t.Helper()
	var mc *ErrMergeConflict
	if !errors.As(err, &mc) {
		t.Fatalf("expected *ErrMergeConflict, got %v", err)
	}
	return mc.ConflictingFiles
}

func TestRepoFileFeatures_Conflict_SameLineEditReturnsMergeConflict(t *testing.T) {
	rf, _, def, feature := conflictDiverged(t, nil)
	headBefore := conflictHead(t, rf, def.ID)

	_, err := rf.MergeBranch(context.Background(), feature.ID)

	files := conflictFiles(t, err)
	found := false
	for _, f := range files {
		if f == conflictPath {
			found = true
		}
	}
	if !found {
		t.Fatalf("ConflictingFiles = %v, want it to name %s", files, conflictPath)
	}
	if got := conflictHead(t, rf, def.ID); got != headBefore {
		t.Fatalf("default head moved from %s to %s despite the conflict", headBefore, got)
	}
	if got := conflictRead(t, rf, def.ID, conflictPath); got != conflictReplaceLine(conflictBase, 2, "from default") {
		t.Fatalf("default content changed by the failed merge: %q", got)
	}
}

func TestRepoFileFeatures_Conflict_NonOverlappingEditsMergeCleanly(t *testing.T) {
	rf, repo, def := conflictOpen(t, nil)
	conflictWrite(t, rf, def.ID, conflictPath, conflictBase)
	feature := conflictBranchFrom(t, rf, repo.ID, def.ID, "feature/a")
	conflictWrite(t, rf, feature.ID, conflictPath, conflictReplaceLine(conflictBase, 1, "top from branch"))
	conflictWrite(t, rf, def.ID, conflictPath, conflictReplaceLine(conflictBase, 9, "bottom from default"))

	if _, err := rf.MergeBranch(context.Background(), feature.ID); err != nil {
		t.Fatalf("MergeBranch of non-overlapping edits: %v", err)
	}

	got := conflictRead(t, rf, def.ID, conflictPath)
	if !strings.Contains(got, "top from branch") {
		t.Fatalf("merged file lost the branch edit: %q", got)
	}
	if !strings.Contains(got, "bottom from default") {
		t.Fatalf("merged file lost the default edit: %q", got)
	}
}

func TestRepoFileFeatures_Conflict_MergeRequestFailsAndPublishesFailure(t *testing.T) {
	pub := &fakePublisher{}
	rf, repo, def, feature := conflictDiverged(t, pub)
	ctx := context.Background()
	mr, err := rf.CreateMergeRequest(ctx, CreateMergeRequestRequest{
		RepositoryID:   repo.ID,
		Title:          "conflicting change",
		SourceBranchID: feature.ID,
	})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}
	headBefore := conflictHead(t, rf, def.ID)

	failed, err := rf.CompleteMergeRequest(ctx, mr.ID, "")

	if err == nil {
		t.Fatal("CompleteMergeRequest of a conflicting branch returned no error")
	}
	conflictFiles(t, err)
	if failed.Status != models.MergeRequestStatusFailed {
		t.Fatalf("returned status = %q, want %q", failed.Status, models.MergeRequestStatusFailed)
	}
	stored, getErr := rf.GetMergeRequest(ctx, mr.ID)
	if getErr != nil {
		t.Fatalf("GetMergeRequest: %v", getErr)
	}
	if stored.Status != models.MergeRequestStatusFailed {
		t.Fatalf("stored status = %q, want %q", stored.Status, models.MergeRequestStatusFailed)
	}
	events := pub.published()
	if countByTopic(events, TopicMergeFailed) != 1 {
		t.Fatalf("TopicMergeFailed published %d times, want 1", countByTopic(events, TopicMergeFailed))
	}
	if countByTopic(events, TopicMergeConflict) != 1 {
		t.Fatalf("TopicMergeConflict published %d times, want 1", countByTopic(events, TopicMergeConflict))
	}
	if hasTopic(events, TopicMergeCompleted) {
		t.Fatal("TopicMergeCompleted published for a failed merge")
	}
	if got := conflictHead(t, rf, def.ID); got != headBefore {
		t.Fatalf("default head moved from %s to %s despite the failed merge request", headBefore, got)
	}
}

func TestRepoFileFeatures_Conflict_ResolvingOnTheSourceBranchLetsTheMergeSucceed(t *testing.T) {
	rf, _, def, feature := conflictDiverged(t, nil)
	ctx := context.Background()
	if _, err := rf.MergeBranch(ctx, feature.ID); err == nil {
		t.Fatal("first MergeBranch of a same-line edit returned no error")
	}
	resolved := conflictReplaceLine(conflictReplaceLine(conflictBase, 2, "from default"), 4, "resolved on branch")
	conflictWrite(t, rf, feature.ID, conflictPath, resolved)

	if _, err := rf.MergeBranch(ctx, feature.ID); err != nil {
		t.Fatalf("MergeBranch after resolving: %v", err)
	}

	if got := conflictRead(t, rf, def.ID, conflictPath); got != resolved {
		t.Fatalf("default content = %q, want %q", got, resolved)
	}
}

func TestRepoFileFeatures_Conflict_ParallelMergesLoseNoUpdate(t *testing.T) {
	rf, repo, def := conflictOpen(t, nil)
	ctx := context.Background()
	conflictWrite(t, rf, def.ID, "base.txt", "base\n")
	branchA := conflictBranchFrom(t, rf, repo.ID, def.ID, "feature/a")
	branchB := conflictBranchFrom(t, rf, repo.ID, def.ID, "feature/b")
	conflictWrite(t, rf, branchA.ID, "a.txt", "from a\n")
	conflictWrite(t, rf, branchB.ID, "b.txt", "from b\n")

	ids := []string{branchA.ID, branchB.ID}
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(idx int, branchID string) {
			defer wg.Done()
			_, errs[idx] = rf.MergeBranch(ctx, branchID)
		}(i, id)
	}
	wg.Wait()

	for i, err := range errs {
		if err == nil {
			continue
		}
		if !errors.Is(err, ErrMergeConcurrencyConflict) {
			t.Fatalf("parallel merge %d failed with %v, want nil or ErrMergeConcurrencyConflict", i, err)
		}
		if _, retryErr := rf.MergeBranch(ctx, ids[i]); retryErr != nil {
			t.Fatalf("retry of parallel merge %d: %v", i, retryErr)
		}
	}

	if got := conflictRead(t, rf, def.ID, "a.txt"); got != "from a\n" {
		t.Fatalf("default lost a.txt: %q", got)
	}
	if got := conflictRead(t, rf, def.ID, "b.txt"); got != "from b\n" {
		t.Fatalf("default lost b.txt: %q", got)
	}
	if got := conflictRead(t, rf, def.ID, "base.txt"); got != "base\n" {
		t.Fatalf("default lost base.txt: %q", got)
	}
}
