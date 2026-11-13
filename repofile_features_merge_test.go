package mwanachamagit

import (
	"context"
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func mergeAssertFile(t *testing.T, rf *RepoFile, branchID, path, want string) {
	t.Helper()
	blob, err := rf.ReadFile(context.Background(), branchID, path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	if blob.Content != want {
		t.Fatalf("%s = %q, want %q", path, blob.Content, want)
	}
}

func mergeAssertFiles(t *testing.T, rf *RepoFile, branchID string, want map[string]string) {
	t.Helper()
	for path, content := range want {
		mergeAssertFile(t, rf, branchID, path, content)
	}
}

func mergeLogSHAs(t *testing.T, rf *RepoFile, branchID string) []string {
	t.Helper()
	entries, err := rf.Log(context.Background(), branchID, LogFilter{})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	shas := make([]string, len(entries))
	for i, e := range entries {
		shas[i] = e.SHA
	}
	return shas
}

func mergeContains(shas []string, sha string) bool {
	for _, s := range shas {
		if s == sha {
			return true
		}
	}
	return false
}

func mergeReload(t *testing.T, rf *RepoFile, branchID string) models.Branch {
	t.Helper()
	branch, err := rf.GetBranch(context.Background(), branchID)
	if err != nil {
		t.Fatalf("GetBranch %s: %v", branchID, err)
	}
	return branch
}

type mergeDiverged struct {
	rf            *RepoFile
	repo          models.Repository
	defaultBranch models.Branch
	source        models.Branch
	defaultCommit models.Commit
	sourceCommit  models.Commit
}

func newMergeDiverged(t *testing.T) mergeDiverged {
	t.Helper()
	rf, repo, defaultBranch := newFeatureRepo(t)
	writeFeatureFiles(t, rf, defaultBranch.ID, []featureFile{{Path: "base.txt", Content: "base"}})
	source := repoFeatureBranch(t, rf, repo, "source", "")
	defaultCommit := writeFeatureFiles(t, rf, defaultBranch.ID, []featureFile{{Path: "d.txt", Content: "from default"}})[0]
	sourceCommit := writeFeatureFiles(t, rf, source.ID, []featureFile{{Path: "s.txt", Content: "from source"}})[0]
	return mergeDiverged{
		rf:            rf,
		repo:          repo,
		defaultBranch: mergeReload(t, rf, defaultBranch.ID),
		source:        mergeReload(t, rf, source.ID),
		defaultCommit: defaultCommit,
		sourceCommit:  sourceCommit,
	}
}

func (d mergeDiverged) assertBothSidesKept(t *testing.T) {
	t.Helper()
	mergeAssertFiles(t, d.rf, d.defaultBranch.ID, map[string]string{
		"base.txt": "base",
		"d.txt":    "from default",
		"s.txt":    "from source",
	})
	shas := mergeLogSHAs(t, d.rf, d.defaultBranch.ID)
	if !mergeContains(shas, d.defaultCommit.SHA) {
		t.Fatalf("default history lost its own commit %s: %v", d.defaultCommit.SHA, shas)
	}
	if !mergeContains(shas, d.sourceCommit.SHA) {
		t.Fatalf("default history lacks the merged commit %s: %v", d.sourceCommit.SHA, shas)
	}
}

func TestRepoFileFeatures_Merge_BranchLifecycleInMergeFlow(t *testing.T) {
	rf, repo, defaultBranch := newFeatureRepo(t)
	ctx := context.Background()
	feature := repoFeatureBranch(t, rf, repo, "feature", "run-flow")

	byName, err := rf.GetBranchByName(ctx, repo.ID, "feature")
	if err != nil || byName.ID != feature.ID {
		t.Fatalf("GetBranchByName = %+v, err=%v", byName, err)
	}
	listed, err := rf.ListBranches(ctx, repo.ID)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if names := repoFeatureBranchNames(listed); !names["main"] || !names["feature"] || len(listed) != 2 {
		t.Fatalf("branches before merge = %+v", listed)
	}

	writeFeatureFiles(t, rf, feature.ID, []featureFile{{Path: "feature.txt", Content: "feature work"}})
	if _, err := rf.MergeBranch(ctx, feature.ID); err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	if err := rf.DeleteBranch(ctx, feature.ID); err != nil {
		t.Fatalf("DeleteBranch merged branch: %v", err)
	}

	if _, err := rf.GetBranch(ctx, feature.ID); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("GetBranch deleted err = %v, want ErrBranchNotFound", err)
	}
	if _, err := rf.GetBranchByName(ctx, repo.ID, "feature"); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("GetBranchByName deleted err = %v, want ErrBranchNotFound", err)
	}
	after, err := rf.ListBranches(ctx, repo.ID)
	if err != nil {
		t.Fatalf("ListBranches after delete: %v", err)
	}
	if len(after) != 1 || after[0].ID != defaultBranch.ID || !after[0].IsDefault {
		t.Fatalf("branches after delete = %+v, want only the default", after)
	}
	mergeAssertFile(t, rf, defaultBranch.ID, "feature.txt", "feature work")
	if err := rf.DeleteBranch(ctx, defaultBranch.ID); !errors.Is(err, ErrDefaultBranchDeleteForbidden) {
		t.Fatalf("DeleteBranch default err = %v, want ErrDefaultBranchDeleteForbidden", err)
	}
}

func TestRepoFileFeatures_Merge_FastForwardMovesDefaultToSourceHead(t *testing.T) {
	rf, repo, defaultBranch := newFeatureRepo(t)
	ctx := context.Background()
	writeFeatureFiles(t, rf, defaultBranch.ID, []featureFile{{Path: "base.txt", Content: "base"}})
	feature := repoFeatureBranch(t, rf, repo, "feature", "")
	writeFeatureFiles(t, rf, feature.ID, []featureFile{
		{Path: "one.txt", Content: "1"},
		{Path: "two.txt", Content: "2"},
	})
	feature = mergeReload(t, rf, feature.ID)

	merged, err := rf.MergeBranch(ctx, feature.ID)
	if err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	if merged.HeadCommitID != feature.HeadCommitID || merged.SHA != feature.SHA {
		t.Fatalf("default head = %q/%q, want source head %q/%q", merged.HeadCommitID, merged.SHA, feature.HeadCommitID, feature.SHA)
	}
	reread := mergeReload(t, rf, defaultBranch.ID)
	if reread.HeadCommitID != feature.HeadCommitID {
		t.Fatalf("persisted default head = %q, want %q", reread.HeadCommitID, feature.HeadCommitID)
	}
	defaultLog := mergeLogSHAs(t, rf, defaultBranch.ID)
	sourceLog := mergeLogSHAs(t, rf, feature.ID)
	if len(defaultLog) != 3 || len(defaultLog) != len(sourceLog) {
		t.Fatalf("logs differ: default %v, source %v", defaultLog, sourceLog)
	}
	for i := range defaultLog {
		if defaultLog[i] != sourceLog[i] {
			t.Fatalf("logs differ at %d: default %v, source %v", i, defaultLog, sourceLog)
		}
	}
	mergeAssertFiles(t, rf, defaultBranch.ID, map[string]string{"base.txt": "base", "one.txt": "1", "two.txt": "2"})
}

func TestRepoFileFeatures_Merge_DivergedBranchesKeepBothSides(t *testing.T) {
	d := newMergeDiverged(t)

	if _, err := d.rf.MergeBranch(context.Background(), d.source.ID); err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	d.assertBothSidesKept(t)
}

func TestRepoFileFeatures_Merge_DivergedMergeLeavesSourceBranchUntouched(t *testing.T) {
	d := newMergeDiverged(t)
	ctx := context.Background()

	if _, err := d.rf.MergeBranch(ctx, d.source.ID); err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	after := mergeReload(t, d.rf, d.source.ID)
	if after.HeadCommitID != d.source.HeadCommitID {
		t.Fatalf("source head moved from %q to %q", d.source.HeadCommitID, after.HeadCommitID)
	}
	mergeAssertFile(t, d.rf, d.source.ID, "s.txt", "from source")
	if _, err := d.rf.ReadFile(ctx, d.source.ID, "d.txt"); err == nil {
		t.Fatal("merging into default must not copy default's file into the source branch")
	}
}

func TestRepoFileFeatures_Merge_TwoWritersBothLand(t *testing.T) {
	rf, repo, defaultBranch := newFeatureRepo(t)
	ctx := context.Background()
	writeFeatureFiles(t, rf, defaultBranch.ID, []featureFile{{Path: "base.txt", Content: "base"}})
	w1 := repoFeatureBranch(t, rf, repo, "w1", "")
	w2 := repoFeatureBranch(t, rf, repo, "w2", "")
	c1 := writeFeatureFiles(t, rf, w1.ID, []featureFile{{Path: "w1.txt", Content: "writer one"}})[0]
	c2 := writeFeatureFiles(t, rf, w2.ID, []featureFile{{Path: "w2.txt", Content: "writer two"}})[0]

	if _, err := rf.MergeBranch(ctx, w1.ID); err != nil {
		t.Fatalf("MergeBranch w1: %v", err)
	}
	if _, err := rf.MergeBranch(ctx, w2.ID); err != nil {
		t.Fatalf("MergeBranch w2: %v", err)
	}

	mergeAssertFiles(t, rf, defaultBranch.ID, map[string]string{
		"base.txt": "base",
		"w1.txt":   "writer one",
		"w2.txt":   "writer two",
	})
	shas := mergeLogSHAs(t, rf, defaultBranch.ID)
	for _, sha := range []string{c1.SHA, c2.SHA} {
		if !mergeContains(shas, sha) {
			t.Fatalf("default history lacks %s: %v", sha, shas)
		}
	}
}

func TestRepoFileFeatures_Merge_EmptyBranchLeavesPopulatedDefaultUnchanged(t *testing.T) {
	rf, repo, defaultBranch := newFeatureRepo(t)
	ctx := context.Background()
	writeFeatureFiles(t, rf, defaultBranch.ID, []featureFile{{Path: "a.txt", Content: "a"}, {Path: "b.txt", Content: "b"}})
	before := mergeReload(t, rf, defaultBranch.ID)
	logBefore := mergeLogSHAs(t, rf, defaultBranch.ID)
	idle := repoFeatureBranch(t, rf, repo, "idle", "")

	merged, err := rf.MergeBranch(ctx, idle.ID)
	if err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	if merged.ID != defaultBranch.ID || merged.HeadCommitID != before.HeadCommitID || merged.SHA != before.SHA {
		t.Fatalf("merged = %+v, want default unchanged at %q/%q", merged, before.HeadCommitID, before.SHA)
	}
	logAfter := mergeLogSHAs(t, rf, defaultBranch.ID)
	if len(logAfter) != len(logBefore) || logAfter[0] != logBefore[0] {
		t.Fatalf("log changed: before %v, after %v", logBefore, logAfter)
	}
	mergeAssertFiles(t, rf, defaultBranch.ID, map[string]string{"a.txt": "a", "b.txt": "b"})
}

func TestRepoFileFeatures_Merge_RequestFastForwardFlow(t *testing.T) {
	rf, repo, defaultBranch := newFeatureRepo(t)
	ctx := context.Background()
	writeFeatureFiles(t, rf, defaultBranch.ID, []featureFile{{Path: "base.txt", Content: "base"}})
	feature := repoFeatureBranch(t, rf, repo, "feature", "run-mr")
	writeFeatureFiles(t, rf, feature.ID, []featureFile{{Path: "f.txt", Content: "feature"}})
	sourceHead := mergeReload(t, rf, feature.ID)

	opened, err := rf.CreateMergeRequest(ctx, CreateMergeRequestRequest{
		RepositoryID:   repo.ID,
		SourceBranchID: feature.ID,
		Title:          "ship feature",
		AuthorName:     "Ada",
	})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}
	if opened.Status != models.MergeRequestStatusOpen || opened.TargetBranchID != defaultBranch.ID {
		t.Fatalf("opened = %+v", opened)
	}
	listed, err := rf.ListMergeRequests(ctx, MergeRequestFilter{RepositoryID: repo.ID, Status: models.MergeRequestStatusOpen})
	if err != nil || len(listed) != 1 || listed[0].ID != opened.ID {
		t.Fatalf("ListMergeRequests open = %+v, err=%v", listed, err)
	}
	got, err := rf.GetMergeRequest(ctx, opened.ID)
	if err != nil || got.ID != opened.ID || got.SourceBranchID != feature.ID {
		t.Fatalf("GetMergeRequest = %+v, err=%v", got, err)
	}

	completed, err := rf.CompleteMergeRequest(ctx, opened.ID, "Grace")
	if err != nil {
		t.Fatalf("CompleteMergeRequest: %v", err)
	}
	head := mergeReload(t, rf, defaultBranch.ID)
	if completed.Status != models.MergeRequestStatusMerged || completed.MergedBy != "Grace" {
		t.Fatalf("completed = %+v", completed)
	}
	if completed.MergedCommitSHA == "" || completed.MergedCommitSHA != head.SHA {
		t.Fatalf("MergedCommitSHA = %q, default head SHA = %q", completed.MergedCommitSHA, head.SHA)
	}
	if head.HeadCommitID != sourceHead.HeadCommitID {
		t.Fatalf("default head = %q, want fast-forward to %q", head.HeadCommitID, sourceHead.HeadCommitID)
	}
	mergeAssertFiles(t, rf, defaultBranch.ID, map[string]string{"base.txt": "base", "f.txt": "feature"})
	stillOpen, err := rf.ListMergeRequests(ctx, MergeRequestFilter{RepositoryID: repo.ID, Status: models.MergeRequestStatusOpen})
	if err != nil || len(stillOpen) != 0 {
		t.Fatalf("open requests after completion = %+v, err=%v", stillOpen, err)
	}
	if _, err := rf.CompleteMergeRequest(ctx, opened.ID, "Grace"); !errors.Is(err, ErrMergeRequestNotOpen) {
		t.Fatalf("second complete err = %v, want ErrMergeRequestNotOpen", err)
	}
}

func TestRepoFileFeatures_Merge_RequestDivergedFlowKeepsBothSides(t *testing.T) {
	d := newMergeDiverged(t)
	ctx := context.Background()

	opened, err := d.rf.CreateMergeRequest(ctx, CreateMergeRequestRequest{
		RepositoryID:   d.repo.ID,
		SourceBranchID: d.source.ID,
		Title:          "bring source in",
	})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}
	listed, err := d.rf.ListMergeRequests(ctx, MergeRequestFilter{RepositoryID: d.repo.ID})
	if err != nil || len(listed) != 1 || listed[0].ID != opened.ID {
		t.Fatalf("ListMergeRequests = %+v, err=%v", listed, err)
	}
	if got, err := d.rf.GetMergeRequest(ctx, opened.ID); err != nil || got.Status != models.MergeRequestStatusOpen {
		t.Fatalf("GetMergeRequest = %+v, err=%v", got, err)
	}

	completed, err := d.rf.CompleteMergeRequest(ctx, opened.ID, "")
	if err != nil {
		t.Fatalf("CompleteMergeRequest: %v", err)
	}
	head := mergeReload(t, d.rf, d.defaultBranch.ID)
	if completed.Status != models.MergeRequestStatusMerged {
		t.Fatalf("status = %q, want merged", completed.Status)
	}
	if completed.MergedCommitSHA == "" || completed.MergedCommitSHA != head.SHA {
		t.Fatalf("MergedCommitSHA = %q, default head SHA = %q", completed.MergedCommitSHA, head.SHA)
	}
	d.assertBothSidesKept(t)
}

func TestRepoFileFeatures_Merge_RequestCloseLeavesDefaultAndBranchIntact(t *testing.T) {
	d := newMergeDiverged(t)
	ctx := context.Background()
	opened, err := d.rf.CreateMergeRequest(ctx, CreateMergeRequestRequest{
		RepositoryID:   d.repo.ID,
		SourceBranchID: d.source.ID,
		Title:          "not now",
	})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}

	closed, err := d.rf.CloseMergeRequest(ctx, opened.ID)
	if err != nil {
		t.Fatalf("CloseMergeRequest: %v", err)
	}
	if closed.Status != models.MergeRequestStatusClosed || closed.MergedCommitSHA != "" {
		t.Fatalf("closed = %+v", closed)
	}
	head := mergeReload(t, d.rf, d.defaultBranch.ID)
	if head.HeadCommitID != d.defaultBranch.HeadCommitID || head.SHA != d.defaultBranch.SHA {
		t.Fatalf("default head moved from %q to %q", d.defaultBranch.HeadCommitID, head.HeadCommitID)
	}
	if _, err := d.rf.ReadFile(ctx, d.defaultBranch.ID, "s.txt"); err == nil {
		t.Fatal("closing a request must not bring the source file into default")
	}
	mergeAssertFile(t, d.rf, d.defaultBranch.ID, "d.txt", "from default")
	source := mergeReload(t, d.rf, d.source.ID)
	if source.HeadCommitID != d.source.HeadCommitID {
		t.Fatalf("source head moved from %q to %q", d.source.HeadCommitID, source.HeadCommitID)
	}
	mergeAssertFile(t, d.rf, d.source.ID, "s.txt", "from source")
	if _, err := d.rf.CompleteMergeRequest(ctx, opened.ID, ""); !errors.Is(err, ErrMergeRequestNotOpen) {
		t.Fatalf("complete after close err = %v, want ErrMergeRequestNotOpen", err)
	}
}
