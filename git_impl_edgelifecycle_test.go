// git_impl_edgelifecycle_test.go — G9 fidelity port of CodeValdGit's
// git_edgelifecycle_test.go: the GIT-022 edge lifecycle hooks.
//
//   - GIT-022a: tagged_with and references edges replicated to the default
//     branch after MergeBranch.
//   - GIT-022b: branch-scoped edges deleted when DeleteBranch is called
//     without a preceding merge, and only those scoped to the deleted branch.
//   - GIT-022c: blob-scoped edges removed when DeleteFile is called.
package mwanachamagit

import (
	"context"
	"testing"

	"github.com/aosanya/mwanachama-go-shared/entitygraph"
)

// countEdgesByBranchID returns the number of relationships with the given
// name and FromID whose "branch_id" property equals targetBranchID.
func countEdgesByBranchID(t *testing.T, m *gitManager, fromID, edgeName, targetBranchID string) int {
	t.Helper()
	ctx := context.Background()
	rels, err := m.dm.ListRelationships(ctx, entitygraph.RelationshipFilter{
		AgencyID: m.agencyID,
		FromID:   fromID,
		Name:     edgeName,
	})
	if err != nil {
		t.Fatalf("countEdgesByBranchID: ListRelationships: %v", err)
	}
	count := 0
	for _, r := range rels {
		if v, ok := r.Properties["branch_id"]; ok && v == targetBranchID {
			count++
		}
	}
	return count
}

// writeTestFile writes a file on the given branch and returns the resulting
// Blob ID (resolved from the branch's HEAD after the write).
func writeTestFile(t *testing.T, m *gitManager, branchID, path, content string) string {
	t.Helper()
	ctx := context.Background()
	if _, err := m.WriteFile(ctx, WriteFileRequest{
		BranchID:   branchID,
		Path:       path,
		Content:    content,
		AuthorName: "test-author",
	}); err != nil {
		t.Fatalf("WriteFile %q on branch %q: %v", path, branchID, err)
	}
	blob, err := m.ReadFile(ctx, branchID, path)
	if err != nil {
		t.Fatalf("ReadFile %q after WriteFile: %v", path, err)
	}
	return blob.ID
}

// ── GIT-022a: Replicate edges on MergeBranch ─────────────────────────────────

func TestEdgeLifecycle_022a_TaggedWith(t *testing.T) {
	ctx := context.Background()
	m := newTestManager()
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "testrepo"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, _ := m.ListBranches(ctx, repo.ID)
	defaultBranchID := branches[0].ID

	taskBranch, err := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "task-kw-001"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	blobID := writeTestFile(t, m, taskBranch.ID, "docs/readme.md", "# Hello")

	kw, err := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "docs"})
	if err != nil {
		t.Fatalf("CreateKeyword: %v", err)
	}
	if err := m.CreateEdge(ctx, CreateEdgeRequest{
		BranchID: taskBranch.ID, FromEntityID: blobID, ToEntityID: kw.ID, RelationshipName: "tagged_with",
	}); err != nil {
		t.Fatalf("CreateEdge tagged_with: %v", err)
	}

	if got := countEdgesByBranchID(t, m, blobID, "tagged_with", taskBranch.ID); got != 1 {
		t.Fatalf("before merge: want 1 tagged_with on task branch, got %d", got)
	}
	if got := countEdgesByBranchID(t, m, blobID, "tagged_with", defaultBranchID); got != 0 {
		t.Fatalf("before merge: want 0 tagged_with on default branch, got %d", got)
	}

	if _, err := m.MergeBranch(ctx, taskBranch.ID); err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}

	if got := countEdgesByBranchID(t, m, blobID, "tagged_with", defaultBranchID); got != 1 {
		t.Fatalf("after merge: want 1 tagged_with on default branch, got %d", got)
	}
	if got := countEdgesByBranchID(t, m, blobID, "tagged_with", taskBranch.ID); got != 1 {
		t.Fatalf("after merge: task-branch tagged_with should still exist, got %d", got)
	}
}

// TestEdgeLifecycle_022a_References verifies that a references edge (with a
// descriptor property) is replicated to the default branch after merge, and
// carries its descriptor along.
func TestEdgeLifecycle_022a_References(t *testing.T) {
	ctx := context.Background()
	m := newTestManager()
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "testrepo"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	branches, _ := m.ListBranches(ctx, repo.ID)
	defaultBranchID := branches[0].ID

	taskBranch, err := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "task-ref-001"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	blobA := writeTestFile(t, m, taskBranch.ID, "src/a.go", "package a")
	blobB := writeTestFile(t, m, taskBranch.ID, "src/b.go", "package b")

	if err := m.CreateEdge(ctx, CreateEdgeRequest{
		BranchID: taskBranch.ID, FromEntityID: blobB, ToEntityID: blobA,
		RelationshipName: "references", Properties: map[string]any{"descriptor": "depends_on"},
	}); err != nil {
		t.Fatalf("CreateEdge references: %v", err)
	}

	if got := countEdgesByBranchID(t, m, blobB, "references", defaultBranchID); got != 0 {
		t.Fatalf("before merge: want 0 references on default branch, got %d", got)
	}

	if _, err := m.MergeBranch(ctx, taskBranch.ID); err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}

	rels, err := m.dm.ListRelationships(ctx, entitygraph.RelationshipFilter{
		AgencyID: m.agencyID, FromID: blobB, Name: "references",
	})
	if err != nil {
		t.Fatalf("ListRelationships: %v", err)
	}
	var found bool
	for _, r := range rels {
		if v, ok := r.Properties["branch_id"]; ok && v == defaultBranchID {
			found = true
			if d, ok := r.Properties["descriptor"]; !ok || d != "depends_on" {
				t.Errorf("replicated references edge: descriptor = %v, want %q", d, "depends_on")
			}
		}
	}
	if !found {
		t.Fatal("after merge: no references edge found scoped to default branch")
	}
}

func TestEdgeLifecycle_022a_NoEdges(t *testing.T) {
	ctx := context.Background()
	m := newTestManager()
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "testrepo"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	taskBranch, err := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "task-noedge-001"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	writeTestFile(t, m, taskBranch.ID, "docs/readme.md", "# Hello")

	if _, err := m.MergeBranch(ctx, taskBranch.ID); err != nil {
		t.Fatalf("MergeBranch (no edges): %v", err)
	}
}

// ── GIT-022b: Delete edges on branch delete ───────────────────────────────────

func TestEdgeLifecycle_022b_DeleteBranchCleansEdges(t *testing.T) {
	ctx := context.Background()
	m := newTestManager()
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "testrepo"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	taskBranch, err := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "task-del-001"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	blobID := writeTestFile(t, m, taskBranch.ID, "src/main.go", "package main")

	kw, err := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "service"})
	if err != nil {
		t.Fatalf("CreateKeyword: %v", err)
	}
	if err := m.CreateEdge(ctx, CreateEdgeRequest{
		BranchID: taskBranch.ID, FromEntityID: blobID, ToEntityID: kw.ID, RelationshipName: "tagged_with",
	}); err != nil {
		t.Fatalf("CreateEdge: %v", err)
	}

	if got := countEdgesByBranchID(t, m, blobID, "tagged_with", taskBranch.ID); got != 1 {
		t.Fatalf("before DeleteBranch: want 1 edge, got %d", got)
	}

	if err := m.DeleteBranch(ctx, taskBranch.ID); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}

	if got := countEdgesByBranchID(t, m, blobID, "tagged_with", taskBranch.ID); got != 0 {
		t.Fatalf("after DeleteBranch: want 0 edges, got %d", got)
	}
}

// TestEdgeLifecycle_022b_OnlyDeletesScopedEdges verifies that DeleteBranch
// only removes edges scoped to the deleted branch, leaving replicated edges
// on the default branch intact.
func TestEdgeLifecycle_022b_OnlyDeletesScopedEdges(t *testing.T) {
	ctx := context.Background()
	m := newTestManager()
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "testrepo"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, _ := m.ListBranches(ctx, repo.ID)
	defaultBranchID := branches[0].ID

	branchA, err := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "task-scope-a"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	blobID := writeTestFile(t, m, branchA.ID, "src/util.go", "package util")

	kw, err := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "util"})
	if err != nil {
		t.Fatalf("CreateKeyword: %v", err)
	}
	if err := m.CreateEdge(ctx, CreateEdgeRequest{
		BranchID: branchA.ID, FromEntityID: blobID, ToEntityID: kw.ID, RelationshipName: "tagged_with",
	}); err != nil {
		t.Fatalf("CreateEdge on branchA: %v", err)
	}

	if _, err := m.MergeBranch(ctx, branchA.ID); err != nil {
		t.Fatalf("MergeBranch branchA: %v", err)
	}
	if err := m.DeleteBranch(ctx, branchA.ID); err != nil {
		t.Fatalf("DeleteBranch branchA: %v", err)
	}

	if got := countEdgesByBranchID(t, m, blobID, "tagged_with", branchA.ID); got != 0 {
		t.Errorf("after delete branchA: want 0 edges on branchA, got %d", got)
	}
	if got := countEdgesByBranchID(t, m, blobID, "tagged_with", defaultBranchID); got != 1 {
		t.Errorf("after delete branchA: default-branch edge should survive, got %d", got)
	}
}

// ── GIT-022c: Remove edges on file delete ────────────────────────────────────

func TestEdgeLifecycle_022c_DeleteFileRemovesEdges(t *testing.T) {
	ctx := context.Background()
	m := newTestManager()
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "testrepo"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	taskBranch, err := m.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "task-delfile-001"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	blobID := writeTestFile(t, m, taskBranch.ID, "docs/guide.md", "# Guide")

	kw, err := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "guide"})
	if err != nil {
		t.Fatalf("CreateKeyword: %v", err)
	}
	if err := m.CreateEdge(ctx, CreateEdgeRequest{
		BranchID: taskBranch.ID, FromEntityID: blobID, ToEntityID: kw.ID, RelationshipName: "tagged_with",
	}); err != nil {
		t.Fatalf("CreateEdge: %v", err)
	}

	if got := countEdgesByBranchID(t, m, blobID, "tagged_with", taskBranch.ID); got != 1 {
		t.Fatalf("before DeleteFile: want 1 edge, got %d", got)
	}

	if _, err := m.DeleteFile(ctx, DeleteFileRequest{
		BranchID: taskBranch.ID, Path: "docs/guide.md", AuthorName: "test",
	}); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}

	if got := countEdgesByBranchID(t, m, blobID, "tagged_with", taskBranch.ID); got != 0 {
		t.Fatalf("after DeleteFile: want 0 edges, got %d", got)
	}
}
