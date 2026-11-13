package mwanachamagit

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func historyDefaultBranch(t *testing.T, rf *RepoFile, repoID string) models.Branch {
	t.Helper()
	branches, err := rf.ListBranches(context.Background(), repoID)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	for _, b := range branches {
		if b.IsDefault {
			return b
		}
	}
	t.Fatalf("repository %s has no default branch", repoID)
	return models.Branch{}
}

func historyBranchWithFiles(t *testing.T, rf *RepoFile, repoID, name, runID string, files []featureFile) (models.Branch, []models.Commit) {
	t.Helper()
	branch, err := rf.CreateBranch(context.Background(), CreateBranchRequest{RepositoryID: repoID, Name: name, WorkflowRunID: runID})
	if err != nil {
		t.Fatalf("CreateBranch %s: %v", name, err)
	}
	return branch, writeFeatureFiles(t, rf, branch.ID, files)
}

func historyMerge(t *testing.T, rf *RepoFile, branchID string) {
	t.Helper()
	if _, err := rf.MergeBranch(context.Background(), branchID); err != nil {
		t.Fatalf("MergeBranch %s: %v", branchID, err)
	}
}

func historyLog(t *testing.T, rf *RepoFile, branchID string, filter LogFilter) []CommitEntry {
	t.Helper()
	entries, err := rf.Log(context.Background(), branchID, filter)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	return entries
}

func historySHAs(entries []CommitEntry) []string {
	shas := make([]string, len(entries))
	for i, e := range entries {
		shas[i] = e.SHA
	}
	return shas
}

func historyNewestFirst(commits []models.Commit) []string {
	shas := make([]string, len(commits))
	for i, c := range commits {
		shas[len(commits)-1-i] = c.SHA
	}
	return shas
}

func historyContains(shas []string, sha string) bool {
	for _, s := range shas {
		if s == sha {
			return true
		}
	}
	return false
}

func TestRepoFileFeatures_History_LogAfterFastForwardMergeIsNewestFirst(t *testing.T) {
	rf, repo, _ := newFeatureRepo(t)
	main := historyDefaultBranch(t, rf, repo.ID)
	feature, commits := historyBranchWithFiles(t, rf, repo.ID, "feature/history", "", []featureFile{
		{Path: "docs/one.md", Content: "one"},
		{Path: "docs/two.md", Content: "two"},
		{Path: "docs/three.md", Content: "three"},
	})
	historyMerge(t, rf, feature.ID)

	got := historySHAs(historyLog(t, rf, main.ID, LogFilter{}))
	want := historyNewestFirst(commits)
	if len(got) < len(want) {
		t.Fatalf("default branch log has %d commits after merge, want at least %d", len(got), len(want))
	}
	if !reflect.DeepEqual(got[:len(want)], want) {
		t.Fatalf("default branch log head = %v, want branch commits newest-first %v", got[:len(want)], want)
	}
}

func TestRepoFileFeatures_History_LogWithLimitAfterMergeKeepsNewest(t *testing.T) {
	rf, repo, _ := newFeatureRepo(t)
	main := historyDefaultBranch(t, rf, repo.ID)
	feature, commits := historyBranchWithFiles(t, rf, repo.ID, "feature/limit", "", []featureFile{
		{Path: "a.txt", Content: "a"},
		{Path: "b.txt", Content: "b"},
		{Path: "c.txt", Content: "c"},
	})
	historyMerge(t, rf, feature.ID)

	got := historySHAs(historyLog(t, rf, main.ID, LogFilter{Limit: 2}))
	want := historyNewestFirst(commits)[:2]
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Log Limit 2 = %v, want %v", got, want)
	}
}

func TestRepoFileFeatures_History_LogByPathAfterMerges(t *testing.T) {
	rf, repo, _ := newFeatureRepo(t)
	main := historyDefaultBranch(t, rf, repo.ID)

	first, firstCommits := historyBranchWithFiles(t, rf, repo.ID, "feature/first", "", []featureFile{{Path: "first.txt", Content: "first"}})
	historyMerge(t, rf, first.ID)
	second, secondCommits := historyBranchWithFiles(t, rf, repo.ID, "feature/second", "", []featureFile{{Path: "second.txt", Content: "second"}})
	historyMerge(t, rf, second.ID)

	firstSHA := firstCommits[0].SHA
	secondSHA := secondCommits[0].SHA

	forFirst := historySHAs(historyLog(t, rf, main.ID, LogFilter{Path: "first.txt"}))
	if !historyContains(forFirst, firstSHA) {
		t.Errorf("Log path first.txt = %v, want it to contain the commit that added it (%s)", forFirst, firstSHA)
	}
	if historyContains(forFirst, secondSHA) {
		t.Errorf("Log path first.txt = %v, must not contain the later commit %s that did not modify first.txt", forFirst, secondSHA)
	}

	forSecond := historySHAs(historyLog(t, rf, main.ID, LogFilter{Path: "second.txt"}))
	if !historyContains(forSecond, secondSHA) {
		t.Errorf("Log path second.txt = %v, want it to contain the commit that added it (%s)", forSecond, secondSHA)
	}
	if historyContains(forSecond, firstSHA) {
		t.Errorf("Log path second.txt = %v, must not contain the earlier commit %s that predates second.txt", forSecond, firstSHA)
	}

	if none := historyLog(t, rf, main.ID, LogFilter{Path: "missing.txt"}); len(none) != 0 {
		t.Errorf("Log path missing.txt = %v, want no commits", historySHAs(none))
	}
}

func TestRepoFileFeatures_History_DiffOrderIsDeterministicAndSortedByPath(t *testing.T) {
	rf, repo, _ := newFeatureRepo(t)
	main := historyDefaultBranch(t, rf, repo.ID)
	base := writeFeatureFiles(t, rf, main.ID, []featureFile{{Path: "base.txt", Content: "base"}})
	feature, commits := historyBranchWithFiles(t, rf, repo.ID, "feature/many", "", []featureFile{
		{Path: "zeta/last.txt", Content: "z"},
		{Path: "alpha/first.txt", Content: "a"},
		{Path: "mid/m.txt", Content: "m"},
		{Path: "beta/second.txt", Content: "b"},
		{Path: "omega.txt", Content: "o"},
		{Path: "gamma/third.txt", Content: "g"},
		{Path: "delta/fourth.txt", Content: "d"},
		{Path: "base.txt", Content: "base changed"},
	})
	last := commits[len(commits)-1]
	from := base[len(base)-1].ID

	ctx := context.Background()
	reference, err := rf.Diff(ctx, from, feature.ID)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(reference) < 6 {
		t.Fatalf("Diff returned %d files, want at least 6: %+v", len(reference), reference)
	}
	paths := make([]string, len(reference))
	for i, d := range reference {
		paths[i] = d.Path
	}
	if !sort.StringsAreSorted(paths) {
		t.Errorf("Diff paths are not sorted by path: %v", paths)
	}
	for call := 1; call <= 20; call++ {
		again, err := rf.Diff(ctx, from, last.ID)
		if err != nil {
			t.Fatalf("Diff call %d: %v", call, err)
		}
		if !reflect.DeepEqual(again, reference) {
			t.Fatalf("Diff call %d order differs from first call:\n first: %v\n  call: %v", call, historyDiffPaths(reference), historyDiffPaths(again))
		}
	}
}

func historyDiffPaths(diffs []FileDiff) []string {
	paths := make([]string, len(diffs))
	for i, d := range diffs {
		paths[i] = fmt.Sprintf("%s:%s", d.Operation, d.Path)
	}
	return paths
}

func TestRepoFileFeatures_History_TagsResolveAfterMerge(t *testing.T) {
	ctx := context.Background()
	rf, repo, _ := newFeatureRepo(t)
	main := historyDefaultBranch(t, rf, repo.ID)
	before := writeFeatureFiles(t, rf, main.ID, []featureFile{{Path: "before.txt", Content: "before"}})[0]
	oldTag, err := rf.CreateTag(ctx, CreateTagRequest{RepositoryID: repo.ID, Name: "v0.1.0", CommitID: before.ID})
	if err != nil {
		t.Fatalf("CreateTag v0.1.0: %v", err)
	}

	feature, commits := historyBranchWithFiles(t, rf, repo.ID, "feature/release", "", []featureFile{
		{Path: "release/notes.md", Content: "notes"},
		{Path: "release/changelog.md", Content: "changes"},
	})
	historyMerge(t, rf, feature.ID)
	merged := commits[len(commits)-1]

	newTag, err := rf.CreateTag(ctx, CreateTagRequest{RepositoryID: repo.ID, Name: "v0.2.0", CommitID: merged.ID, Message: "release"})
	if err != nil {
		t.Fatalf("CreateTag v0.2.0: %v", err)
	}
	if newTag.SHA != merged.SHA {
		t.Errorf("new tag SHA = %s, want merged commit %s", newTag.SHA, merged.SHA)
	}

	gotNew, err := rf.GetTag(ctx, newTag.ID)
	if err != nil {
		t.Fatalf("GetTag new: %v", err)
	}
	if gotNew.SHA != merged.SHA || gotNew.Name != "v0.2.0" {
		t.Errorf("GetTag new = %s@%s, want v0.2.0@%s", gotNew.Name, gotNew.SHA, merged.SHA)
	}
	gotOld, err := rf.GetTag(ctx, oldTag.ID)
	if err != nil {
		t.Fatalf("GetTag old: %v", err)
	}
	if gotOld.SHA != before.SHA {
		t.Errorf("pre-merge tag SHA = %s, want the old commit %s", gotOld.SHA, before.SHA)
	}
	if gotOld.SHA == merged.SHA {
		t.Errorf("pre-merge tag moved to the merged commit %s", merged.SHA)
	}

	tags, err := rf.ListTags(ctx, repo.ID)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	bySHA := map[string]string{}
	for _, tag := range tags {
		bySHA[tag.Name] = tag.SHA
	}
	if len(tags) != 2 || bySHA["v0.1.0"] != before.SHA || bySHA["v0.2.0"] != merged.SHA {
		t.Errorf("ListTags = %v, want v0.1.0@%s and v0.2.0@%s", bySHA, before.SHA, merged.SHA)
	}

	mainLog := historySHAs(historyLog(t, rf, main.ID, LogFilter{}))
	if !historyContains(mainLog, merged.SHA) || !historyContains(mainLog, before.SHA) {
		t.Errorf("default branch log %v must contain both tagged commits %s and %s", mainLog, before.SHA, merged.SHA)
	}
}

func TestRepoFileFeatures_History_RollbackKeepsMergedSHAOfMergedRequest(t *testing.T) {
	const runID = "wfr_history_rollback"
	ctx := context.Background()
	rf, repo, _ := newFeatureRepo(t)
	main := historyDefaultBranch(t, rf, repo.ID)
	feature, commits := historyBranchWithFiles(t, rf, repo.ID, "feature/rollback", runID, []featureFile{
		{Path: "run/output.txt", Content: "output"},
	})
	mr, err := rf.CreateMergeRequest(ctx, CreateMergeRequestRequest{RepositoryID: repo.ID, Title: "run output", SourceBranchID: feature.ID, WorkflowRunID: runID})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}
	merged, err := rf.CompleteMergeRequest(ctx, mr.ID, "history-test")
	if err != nil {
		t.Fatalf("CompleteMergeRequest: %v", err)
	}
	if merged.Status != models.MergeRequestStatusMerged || merged.MergedCommitSHA == "" {
		t.Fatalf("merged request = status %q sha %q, want merged with a SHA", merged.Status, merged.MergedCommitSHA)
	}
	mergedSHA := merged.MergedCommitSHA
	if want := commits[len(commits)-1].SHA; mergedSHA != want {
		t.Errorf("merged_commit_sha = %s, want fast-forwarded branch head %s", mergedSHA, want)
	}

	result, err := rf.RollbackByWorkflowRun(ctx, runID)
	if err != nil {
		t.Fatalf("RollbackByWorkflowRun: %v", err)
	}
	if result.MergeRequestsRolledBack != 1 {
		t.Errorf("rollback result = %+v, want 1 merge request rolled back", result)
	}

	got, err := rf.GetMergeRequest(ctx, mr.ID)
	if err != nil {
		t.Fatalf("GetMergeRequest: %v", err)
	}
	if got.Status != models.MergeRequestStatusRolledBack {
		t.Errorf("status = %q, want %q", got.Status, models.MergeRequestStatusRolledBack)
	}
	if got.MergedCommitSHA != mergedSHA {
		t.Errorf("merged_commit_sha after rollback = %q, want preserved %q", got.MergedCommitSHA, mergedSHA)
	}
	if _, err := rf.GetBranch(ctx, main.ID); err != nil {
		t.Errorf("default branch lost by rollback: %v", err)
	}
}

func TestRepoFileFeatures_History_RebaseOperationExists(t *testing.T) {
	managerType := reflect.TypeOf((*GitManager)(nil)).Elem()
	for i := 0; i < managerType.NumMethod(); i++ {
		if strings.Contains(managerType.Method(i).Name, "Rebase") {
			return
		}
	}
	t.Fatalf("GitManager has no rebase operation although git.go MergeBranch documents rebase conflicts")
}
