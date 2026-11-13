package mwanachamagit

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

type recordingBlobSearcher struct {
	results []BlobSearchResult
	query   string
	limit   int
	calls   int
}

func (s *recordingBlobSearcher) Search(_ context.Context, query string, limit int) ([]BlobSearchResult, error) {
	s.calls++
	s.query = query
	s.limit = limit
	return s.results, nil
}

func blobOnBranch(t *testing.T, rf *RepoFile, branchID, path string) models.Blob {
	t.Helper()
	blob, err := rf.ReadFile(context.Background(), branchID, path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	return blob
}

func nodeIDs(result GraphResult) []string {
	ids := make([]string, 0, len(result.Nodes))
	for _, n := range result.Nodes {
		ids = append(ids, n.ID)
	}
	sort.Strings(ids)
	return ids
}

func nodePaths(result GraphResult) []string {
	paths := make([]string, 0, len(result.Nodes))
	for _, n := range result.Nodes {
		p, _ := n.Properties["path"].(string)
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func mustCreateKeyword(t *testing.T, rf *RepoFile, req CreateKeywordRequest) models.Keyword {
	t.Helper()
	kw, err := rf.CreateKeyword(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateKeyword %s: %v", req.Name, err)
	}
	return kw
}

func mustCreateEdge(t *testing.T, rf *RepoFile, req CreateEdgeRequest) {
	t.Helper()
	if err := rf.CreateEdge(context.Background(), req); err != nil {
		t.Fatalf("CreateEdge %s: %v", req.RelationshipName, err)
	}
}

func TestRepoFileFeatures_WriteFile(t *testing.T) {
	ctx := context.Background()
	rf, _, branch := newFeatureRepo(t)

	first, err := rf.WriteFile(ctx, WriteFileRequest{
		BranchID:    branch.ID,
		Path:        "/docs/guide.md",
		Content:     "# guide",
		Message:     "add guide",
		AuthorName:  "Ada",
		AuthorEmail: "ada@example.org",
	})
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if first.Message != "add guide" || first.AuthorName != "Ada" || first.SHA == "" {
		t.Fatalf("unexpected first commit %+v", first)
	}
	if len(first.ParentIDs) != 0 {
		t.Fatalf("first commit has parents %v", first.ParentIDs)
	}

	second, err := rf.WriteFile(ctx, WriteFileRequest{BranchID: branch.ID, Path: "docs/guide.md", Content: "# guide v2"})
	if err != nil {
		t.Fatalf("WriteFile second: %v", err)
	}
	if second.Message != "Update docs/guide.md" {
		t.Fatalf("default message = %q", second.Message)
	}
	if len(second.ParentIDs) != 1 || second.ParentIDs[0] != first.ID {
		t.Fatalf("second commit parents = %v, want [%s]", second.ParentIDs, first.ID)
	}

	got, err := rf.GetBranch(ctx, branch.ID)
	if err != nil {
		t.Fatalf("GetBranch: %v", err)
	}
	if got.HeadCommitID != second.ID {
		t.Fatalf("head = %s, want %s", got.HeadCommitID, second.ID)
	}

	if _, err := rf.WriteFile(ctx, WriteFileRequest{BranchID: "missing", Path: "a.txt", Content: "x"}); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("write to missing branch: want ErrBranchNotFound, got %v", err)
	}
}

func TestRepoFileFeatures_ReadFile(t *testing.T) {
	ctx := context.Background()
	rf, _, branch := newFeatureRepo(t)
	writeFeatureFiles(t, rf, branch.ID, []featureFile{
		{Path: "src/main.go", Content: "package main"},
		{Path: "README.md", Content: "# widgets"},
	})

	blob, err := rf.ReadFile(ctx, branch.ID, "src/main.go")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if blob.Content != "package main" || blob.Path != "src/main.go" || blob.Name != "main.go" || blob.Extension != "go" {
		t.Fatalf("unexpected blob %+v", blob)
	}
	if blob.Size != int64(len("package main")) {
		t.Fatalf("size = %d", blob.Size)
	}

	if _, err := rf.ReadFile(ctx, branch.ID, "nope.txt"); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("missing path: want ErrFileNotFound, got %v", err)
	}
	if _, err := rf.ReadFile(ctx, "missing", "README.md"); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("missing branch: want ErrBranchNotFound, got %v", err)
	}
}

func TestRepoFileFeatures_DeleteFile(t *testing.T) {
	ctx := context.Background()
	rf, _, branch := newFeatureRepo(t)
	commits := writeFeatureFiles(t, rf, branch.ID, []featureFile{
		{Path: "keep.txt", Content: "keep"},
		{Path: "drop.txt", Content: "drop"},
	})

	deletion, err := rf.DeleteFile(ctx, DeleteFileRequest{BranchID: branch.ID, Path: "drop.txt", Message: "remove drop"})
	if err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if deletion.Message != "remove drop" {
		t.Fatalf("message = %q", deletion.Message)
	}
	if len(deletion.ParentIDs) != 1 || deletion.ParentIDs[0] != commits[1].ID {
		t.Fatalf("deletion parents = %v, want [%s]", deletion.ParentIDs, commits[1].ID)
	}

	if _, err := rf.ReadFile(ctx, branch.ID, "drop.txt"); err == nil {
		t.Fatalf("deleted file still reads back")
	}
	if blob := blobOnBranch(t, rf, branch.ID, "keep.txt"); blob.Content != "keep" {
		t.Fatalf("keep.txt content = %q", blob.Content)
	}

	if _, err := rf.DeleteFile(ctx, DeleteFileRequest{BranchID: branch.ID, Path: "never-existed.txt"}); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("delete missing: want ErrFileNotFound, got %v", err)
	}
}

func TestRepoFileFeatures_ListDirectory(t *testing.T) {
	ctx := context.Background()
	rf, _, branch := newFeatureRepo(t)

	empty, err := rf.ListDirectory(ctx, branch.ID, "")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty branch listing = %+v, err=%v", empty, err)
	}

	writeFeatureFiles(t, rf, branch.ID, []featureFile{
		{Path: "README.md", Content: "readme"},
		{Path: "src/main.go", Content: "package main"},
		{Path: "src/util/strings.go", Content: "package util"},
		{Path: "docs/a.md", Content: "a"},
	})

	root, err := rf.ListDirectory(ctx, branch.ID, "")
	if err != nil {
		t.Fatalf("ListDirectory root: %v", err)
	}
	byName := map[string]FileEntry{}
	for _, e := range root {
		byName[e.Name] = e
	}
	if len(root) != 3 || byName["README.md"].IsDir || byName["README.md"].Size != 6 || !byName["src"].IsDir || !byName["docs"].IsDir {
		t.Fatalf("unexpected root listing %+v", root)
	}

	src, err := rf.ListDirectory(ctx, branch.ID, "src")
	if err != nil {
		t.Fatalf("ListDirectory src: %v", err)
	}
	srcByPath := map[string]FileEntry{}
	for _, e := range src {
		srcByPath[e.Path] = e
	}
	if len(src) != 2 || srcByPath["src/main.go"].IsDir || !srcByPath["src/util"].IsDir {
		t.Fatalf("unexpected src listing %+v", src)
	}

	if _, err := rf.ListDirectory(ctx, branch.ID, "ghost"); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("missing directory: want ErrFileNotFound, got %v", err)
	}
}

func TestRepoFileFeatures_Log(t *testing.T) {
	ctx := context.Background()
	rf, _, branch := newFeatureRepo(t)

	none, err := rf.Log(ctx, branch.ID, LogFilter{})
	if err != nil || len(none) != 0 {
		t.Fatalf("empty branch log = %+v, err=%v", none, err)
	}

	commits := make([]models.Commit, 0, 3)
	for _, f := range []WriteFileRequest{
		{BranchID: branch.ID, Path: "a.txt", Content: "a", Message: "add a", AuthorName: "Ada"},
		{BranchID: branch.ID, Path: "b.txt", Content: "b", Message: "add b", AuthorName: "Ben"},
		{BranchID: branch.ID, Path: "c.txt", Content: "c", Message: "add c", AuthorName: "Cy"},
	} {
		c, err := rf.WriteFile(ctx, f)
		if err != nil {
			t.Fatalf("WriteFile %s: %v", f.Path, err)
		}
		commits = append(commits, c)
	}

	full, err := rf.Log(ctx, branch.ID, LogFilter{})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(full) != 3 {
		t.Fatalf("log length = %d, want 3: %+v", len(full), full)
	}
	wantMessages := []string{"add c", "add b", "add a"}
	for i, want := range wantMessages {
		if full[i].Message != want || full[i].SHA != commits[2-i].SHA {
			t.Fatalf("entry %d = %+v, want message %q sha %s", i, full[i], want, commits[2-i].SHA)
		}
	}
	if full[0].Author != "Cy" || full[0].Timestamp.IsZero() {
		t.Fatalf("newest entry = %+v", full[0])
	}

	limited, err := rf.Log(ctx, branch.ID, LogFilter{Limit: 2})
	if err != nil {
		t.Fatalf("Log limit: %v", err)
	}
	if len(limited) != 2 || limited[0].Message != "add c" || limited[1].Message != "add b" {
		t.Fatalf("limited log = %+v", limited)
	}

	byPath, err := rf.Log(ctx, branch.ID, LogFilter{Path: "c.txt"})
	if err != nil {
		t.Fatalf("Log path: %v", err)
	}
	if len(byPath) != 1 || byPath[0].Message != "add c" {
		t.Fatalf("path log = %+v, want only the commit whose tree holds c.txt", byPath)
	}

	if _, err := rf.Log(ctx, "missing", LogFilter{}); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("missing branch: want ErrBranchNotFound, got %v", err)
	}
}

func TestRepoFileFeatures_Diff(t *testing.T) {
	ctx := context.Background()
	rf, _, branch := newFeatureRepo(t)
	commits := writeFeatureFiles(t, rf, branch.ID, []featureFile{
		{Path: "a.txt", Content: "one"},
		{Path: "a.txt", Content: "two"},
		{Path: "b.txt", Content: "new"},
	})

	modified, err := rf.Diff(ctx, commits[0].ID, commits[1].ID)
	if err != nil {
		t.Fatalf("Diff commits: %v", err)
	}
	if len(modified) != 1 || modified[0].Path != "a.txt" || modified[0].Operation != "modified" {
		t.Fatalf("diff c0..c1 = %+v", modified)
	}

	toHead, err := rf.Diff(ctx, commits[0].ID, branch.ID)
	if err != nil {
		t.Fatalf("Diff to branch: %v", err)
	}
	ops := map[string]string{}
	for _, d := range toHead {
		ops[d.Path] = d.Operation
	}
	if len(toHead) != 2 || ops["a.txt"] != "modified" || ops["b.txt"] != "added" {
		t.Fatalf("diff c0..head = %+v", toHead)
	}

	reverse, err := rf.Diff(ctx, commits[2].ID, commits[0].ID)
	if err != nil {
		t.Fatalf("Diff reverse: %v", err)
	}
	reverseOps := map[string]string{}
	for _, d := range reverse {
		reverseOps[d.Path] = d.Operation
	}
	if len(reverse) != 2 || reverseOps["b.txt"] != "deleted" || reverseOps["a.txt"] != "modified" {
		t.Fatalf("reverse diff = %+v", reverse)
	}

	bySHA, err := rf.Diff(ctx, commits[0].SHA, commits[1].SHA)
	if err != nil || len(bySHA) != 1 {
		t.Fatalf("diff by sha = %+v, err=%v", bySHA, err)
	}

	same, err := rf.Diff(ctx, commits[2].ID, branch.ID)
	if err != nil || len(same) != 0 {
		t.Fatalf("diff of identical refs = %+v, err=%v", same, err)
	}

	if _, err := rf.Diff(ctx, "nonexistent-ref", branch.ID); !errors.Is(err, ErrRefNotFound) {
		t.Fatalf("want ErrRefNotFound, got %v", err)
	}
}

func TestRepoFileFeatures_RollbackByWorkflowRun(t *testing.T) {
	ctx := context.Background()
	const runID = "run-feature-1"
	const otherRunID = "run-feature-2"
	rf, repo, defaultBranch := newFeatureRepo(t)
	writeFeatureFiles(t, rf, defaultBranch.ID, []featureFile{{Path: "base.txt", Content: "base"}})

	mergedBranch, err := rf.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "task/merged", WorkflowRunID: runID})
	if err != nil {
		t.Fatalf("CreateBranch merged: %v", err)
	}
	openBranch, err := rf.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "task/open", WorkflowRunID: runID})
	if err != nil {
		t.Fatalf("CreateBranch open: %v", err)
	}
	otherBranch, err := rf.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "task/other", WorkflowRunID: otherRunID})
	if err != nil {
		t.Fatalf("CreateBranch other: %v", err)
	}
	writeFeatureFiles(t, rf, mergedBranch.ID, []featureFile{{Path: "merged.txt", Content: "merged"}})

	mergedMR, err := rf.CreateMergeRequest(ctx, CreateMergeRequestRequest{RepositoryID: repo.ID, Title: "merged", SourceBranchID: mergedBranch.ID, WorkflowRunID: runID})
	if err != nil {
		t.Fatalf("CreateMergeRequest merged: %v", err)
	}
	completed, err := rf.CompleteMergeRequest(ctx, mergedMR.ID, "")
	if err != nil {
		t.Fatalf("CompleteMergeRequest: %v", err)
	}
	if completed.Status != models.MergeRequestStatusMerged || completed.MergedCommitSHA == "" {
		t.Fatalf("merge request not merged: %+v", completed)
	}
	openMR, err := rf.CreateMergeRequest(ctx, CreateMergeRequestRequest{RepositoryID: repo.ID, Title: "open", SourceBranchID: openBranch.ID, WorkflowRunID: runID})
	if err != nil {
		t.Fatalf("CreateMergeRequest open: %v", err)
	}
	otherMR, err := rf.CreateMergeRequest(ctx, CreateMergeRequestRequest{RepositoryID: repo.ID, Title: "other", SourceBranchID: otherBranch.ID, WorkflowRunID: otherRunID})
	if err != nil {
		t.Fatalf("CreateMergeRequest other: %v", err)
	}

	result, err := rf.RollbackByWorkflowRun(ctx, runID)
	if err != nil {
		t.Fatalf("RollbackByWorkflowRun: %v", err)
	}
	want := RollbackResult{WorkflowRunID: runID, BranchesDeleted: 2, MergeRequestsRolledBack: 2, DefaultBranchesSkipped: 0}
	if result != want {
		t.Fatalf("result = %+v, want %+v", result, want)
	}

	for _, id := range []string{mergedBranch.ID, openBranch.ID} {
		if _, err := rf.GetBranch(ctx, id); !errors.Is(err, ErrBranchNotFound) {
			t.Fatalf("branch %s should be gone, got %v", id, err)
		}
	}
	if _, err := rf.GetBranch(ctx, otherBranch.ID); err != nil {
		t.Fatalf("other run's branch should survive: %v", err)
	}
	if _, err := rf.GetBranch(ctx, defaultBranch.ID); err != nil {
		t.Fatalf("default branch should survive: %v", err)
	}

	gotMerged, err := rf.GetMergeRequest(ctx, mergedMR.ID)
	if err != nil {
		t.Fatalf("GetMergeRequest merged: %v", err)
	}
	if gotMerged.Status != models.MergeRequestStatusRolledBack || gotMerged.MergedCommitSHA != completed.MergedCommitSHA {
		t.Fatalf("merged MR after rollback = %+v", gotMerged)
	}
	gotOpen, err := rf.GetMergeRequest(ctx, openMR.ID)
	if err != nil || gotOpen.Status != models.MergeRequestStatusRolledBack {
		t.Fatalf("open MR after rollback = %+v, err=%v", gotOpen, err)
	}
	gotOther, err := rf.GetMergeRequest(ctx, otherMR.ID)
	if err != nil || gotOther.Status != models.MergeRequestStatusOpen {
		t.Fatalf("other MR after rollback = %+v, err=%v", gotOther, err)
	}

	again, err := rf.RollbackByWorkflowRun(ctx, runID)
	if err != nil {
		t.Fatalf("second rollback: %v", err)
	}
	if again.BranchesDeleted != 0 || again.MergeRequestsRolledBack != 0 {
		t.Fatalf("second rollback not a no-op: %+v", again)
	}

	if _, err := rf.RollbackByWorkflowRun(ctx, ""); !errors.Is(err, ErrWorkflowRunIDRequired) {
		t.Fatalf("empty run id: want ErrWorkflowRunIDRequired, got %v", err)
	}
}

func TestRepoFileFeatures_CreateKeyword(t *testing.T) {
	ctx := context.Background()
	rf, _, _ := newFeatureRepo(t)

	root := mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "security", Description: "all security", Scope: "agency"})
	if root.ID == "" || root.Name != "security" || root.Description != "all security" || root.Scope != "agency" || root.ParentID != "" {
		t.Fatalf("unexpected root keyword %+v", root)
	}
	child := mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "auth", ParentID: root.ID})
	if child.ParentID != root.ID {
		t.Fatalf("child parent = %q, want %q", child.ParentID, root.ID)
	}

	if _, err := rf.CreateKeyword(ctx, CreateKeywordRequest{Name: "security"}); !errors.Is(err, ErrKeywordAlreadyExists) {
		t.Fatalf("duplicate root: want ErrKeywordAlreadyExists, got %v", err)
	}
	if _, err := rf.CreateKeyword(ctx, CreateKeywordRequest{Name: "auth", ParentID: root.ID}); !errors.Is(err, ErrKeywordAlreadyExists) {
		t.Fatalf("duplicate sibling: want ErrKeywordAlreadyExists, got %v", err)
	}
	if _, err := rf.CreateKeyword(ctx, CreateKeywordRequest{Name: "orphan", ParentID: "missing"}); !errors.Is(err, ErrKeywordNotFound) {
		t.Fatalf("missing parent: want ErrKeywordNotFound, got %v", err)
	}
	sameNameElsewhere, err := rf.CreateKeyword(ctx, CreateKeywordRequest{Name: "security", ParentID: root.ID})
	if err != nil || sameNameElsewhere.ParentID != root.ID {
		t.Fatalf("same name under a different parent should be allowed: %+v, err=%v", sameNameElsewhere, err)
	}
}

func TestRepoFileFeatures_GetKeyword(t *testing.T) {
	ctx := context.Background()
	rf, _, _ := newFeatureRepo(t)
	root := mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "security", Scope: "agency"})
	a := mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "auth", ParentID: root.ID})
	b := mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "crypto", ParentID: root.ID})

	got, err := rf.GetKeyword(ctx, root.ID)
	if err != nil {
		t.Fatalf("GetKeyword: %v", err)
	}
	if got.Name != "security" || got.Scope != "agency" {
		t.Fatalf("unexpected keyword %+v", got)
	}
	children := append([]string(nil), got.ChildIDs...)
	sort.Strings(children)
	wantChildren := []string{a.ID, b.ID}
	sort.Strings(wantChildren)
	if !equalStrings(children, wantChildren) {
		t.Fatalf("child ids = %v, want %v", children, wantChildren)
	}

	if _, err := rf.GetKeyword(ctx, "missing"); !errors.Is(err, ErrKeywordNotFound) {
		t.Fatalf("want ErrKeywordNotFound, got %v", err)
	}
}

func TestRepoFileFeatures_ListKeywords(t *testing.T) {
	ctx := context.Background()
	rf, _, _ := newFeatureRepo(t)
	security := mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "security", Scope: "agency"})
	mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "billing", Scope: "finance"})
	mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "auth", Scope: "agency", ParentID: security.ID})

	names := func(kws []models.Keyword) []string {
		out := make([]string, len(kws))
		for i, k := range kws {
			out[i] = k.Name
		}
		sort.Strings(out)
		return out
	}

	roots, err := rf.ListKeywords(ctx, KeywordFilter{})
	if err != nil {
		t.Fatalf("ListKeywords roots: %v", err)
	}
	if got := names(roots); !equalStrings(got, []string{"billing", "security"}) {
		t.Fatalf("roots = %v", got)
	}

	kids, err := rf.ListKeywords(ctx, KeywordFilter{ParentID: security.ID})
	if err != nil {
		t.Fatalf("ListKeywords children: %v", err)
	}
	if got := names(kids); !equalStrings(got, []string{"auth"}) {
		t.Fatalf("children = %v", got)
	}

	scoped, err := rf.ListKeywords(ctx, KeywordFilter{Scope: "finance"})
	if err != nil {
		t.Fatalf("ListKeywords scope: %v", err)
	}
	if got := names(scoped); !equalStrings(got, []string{"billing"}) {
		t.Fatalf("scoped = %v", got)
	}

	limited, err := rf.ListKeywords(ctx, KeywordFilter{Limit: 1})
	if err != nil || len(limited) != 1 {
		t.Fatalf("limited = %+v, err=%v", limited, err)
	}
}

func TestRepoFileFeatures_GetKeywordTree(t *testing.T) {
	ctx := context.Background()
	rf, _, _ := newFeatureRepo(t)
	security := mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "security"})
	auth := mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "auth", ParentID: security.ID})
	mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "oauth", ParentID: auth.ID})
	mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "billing"})

	forest, err := rf.GetKeywordTree(ctx, "")
	if err != nil {
		t.Fatalf("GetKeywordTree forest: %v", err)
	}
	if len(forest) != 2 {
		t.Fatalf("forest roots = %d, want 2: %+v", len(forest), forest)
	}
	var securityNode KeywordTreeNode
	for _, n := range forest {
		if n.Keyword.Name == "security" {
			securityNode = n
		}
	}
	if len(securityNode.Children) != 1 || securityNode.Children[0].Keyword.Name != "auth" {
		t.Fatalf("security children = %+v", securityNode.Children)
	}
	authNode := securityNode.Children[0]
	if len(authNode.Children) != 1 || authNode.Children[0].Keyword.Name != "oauth" || len(authNode.Children[0].Children) != 0 {
		t.Fatalf("auth children = %+v", authNode.Children)
	}

	sub, err := rf.GetKeywordTree(ctx, auth.ID)
	if err != nil {
		t.Fatalf("GetKeywordTree subtree: %v", err)
	}
	if len(sub) != 1 || sub[0].Keyword.Name != "oauth" {
		t.Fatalf("subtree under auth = %+v", sub)
	}
}

func TestRepoFileFeatures_UpdateKeyword(t *testing.T) {
	ctx := context.Background()
	rf, _, _ := newFeatureRepo(t)
	kw := mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "secuirty", Description: "typo", Scope: "old"})

	updated, err := rf.UpdateKeyword(ctx, kw.ID, UpdateKeywordRequest{Name: "security", Description: "fixed", Scope: "agency"})
	if err != nil {
		t.Fatalf("UpdateKeyword: %v", err)
	}
	if updated.ID != kw.ID || updated.Name != "security" || updated.Description != "fixed" || updated.Scope != "agency" {
		t.Fatalf("unexpected updated keyword %+v", updated)
	}
	reread, err := rf.GetKeyword(ctx, kw.ID)
	if err != nil || reread.Name != "security" || reread.Scope != "agency" {
		t.Fatalf("reread = %+v, err=%v", reread, err)
	}

	if _, err := rf.UpdateKeyword(ctx, "missing", UpdateKeywordRequest{Name: "x"}); !errors.Is(err, ErrKeywordNotFound) {
		t.Fatalf("want ErrKeywordNotFound, got %v", err)
	}
}

func TestRepoFileFeatures_DeleteKeyword(t *testing.T) {
	ctx := context.Background()
	rf, _, branch := newFeatureRepo(t)
	writeFeatureFiles(t, rf, branch.ID, []featureFile{{Path: "auth.go", Content: "package auth"}})
	blob := blobOnBranch(t, rf, branch.ID, "auth.go")

	top := mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "top"})
	middle := mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "middle", ParentID: top.ID})
	leaf := mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "leaf", ParentID: middle.ID})
	mustCreateEdge(t, rf, CreateEdgeRequest{BranchID: branch.ID, FromEntityID: blob.ID, RelationshipName: "tagged_with", ToEntityID: middle.ID})

	if err := rf.DeleteKeyword(ctx, middle.ID); err != nil {
		t.Fatalf("DeleteKeyword: %v", err)
	}
	if _, err := rf.GetKeyword(ctx, middle.ID); !errors.Is(err, ErrKeywordNotFound) {
		t.Fatalf("deleted keyword still readable: %v", err)
	}
	reparented, err := rf.GetKeyword(ctx, leaf.ID)
	if err != nil || reparented.ParentID != top.ID {
		t.Fatalf("leaf after delete = %+v, err=%v, want parent %s", reparented, err, top.ID)
	}
	found, err := rf.SearchByKeywords(ctx, SearchByKeywordsRequest{BranchID: branch.ID, Keywords: []string{middle.ID}})
	if err != nil || len(found.Nodes) != 0 {
		t.Fatalf("tag on deleted keyword should be gone: %+v, err=%v", found, err)
	}

	if err := rf.DeleteKeyword(ctx, middle.ID); !errors.Is(err, ErrKeywordNotFound) {
		t.Fatalf("second delete: want ErrKeywordNotFound, got %v", err)
	}
}

func TestRepoFileFeatures_CreateEdge(t *testing.T) {
	ctx := context.Background()
	rf, _, branch := newFeatureRepo(t)
	writeFeatureFiles(t, rf, branch.ID, []featureFile{
		{Path: "api.go", Content: "package api"},
		{Path: "api.md", Content: "# api"},
	})
	api := blobOnBranch(t, rf, branch.ID, "api.go")
	doc := blobOnBranch(t, rf, branch.ID, "api.md")
	kw := mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "api"})

	mustCreateEdge(t, rf, CreateEdgeRequest{BranchID: branch.ID, FromEntityID: doc.ID, RelationshipName: "documents", ToEntityID: api.ID})
	mustCreateEdge(t, rf, CreateEdgeRequest{BranchID: branch.ID, FromEntityID: doc.ID, RelationshipName: "documents", ToEntityID: api.ID})
	mustCreateEdge(t, rf, CreateEdgeRequest{
		BranchID: branch.ID, FromEntityID: api.ID, RelationshipName: "tagged_with", ToEntityID: kw.ID,
		Properties: map[string]any{"signal": "authority"},
	})

	graph, err := rf.GetNeighborhood(ctx, branch.ID, doc.ID, 1)
	if err != nil {
		t.Fatalf("GetNeighborhood: %v", err)
	}
	documents := 0
	for _, e := range graph.Edges {
		if e.Name == "documents" && e.FromID == doc.ID && e.ToID == api.ID {
			documents++
		}
	}
	if documents != 1 {
		t.Fatalf("expected exactly one documents edge doc->api, got %d in %+v", documents, graph.Edges)
	}

	tagged, err := rf.QueryGraph(ctx, QueryGraphRequest{BranchID: branch.ID, Signals: []string{"authority"}})
	if err != nil {
		t.Fatalf("QueryGraph: %v", err)
	}
	if got := nodeIDs(tagged); !equalStrings(got, []string{api.ID}) {
		t.Fatalf("authority-tagged nodes = %v, want [%s]", got, api.ID)
	}

	if err := rf.CreateEdge(ctx, CreateEdgeRequest{BranchID: branch.ID, FromEntityID: doc.ID, RelationshipName: "not-a-real-edge", ToEntityID: api.ID}); !errors.Is(err, ErrInvalidRelationship) {
		t.Fatalf("invalid relationship: want ErrInvalidRelationship, got %v", err)
	}
	if err := rf.CreateEdge(ctx, CreateEdgeRequest{BranchID: "missing", FromEntityID: doc.ID, RelationshipName: "documents", ToEntityID: api.ID}); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("missing branch: want ErrBranchNotFound, got %v", err)
	}
}

func TestRepoFileFeatures_DeleteEdge(t *testing.T) {
	ctx := context.Background()
	rf, _, branch := newFeatureRepo(t)
	writeFeatureFiles(t, rf, branch.ID, []featureFile{
		{Path: "api.go", Content: "package api"},
		{Path: "api.md", Content: "# api"},
	})
	api := blobOnBranch(t, rf, branch.ID, "api.go")
	doc := blobOnBranch(t, rf, branch.ID, "api.md")
	kw := mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "api"})
	mustCreateEdge(t, rf, CreateEdgeRequest{BranchID: branch.ID, FromEntityID: doc.ID, RelationshipName: "documents", ToEntityID: api.ID})
	mustCreateEdge(t, rf, CreateEdgeRequest{BranchID: branch.ID, FromEntityID: api.ID, RelationshipName: "tagged_with", ToEntityID: kw.ID})

	if err := rf.DeleteEdge(ctx, DeleteEdgeRequest{BranchID: branch.ID, FromEntityID: doc.ID, RelationshipName: "documents", ToEntityID: api.ID}); err != nil {
		t.Fatalf("DeleteEdge documents: %v", err)
	}
	graph, err := rf.GetNeighborhood(ctx, branch.ID, doc.ID, 1)
	if err != nil {
		t.Fatalf("GetNeighborhood: %v", err)
	}
	for _, e := range graph.Edges {
		if e.Name == "documents" {
			t.Fatalf("documents edge still present: %+v", graph.Edges)
		}
	}

	if err := rf.DeleteEdge(ctx, DeleteEdgeRequest{BranchID: branch.ID, FromEntityID: api.ID, RelationshipName: "tagged_with", ToEntityID: kw.ID}); err != nil {
		t.Fatalf("DeleteEdge tagged_with: %v", err)
	}
	found, err := rf.SearchByKeywords(ctx, SearchByKeywordsRequest{BranchID: branch.ID, Keywords: []string{kw.ID}})
	if err != nil || len(found.Nodes) != 0 {
		t.Fatalf("tag still searchable after delete: %+v, err=%v", found, err)
	}

	if err := rf.DeleteEdge(ctx, DeleteEdgeRequest{BranchID: branch.ID, FromEntityID: doc.ID, RelationshipName: "documents", ToEntityID: api.ID}); !errors.Is(err, ErrEdgeNotFound) {
		t.Fatalf("second delete: want ErrEdgeNotFound, got %v", err)
	}
	if err := rf.DeleteEdge(ctx, DeleteEdgeRequest{BranchID: branch.ID, FromEntityID: doc.ID, RelationshipName: "bogus", ToEntityID: api.ID}); !errors.Is(err, ErrInvalidRelationship) {
		t.Fatalf("bogus name: want ErrInvalidRelationship, got %v", err)
	}
}

func TestRepoFileFeatures_GetNeighborhood(t *testing.T) {
	ctx := context.Background()
	rf, _, branch := newFeatureRepo(t)
	writeFeatureFiles(t, rf, branch.ID, []featureFile{
		{Path: "a.md", Content: "a"},
		{Path: "b.md", Content: "b"},
		{Path: "c.md", Content: "c"},
	})
	a := blobOnBranch(t, rf, branch.ID, "a.md")
	b := blobOnBranch(t, rf, branch.ID, "b.md")
	c := blobOnBranch(t, rf, branch.ID, "c.md")
	kw := mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "topic"})
	mustCreateEdge(t, rf, CreateEdgeRequest{BranchID: branch.ID, FromEntityID: a.ID, RelationshipName: "depends_on", ToEntityID: b.ID})
	mustCreateEdge(t, rf, CreateEdgeRequest{BranchID: branch.ID, FromEntityID: a.ID, RelationshipName: "tagged_with", ToEntityID: kw.ID})

	has := func(result GraphResult, id string) bool {
		for _, n := range result.Nodes {
			if n.ID == id {
				return true
			}
		}
		return false
	}
	hasEdge := func(result GraphResult, name, from, to string) bool {
		for _, e := range result.Edges {
			if e.Name == name && e.FromID == from && e.ToID == to {
				return true
			}
		}
		return false
	}

	depthOne, err := rf.GetNeighborhood(ctx, branch.ID, a.ID, 1)
	if err != nil {
		t.Fatalf("depth 1: %v", err)
	}
	if depthOne.Nodes[0].ID != a.ID || depthOne.Nodes[0].TypeID != "Blob" {
		t.Fatalf("start node should come first as a Blob, got %+v", depthOne.Nodes[0])
	}
	if !has(depthOne, b.ID) || !has(depthOne, kw.ID) || has(depthOne, c.ID) {
		t.Fatalf("depth 1 nodes = %v, want a, b and the keyword but not c", nodeIDs(depthOne))
	}
	if !hasEdge(depthOne, "depends_on", a.ID, b.ID) || !hasEdge(depthOne, "tagged_with", a.ID, kw.ID) {
		t.Fatalf("depth 1 edges = %+v", depthOne.Edges)
	}

	incoming, err := rf.GetNeighborhood(ctx, branch.ID, b.ID, 1)
	if err != nil {
		t.Fatalf("incoming traversal: %v", err)
	}
	if !has(incoming, a.ID) || !hasEdge(incoming, "depends_on", a.ID, b.ID) || has(incoming, c.ID) {
		t.Fatalf("incoming traversal nodes = %v edges = %+v", nodeIDs(incoming), incoming.Edges)
	}

	deep, err := rf.GetNeighborhood(ctx, branch.ID, a.ID, 10)
	if err != nil {
		t.Fatalf("clamped depth: %v", err)
	}
	if !has(deep, c.ID) {
		t.Fatalf("depth 10 (clamped to 3) should reach c through the shared tree, nodes = %v", nodeIDs(deep))
	}

	byPath, err := rf.GetNeighborhood(ctx, branch.ID, "a.md", 1)
	if err != nil {
		t.Fatalf("by path: %v", err)
	}
	if byPath.Nodes[0].ID != a.ID || !has(byPath, b.ID) {
		t.Fatalf("by path nodes = %v", nodeIDs(byPath))
	}

	if _, err := rf.GetNeighborhood(ctx, branch.ID, "ghost.md", 1); !errors.Is(err, ErrEntityNotFound) {
		t.Fatalf("unknown entity: want ErrEntityNotFound, got %v", err)
	}
	if _, err := rf.GetNeighborhood(ctx, "missing", a.ID, 1); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("unknown branch: want ErrBranchNotFound, got %v", err)
	}
}

func TestRepoFileFeatures_SearchByKeywords(t *testing.T) {
	ctx := context.Background()
	rf, _, branch := newFeatureRepo(t)
	writeFeatureFiles(t, rf, branch.ID, []featureFile{
		{Path: "auth.go", Content: "package auth"},
		{Path: "oauth.go", Content: "package oauth"},
		{Path: "billing.go", Content: "package billing"},
	})
	auth := blobOnBranch(t, rf, branch.ID, "auth.go")
	oauth := blobOnBranch(t, rf, branch.ID, "oauth.go")
	billing := blobOnBranch(t, rf, branch.ID, "billing.go")

	security := mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "security"})
	authKw := mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "auth", ParentID: security.ID})
	money := mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "money"})
	tag := func(blobID, keywordID string) {
		mustCreateEdge(t, rf, CreateEdgeRequest{BranchID: branch.ID, FromEntityID: blobID, RelationshipName: "tagged_with", ToEntityID: keywordID})
	}
	tag(auth.ID, authKw.ID)
	tag(auth.ID, money.ID)
	tag(oauth.ID, authKw.ID)
	tag(billing.ID, money.ID)
	tag(oauth.ID, security.ID)

	search := func(req SearchByKeywordsRequest) []string {
		t.Helper()
		req.BranchID = branch.ID
		result, err := rf.SearchByKeywords(ctx, req)
		if err != nil {
			t.Fatalf("SearchByKeywords %+v: %v", req, err)
		}
		return nodePaths(result)
	}

	if got := search(SearchByKeywordsRequest{Keywords: []string{authKw.ID}}); !equalStrings(got, []string{"auth.go", "oauth.go"}) {
		t.Fatalf("single keyword = %v", got)
	}
	if got := search(SearchByKeywordsRequest{Keywords: []string{authKw.ID, money.ID}}); !equalStrings(got, []string{"auth.go", "billing.go", "oauth.go"}) {
		t.Fatalf("default OR = %v", got)
	}
	if got := search(SearchByKeywordsRequest{Keywords: []string{authKw.ID, money.ID}, MatchMode: KeywordMatchModeAND}); !equalStrings(got, []string{"auth.go"}) {
		t.Fatalf("AND = %v", got)
	}
	if got := search(SearchByKeywordsRequest{Keywords: []string{security.ID}}); !equalStrings(got, []string{"oauth.go"}) {
		t.Fatalf("no cascade = %v", got)
	}
	if got := search(SearchByKeywordsRequest{Keywords: []string{security.ID}, Cascade: true}); !equalStrings(got, []string{"auth.go", "oauth.go"}) {
		t.Fatalf("cascade = %v", got)
	}
	if got := search(SearchByKeywordsRequest{Keywords: nil}); len(got) != 0 {
		t.Fatalf("no keywords = %v", got)
	}

	if _, err := rf.SearchByKeywords(ctx, SearchByKeywordsRequest{BranchID: "missing", Keywords: []string{authKw.ID}}); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("missing branch: want ErrBranchNotFound, got %v", err)
	}
}

func TestRepoFileFeatures_QueryGraph(t *testing.T) {
	ctx := context.Background()
	rf, _, branch := newFeatureRepo(t)
	writeFeatureFiles(t, rf, branch.ID, []featureFile{
		{Path: "internal/server.go", Content: "package server"},
		{Path: "internal/store.go", Content: "package store"},
		{Path: "cmd/main.go", Content: "package main"},
		{Path: "docs/guide.md", Content: "# guide"},
	})
	server := blobOnBranch(t, rf, branch.ID, "internal/server.go")
	store := blobOnBranch(t, rf, branch.ID, "internal/store.go")
	mainBlob := blobOnBranch(t, rf, branch.ID, "cmd/main.go")
	guide := blobOnBranch(t, rf, branch.ID, "docs/guide.md")

	kw := mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "core"})
	other := mustCreateKeyword(t, rf, CreateKeywordRequest{Name: "extra"})
	mustCreateEdge(t, rf, CreateEdgeRequest{BranchID: branch.ID, FromEntityID: server.ID, RelationshipName: "tagged_with", ToEntityID: kw.ID, Properties: map[string]any{"signal": "authority"}})
	mustCreateEdge(t, rf, CreateEdgeRequest{BranchID: branch.ID, FromEntityID: store.ID, RelationshipName: "tagged_with", ToEntityID: kw.ID, Properties: map[string]any{"signal": "surface"}})
	mustCreateEdge(t, rf, CreateEdgeRequest{BranchID: branch.ID, FromEntityID: mainBlob.ID, RelationshipName: "tagged_with", ToEntityID: other.ID, Properties: map[string]any{"signal": "structural"}})
	mustCreateEdge(t, rf, CreateEdgeRequest{BranchID: branch.ID, FromEntityID: guide.ID, RelationshipName: "documents", ToEntityID: server.ID})
	mustCreateEdge(t, rf, CreateEdgeRequest{BranchID: branch.ID, FromEntityID: server.ID, RelationshipName: "depends_on", ToEntityID: store.ID})

	query := func(req QueryGraphRequest) GraphResult {
		t.Helper()
		req.BranchID = branch.ID
		result, err := rf.QueryGraph(ctx, req)
		if err != nil {
			t.Fatalf("QueryGraph %+v: %v", req, err)
		}
		return result
	}

	all := query(QueryGraphRequest{})
	if got := nodePaths(all); !equalStrings(got, []string{"cmd/main.go", "docs/guide.md", "internal/server.go", "internal/store.go"}) {
		t.Fatalf("all nodes = %v", got)
	}
	wantOrder := []string{"internal/server.go", "cmd/main.go", "internal/store.go"}
	for i, want := range wantOrder {
		if p, _ := all.Nodes[i].Properties["path"].(string); p != want {
			t.Fatalf("signal order position %d = %q, want %q (nodes %v)", i, p, want, nodePaths(all))
		}
	}
	if len(all.Edges) != 2 {
		t.Fatalf("all edges = %+v, want documents and depends_on", all.Edges)
	}

	if got := nodePaths(query(QueryGraphRequest{Limit: 2})); len(got) != 2 {
		t.Fatalf("limit 2 = %v", got)
	}
	if got := nodePaths(query(QueryGraphRequest{FileTypes: []string{".md"}})); !equalStrings(got, []string{"docs/guide.md"}) {
		t.Fatalf("file types = %v", got)
	}
	if got := nodePaths(query(QueryGraphRequest{Folders: []string{"internal/"}})); !equalStrings(got, []string{"internal/server.go", "internal/store.go"}) {
		t.Fatalf("folders = %v", got)
	}
	if got := nodePaths(query(QueryGraphRequest{KeywordIDs: []string{kw.ID}})); !equalStrings(got, []string{"internal/server.go", "internal/store.go"}) {
		t.Fatalf("keyword ids = %v", got)
	}
	if got := nodePaths(query(QueryGraphRequest{Signals: []string{"structural", "authority"}})); !equalStrings(got, []string{"cmd/main.go", "internal/server.go"}) {
		t.Fatalf("signals = %v", got)
	}

	dependsOnly := query(QueryGraphRequest{Relationships: []string{"depends_on"}})
	if len(dependsOnly.Edges) != 1 || dependsOnly.Edges[0].Name != "depends_on" || dependsOnly.Edges[0].FromID != server.ID || dependsOnly.Edges[0].ToID != store.ID {
		t.Fatalf("relationship filter edges = %+v", dependsOnly.Edges)
	}

	if _, err := rf.QueryGraph(ctx, QueryGraphRequest{BranchID: "missing"}); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("missing branch: want ErrBranchNotFound, got %v", err)
	}
}

func TestRepoFileFeatures_SearchBlobs(t *testing.T) {
	ctx := context.Background()
	s, err := LoadSpec(filepath.Join("spec", "examples", "engineering.git.json"))
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	searcher := &recordingBlobSearcher{results: []BlobSearchResult{
		{ID: "b1", Path: "docs/guide.md", Name: "guide.md", Extension: "md", Snippet: "the guide", Score: 0.9},
		{ID: "b2", Path: "src/main.go", Name: "main.go", Extension: "go", Score: 0.4},
	}}
	rf, err := OpenRepoFile(filepath.Join(t.TempDir(), "search.db"), s, nil, nil, searcher)
	if err != nil {
		t.Fatalf("OpenRepoFile: %v", err)
	}
	t.Cleanup(func() { _ = rf.Close() })

	got, err := rf.SearchBlobs(ctx, SearchBlobsRequest{Query: "guide", RepositoryName: featureRepoName})
	if err != nil {
		t.Fatalf("SearchBlobs: %v", err)
	}
	if len(got) != 2 || got[0].ID != "b1" || got[0].Score != 0.9 || got[0].Snippet != "the guide" || got[1].Path != "src/main.go" {
		t.Fatalf("results = %+v", got)
	}
	if searcher.query != "guide" || searcher.limit != 20 {
		t.Fatalf("searcher saw query=%q limit=%d, want guide/20", searcher.query, searcher.limit)
	}

	if _, err := rf.SearchBlobs(ctx, SearchBlobsRequest{Query: "guide", RepositoryName: featureRepoName, Limit: 5}); err != nil {
		t.Fatalf("SearchBlobs with limit: %v", err)
	}
	if searcher.limit != 5 || searcher.calls != 2 {
		t.Fatalf("searcher limit=%d calls=%d, want 5/2", searcher.limit, searcher.calls)
	}

	if _, err := rf.SearchBlobs(ctx, SearchBlobsRequest{RepositoryName: featureRepoName}); err == nil {
		t.Fatalf("empty query should be refused")
	}
	if searcher.calls != 2 {
		t.Fatalf("empty query reached the searcher")
	}
}

func TestRepoFileFeatures_SearchBlobsWithoutSearcher(t *testing.T) {
	rf, _, _ := newFeatureRepo(t)
	got, err := rf.SearchBlobs(context.Background(), SearchBlobsRequest{Query: "anything", RepositoryName: featureRepoName})
	if err != nil {
		t.Fatalf("SearchBlobs: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("expected an empty non-nil slice, got %#v", got)
	}
}
