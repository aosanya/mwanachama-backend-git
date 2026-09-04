package mwanachamagit

import (
	"context"
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/gormstore"
	"github.com/aosanya/mwanachama-backend-git/models"
)

// TestNewGitManagerSatisfiesInterface locks in that *gitManager fully
// implements GitManager. IndexPushedBranch is a deliberate G6 stub;
// SearchBlobs gracefully no-ops with a nil searcher.
func TestNewGitManagerSatisfiesInterface(t *testing.T) {
	m := newTestManager(t)
	var gm GitManager = m
	if _, err := gm.ListRepositories(context.Background()); err != nil {
		t.Fatalf("ListRepositories on a fresh manager: %v", err)
	}
	if err := gm.IndexPushedBranch(context.Background(), "repo", "refs/heads/main", "", "abc"); !errors.Is(err, ErrPushIndexingNotImplemented) {
		t.Fatalf("expected ErrPushIndexingNotImplemented, got %v", err)
	}
	results, err := gm.SearchBlobs(context.Background(), SearchBlobsRequest{Query: "auth"})
	if err != nil || len(results) != 0 {
		t.Fatalf("expected graceful empty result with no searcher, got %+v (err=%v)", results, err)
	}
}

// createCommit is a test-only stand-in for WriteFile: it creates a bare
// Commit row linked to repoID so branch/merge flows have something real to
// advance HEAD to.
func createCommit(t *testing.T, m *gitManager, repoID, sha string) gormstore.CommitRow {
	t.Helper()
	row := gormstore.CommitToRow(models.Commit{SHA: sha, Message: "test commit " + sha, CreatedAt: models.NowRFC3339()})
	row.RepositoryID = gormstore.StringToNullable(repoID)
	if err := m.db.WithContext(context.Background()).Table(m.tables.Commits).Create(&row).Error; err != nil {
		t.Fatalf("createCommit: %v", err)
	}
	return row
}

func TestInitRepoAndBranchLifecycle(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)

	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	if repo.DefaultBranch != "main" {
		t.Fatalf("expected default branch 'main', got %q", repo.DefaultBranch)
	}

	if _, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"}); !errors.Is(err, ErrRepoAlreadyExists) {
		t.Fatalf("expected ErrRepoAlreadyExists, got %v", err)
	}

	branches, err := m.ListBranches(ctx, repo.ID)
	if err != nil || len(branches) != 1 || !branches[0].IsDefault {
		t.Fatalf("expected one default branch, got %+v (err=%v)", branches, err)
	}
	defaultBranch := branches[0]

	feature, err := m.CreateBranch(ctx, CreateBranchRequest{
		RepositoryID: repo.ID,
		Name:         "feature/x",
	})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if feature.IsDefault {
		t.Fatalf("new branch should not be default")
	}

	if _, err := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "feature/x"}); !errors.Is(err, ErrBranchExists) {
		t.Fatalf("expected ErrBranchExists, got %v", err)
	}

	if err := m.DeleteBranch(ctx, defaultBranch.ID); !errors.Is(err, ErrDefaultBranchDeleteForbidden) {
		t.Fatalf("expected ErrDefaultBranchDeleteForbidden, got %v", err)
	}
	if err := m.DeleteBranch(ctx, feature.ID); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	if _, err := m.GetBranch(ctx, feature.ID); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("expected ErrBranchNotFound after delete, got %v", err)
	}
}

// TestGetByIDRejectsWrongEntityType guards against fetching a row from the
// wrong table by ID: a Commit ID passed to GetBranch/GetTag/GetKeyword must
// come back as not-found, since each type now lives in its own table (no
// shared ID space to accidentally match against).
func TestGetByIDRejectsWrongEntityType(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)

	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	commit := createCommit(t, m, repo.ID, "abc123")

	if _, err := m.GetBranch(ctx, commit.ID); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("GetBranch(commitID): expected ErrBranchNotFound, got %v", err)
	}
	if _, err := m.GetTag(ctx, commit.ID); !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("GetTag(commitID): expected ErrTagNotFound, got %v", err)
	}
	if _, err := m.GetKeyword(ctx, commit.ID); !errors.Is(err, ErrKeywordNotFound) {
		t.Fatalf("GetKeyword(commitID): expected ErrKeywordNotFound, got %v", err)
	}
}

func TestMergeBranchAdvancesDefaultHead(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)

	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	feature, err := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "feature/x"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	commit := createCommit(t, m, repo.ID, "deadbeef")
	if _, err := m.advanceBranchHead(ctx, feature.ID, commit.ID, ""); err != nil {
		t.Fatalf("advanceBranchHead: %v", err)
	}

	updated, err := m.MergeBranch(ctx, feature.ID)
	if err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	if !updated.IsDefault {
		t.Fatalf("MergeBranch should return the default branch")
	}
	if updated.HeadCommitID != commit.ID {
		t.Fatalf("expected default HEAD %s, got %s", commit.ID, updated.HeadCommitID)
	}
	if updated.SHA != "deadbeef" {
		t.Fatalf("expected default SHA to be copied from commit, got %q", updated.SHA)
	}
}

func TestMergeRequestLifecycle(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)

	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	feature, err := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "feature/x"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	commit := createCommit(t, m, repo.ID, "cafebabe")
	if _, err := m.advanceBranchHead(ctx, feature.ID, commit.ID, ""); err != nil {
		t.Fatalf("advanceBranchHead: %v", err)
	}

	mr, err := m.CreateMergeRequest(ctx, CreateMergeRequestRequest{
		RepositoryID:   repo.ID,
		Title:          "Ship the widget",
		SourceBranchID: feature.ID,
	})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}
	if mr.Status != models.MergeRequestStatusOpen {
		t.Fatalf("expected open status, got %q", mr.Status)
	}

	completed, err := m.CompleteMergeRequest(ctx, mr.ID)
	if err != nil {
		t.Fatalf("CompleteMergeRequest: %v", err)
	}
	if completed.Status != models.MergeRequestStatusMerged {
		t.Fatalf("expected merged status, got %q", completed.Status)
	}
	if completed.MergedCommitSHA != "cafebabe" {
		t.Fatalf("expected merged_commit_sha cafebabe, got %q", completed.MergedCommitSHA)
	}

	if _, err := m.CompleteMergeRequest(ctx, mr.ID); !errors.Is(err, ErrMergeRequestNotOpen) {
		t.Fatalf("expected ErrMergeRequestNotOpen on re-complete, got %v", err)
	}
}

func TestCreateTag(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)

	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	commit := createCommit(t, m, repo.ID, "1234567")

	tag, err := m.CreateTag(ctx, CreateTagRequest{RepositoryID: repo.ID, Name: "v1.0.0", CommitID: commit.ID})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if tag.SHA != "1234567" {
		t.Fatalf("expected tag sha copied from commit, got %q", tag.SHA)
	}
	if _, err := m.CreateTag(ctx, CreateTagRequest{RepositoryID: repo.ID, Name: "v1.0.0", CommitID: commit.ID}); !errors.Is(err, ErrTagAlreadyExists) {
		t.Fatalf("expected ErrTagAlreadyExists, got %v", err)
	}

	tags, err := m.ListTags(ctx, repo.ID)
	if err != nil || len(tags) != 1 {
		t.Fatalf("expected one tag, got %+v (err=%v)", tags, err)
	}

	if err := m.DeleteTag(ctx, tag.ID); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	if _, err := m.GetTag(ctx, tag.ID); !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("expected ErrTagNotFound after delete, got %v", err)
	}
}

func TestKeywordTreeAndDeleteReparents(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)

	root, err := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "domain"})
	if err != nil {
		t.Fatalf("CreateKeyword root: %v", err)
	}
	child, err := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "auth", ParentID: root.ID})
	if err != nil {
		t.Fatalf("CreateKeyword child: %v", err)
	}
	grandchild, err := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "oauth", ParentID: child.ID})
	if err != nil {
		t.Fatalf("CreateKeyword grandchild: %v", err)
	}

	if _, err := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "auth", ParentID: root.ID}); !errors.Is(err, ErrKeywordAlreadyExists) {
		t.Fatalf("expected ErrKeywordAlreadyExists, got %v", err)
	}

	tree, err := m.GetKeywordTree(ctx, root.ID)
	if err != nil || len(tree) != 1 || len(tree[0].Children) != 1 {
		t.Fatalf("unexpected tree shape: %+v (err=%v)", tree, err)
	}

	// Deleting the middle node should re-parent grandchild onto root.
	if err := m.DeleteKeyword(ctx, child.ID); err != nil {
		t.Fatalf("DeleteKeyword: %v", err)
	}
	got, err := m.GetKeyword(ctx, grandchild.ID)
	if err != nil {
		t.Fatalf("GetKeyword grandchild: %v", err)
	}
	if got.ParentID != root.ID {
		t.Fatalf("expected grandchild reparented to root %s, got %q", root.ID, got.ParentID)
	}
}

func TestCreateEdgeAndSearchByKeywords(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)

	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, _ := m.ListBranches(ctx, repo.ID)
	branch := branches[0]

	kw, err := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "auth"})
	if err != nil {
		t.Fatalf("CreateKeyword: %v", err)
	}
	blobRow := gormstore.BlobToRow(models.Blob{Path: "auth.go", Name: "auth.go", CreatedAt: models.NowRFC3339()})
	if err := m.db.WithContext(ctx).Table(m.tables.Blobs).Create(&blobRow).Error; err != nil {
		t.Fatalf("create blob: %v", err)
	}

	if err := m.CreateEdge(ctx, CreateEdgeRequest{
		BranchID:         branch.ID,
		FromEntityID:     blobRow.ID,
		RelationshipName: "tagged_with",
		ToEntityID:       kw.ID,
	}); err != nil {
		t.Fatalf("CreateEdge: %v", err)
	}

	if err := m.CreateEdge(ctx, CreateEdgeRequest{
		BranchID:         branch.ID,
		FromEntityID:     blobRow.ID,
		RelationshipName: "not-a-real-edge",
		ToEntityID:       kw.ID,
	}); !errors.Is(err, ErrInvalidRelationship) {
		t.Fatalf("expected ErrInvalidRelationship, got %v", err)
	}

	result, err := m.SearchByKeywords(ctx, SearchByKeywordsRequest{
		BranchID: branch.ID,
		Keywords: []string{kw.ID},
	})
	if err != nil {
		t.Fatalf("SearchByKeywords: %v", err)
	}
	if len(result.Nodes) != 1 || result.Nodes[0].ID != blobRow.ID {
		t.Fatalf("expected blob %s in search results, got %+v", blobRow.ID, result.Nodes)
	}

	if err := m.DeleteEdge(ctx, DeleteEdgeRequest{
		BranchID:         branch.ID,
		FromEntityID:     blobRow.ID,
		RelationshipName: "tagged_with",
		ToEntityID:       kw.ID,
	}); err != nil {
		t.Fatalf("DeleteEdge: %v", err)
	}
	if err := m.DeleteEdge(ctx, DeleteEdgeRequest{
		BranchID:         branch.ID,
		FromEntityID:     blobRow.ID,
		RelationshipName: "tagged_with",
		ToEntityID:       kw.ID,
	}); !errors.Is(err, ErrEdgeNotFound) {
		t.Fatalf("expected ErrEdgeNotFound on redundant delete, got %v", err)
	}
}

func TestRollbackByWorkflowRun(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	const runID = "run-123"

	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branch, err := m.CreateBranch(ctx, CreateBranchRequest{
		RepositoryID:  repo.ID,
		Name:          "task/abc",
		WorkflowRunID: runID,
	})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	commit := createCommit(t, m, repo.ID, "0ff1ce")
	if _, err := m.advanceBranchHead(ctx, branch.ID, commit.ID, ""); err != nil {
		t.Fatalf("advanceBranchHead: %v", err)
	}
	mr, err := m.CreateMergeRequest(ctx, CreateMergeRequestRequest{
		RepositoryID:   repo.ID,
		Title:          "Ship it",
		SourceBranchID: branch.ID,
		WorkflowRunID:  runID,
	})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}
	if _, err := m.CompleteMergeRequest(ctx, mr.ID); err != nil {
		t.Fatalf("CompleteMergeRequest: %v", err)
	}

	result, err := m.RollbackByWorkflowRun(ctx, runID)
	if err != nil {
		t.Fatalf("RollbackByWorkflowRun: %v", err)
	}
	if result.BranchesDeleted != 1 {
		t.Fatalf("expected 1 branch deleted, got %d", result.BranchesDeleted)
	}
	if result.MergeRequestsRolledBack != 1 {
		t.Fatalf("expected 1 MR rolled back, got %d", result.MergeRequestsRolledBack)
	}
	if result.DefaultBranchesSkipped != 0 {
		t.Fatalf("expected 0 default branches skipped, got %d", result.DefaultBranchesSkipped)
	}

	if _, err := m.GetBranch(ctx, branch.ID); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("expected task branch to be gone, got %v", err)
	}
	rolledMR, err := m.GetMergeRequest(ctx, mr.ID)
	if err != nil {
		t.Fatalf("GetMergeRequest: %v", err)
	}
	if rolledMR.Status != models.MergeRequestStatusRolledBack {
		t.Fatalf("expected rolled_back status, got %q", rolledMR.Status)
	}

	// Idempotent re-invocation: nothing left to roll back.
	again, err := m.RollbackByWorkflowRun(ctx, runID)
	if err != nil {
		t.Fatalf("RollbackByWorkflowRun (2nd): %v", err)
	}
	if again.BranchesDeleted != 0 || again.MergeRequestsRolledBack != 0 {
		t.Fatalf("expected no-op on re-invocation, got %+v", again)
	}

	if _, err := m.RollbackByWorkflowRun(ctx, ""); !errors.Is(err, ErrWorkflowRunIDRequired) {
		t.Fatalf("expected ErrWorkflowRunIDRequired, got %v", err)
	}
}
