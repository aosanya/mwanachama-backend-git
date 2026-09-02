// git_impl_graph_query_test.go — G9 fidelity port of CodeValdGit's
// git_impl_graph_query_test.go: GitManager.QueryGraph (GIT-026).
package mwanachamagit

import (
	"context"
	"testing"

	"github.com/aosanya/mwanachama-go-shared/entitygraph"
)

// seedTaggedWith creates a tagged_with relationship directly on m.dm so the
// signal/note/branch_id properties are preserved exactly as a real sync
// would write them.
func seedTaggedWith(t *testing.T, m *gitManager, blobID, kwID, signal, branchID string) {
	t.Helper()
	_, err := m.dm.CreateRelationship(context.Background(), entitygraph.CreateRelationshipRequest{
		AgencyID: m.agencyID,
		Name:     "tagged_with",
		FromID:   blobID,
		ToID:     kwID,
		Properties: map[string]any{
			"signal":    signal,
			"note":      "",
			"branch_id": branchID,
		},
	})
	if err != nil {
		t.Fatalf("seedTaggedWith: %v", err)
	}
}

// listBlobIDs returns the entity IDs of all Blob entities in m.dm.
func listBlobIDs(t *testing.T, m *gitManager) []string {
	t.Helper()
	blobs, err := m.dm.ListEntities(context.Background(), entitygraph.EntityFilter{
		AgencyID: m.agencyID, TypeID: "Blob",
	})
	if err != nil {
		t.Fatalf("listBlobIDs: %v", err)
	}
	ids := make([]string, len(blobs))
	for i, b := range blobs {
		ids[i] = b.ID
	}
	return ids
}

func TestQueryGraph_EmptyBody_ReturnsTaggedBlobs(t *testing.T) {
	ctx := context.Background()
	m := newTestManager()
	repo, _ := m.InitRepo(ctx, CreateRepoRequest{Name: "r"})
	branches, _ := m.ListBranches(ctx, repo.ID)
	branch := branches[0]
	writeTestFile(t, m, branch.ID, "src/auth.go", "package auth")
	writeTestFile(t, m, branch.ID, "src/user.go", "package user")

	kw, err := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "auth", Scope: "agency"})
	if err != nil {
		t.Fatalf("CreateKeyword: %v", err)
	}
	for _, id := range listBlobIDs(t, m) {
		seedTaggedWith(t, m, id, kw.ID, "authority", branch.ID)
	}

	result, err := m.QueryGraph(ctx, QueryGraphRequest{BranchID: branch.ID})
	if err != nil {
		t.Fatalf("QueryGraph: %v", err)
	}
	if len(result.Nodes) == 0 {
		t.Error("expected nodes, got empty result")
	}
}

func TestQueryGraph_FileTypeFilter(t *testing.T) {
	ctx := context.Background()
	m := newTestManager()
	repo, _ := m.InitRepo(ctx, CreateRepoRequest{Name: "r"})
	branches, _ := m.ListBranches(ctx, repo.ID)
	branch := branches[0]
	writeTestFile(t, m, branch.ID, "src/main.go", "package main")
	writeTestFile(t, m, branch.ID, "docs/guide.md", "# guide")

	kw, _ := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "kw1", Scope: "agency"})
	for _, id := range listBlobIDs(t, m) {
		seedTaggedWith(t, m, id, kw.ID, "surface", branch.ID)
	}

	result, err := m.QueryGraph(ctx, QueryGraphRequest{BranchID: branch.ID, FileTypes: []string{".go"}})
	if err != nil {
		t.Fatalf("QueryGraph file-type filter: %v", err)
	}
	for _, n := range result.Nodes {
		path, _ := n.Properties["path"].(string)
		if len(path) < 3 || path[len(path)-3:] != ".go" {
			t.Errorf("unexpected non-.go node path: %q", path)
		}
	}
}

func TestQueryGraph_FolderFilter(t *testing.T) {
	ctx := context.Background()
	m := newTestManager()
	repo, _ := m.InitRepo(ctx, CreateRepoRequest{Name: "r"})
	branches, _ := m.ListBranches(ctx, repo.ID)
	branch := branches[0]
	writeTestFile(t, m, branch.ID, "internal/server.go", "package server")
	writeTestFile(t, m, branch.ID, "cmd/main.go", "package main")

	kw, _ := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "kw2", Scope: "agency"})
	for _, id := range listBlobIDs(t, m) {
		seedTaggedWith(t, m, id, kw.ID, "index", branch.ID)
	}

	result, err := m.QueryGraph(ctx, QueryGraphRequest{BranchID: branch.ID, Folders: []string{"internal/"}})
	if err != nil {
		t.Fatalf("QueryGraph folder filter: %v", err)
	}
	if len(result.Nodes) == 0 {
		t.Fatal("expected at least one node in internal/")
	}
	for _, n := range result.Nodes {
		path, _ := n.Properties["path"].(string)
		if len(path) < 9 || path[:9] != "internal/" {
			t.Errorf("unexpected node outside folder: %q", path)
		}
	}
}

func TestQueryGraph_BranchNotFound(t *testing.T) {
	m := newTestManager()
	_, err := m.QueryGraph(context.Background(), QueryGraphRequest{BranchID: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent branch, got nil")
	}
}

func TestQueryGraph_LimitEnforced(t *testing.T) {
	ctx := context.Background()
	m := newTestManager()
	repo, _ := m.InitRepo(ctx, CreateRepoRequest{Name: "r"})
	branches, _ := m.ListBranches(ctx, repo.ID)
	branch := branches[0]
	for _, name := range []string{"a.go", "b.go", "c.go", "d.go", "e.go"} {
		writeTestFile(t, m, branch.ID, "src/"+name, "package p")
	}

	kw, _ := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "kw3", Scope: "agency"})
	for _, id := range listBlobIDs(t, m) {
		seedTaggedWith(t, m, id, kw.ID, "surface", branch.ID)
	}

	result, err := m.QueryGraph(ctx, QueryGraphRequest{BranchID: branch.ID, Limit: 2})
	if err != nil {
		t.Fatalf("QueryGraph limit: %v", err)
	}
	if len(result.Nodes) > 2 {
		t.Errorf("limit=2: got %d nodes, want <=2", len(result.Nodes))
	}
}

func TestQueryGraph_SignalFilter(t *testing.T) {
	ctx := context.Background()
	m := newTestManager()
	repo, _ := m.InitRepo(ctx, CreateRepoRequest{Name: "r"})
	branches, _ := m.ListBranches(ctx, repo.ID)
	branch := branches[0]
	writeTestFile(t, m, branch.ID, "high.go", "package p")
	writeTestFile(t, m, branch.ID, "low.go", "package p")

	kw, _ := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "kw4", Scope: "agency"})

	pathSignal := map[string]string{"high.go": "authority", "low.go": "surface"}
	blobsByID := map[string]string{}
	allBlobs, _ := m.dm.ListEntities(ctx, entitygraph.EntityFilter{AgencyID: m.agencyID, TypeID: "Blob"})
	for _, b := range allBlobs {
		path, _ := b.Properties["path"].(string)
		sig := pathSignal[path]
		if sig == "" {
			sig = "surface"
		}
		blobsByID[b.ID] = path
		seedTaggedWith(t, m, b.ID, kw.ID, sig, branch.ID)
	}

	result, err := m.QueryGraph(ctx, QueryGraphRequest{BranchID: branch.ID, Signals: []string{"authority"}})
	if err != nil {
		t.Fatalf("QueryGraph signal filter: %v", err)
	}
	if len(result.Nodes) == 0 {
		t.Fatal("expected at least one authority node")
	}
	for _, n := range result.Nodes {
		path := blobsByID[n.ID]
		if path != "high.go" {
			t.Errorf("signal filter: unexpected node path %q (want high.go only)", path)
		}
	}
}

func TestQueryGraph_KeywordIDFilter(t *testing.T) {
	ctx := context.Background()
	m := newTestManager()
	repo, _ := m.InitRepo(ctx, CreateRepoRequest{Name: "r"})
	branches, _ := m.ListBranches(ctx, repo.ID)
	branch := branches[0]
	writeTestFile(t, m, branch.ID, "a.go", "package a")
	writeTestFile(t, m, branch.ID, "b.go", "package b")

	kwA, _ := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "kwA", Scope: "agency"})
	kwB, _ := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "kwB", Scope: "agency"})

	allBlobs, _ := m.dm.ListEntities(ctx, entitygraph.EntityFilter{AgencyID: m.agencyID, TypeID: "Blob"})
	var aID, bID string
	for _, b := range allBlobs {
		path, _ := b.Properties["path"].(string)
		switch path {
		case "a.go":
			aID = b.ID
		case "b.go":
			bID = b.ID
		}
	}
	seedTaggedWith(t, m, aID, kwA.ID, "authority", branch.ID)
	seedTaggedWith(t, m, bID, kwB.ID, "authority", branch.ID)

	result, err := m.QueryGraph(ctx, QueryGraphRequest{BranchID: branch.ID, KeywordIDs: []string{kwA.ID}})
	if err != nil {
		t.Fatalf("QueryGraph keyword filter: %v", err)
	}
	if len(result.Nodes) != 1 || result.Nodes[0].ID != aID {
		t.Errorf("keyword filter: want node %s only, got %v", aID, result.Nodes)
	}
}

func TestQueryGraph_EdgesOnlyBetweenReturnedNodes(t *testing.T) {
	ctx := context.Background()
	m := newTestManager()
	repo, _ := m.InitRepo(ctx, CreateRepoRequest{Name: "r"})
	branches, _ := m.ListBranches(ctx, repo.ID)
	branch := branches[0]
	writeTestFile(t, m, branch.ID, "x.go", "package x")
	writeTestFile(t, m, branch.ID, "y.go", "package y")

	kw, _ := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "kwE", Scope: "agency"})
	allBlobs, _ := m.dm.ListEntities(ctx, entitygraph.EntityFilter{AgencyID: m.agencyID, TypeID: "Blob"})
	var xID, yID string
	for _, b := range allBlobs {
		path, _ := b.Properties["path"].(string)
		switch path {
		case "x.go":
			xID = b.ID
		case "y.go":
			yID = b.ID
		}
		seedTaggedWith(t, m, b.ID, kw.ID, "surface", branch.ID)
	}
	if _, err := m.dm.CreateRelationship(ctx, entitygraph.CreateRelationshipRequest{
		AgencyID: m.agencyID, Name: "references", FromID: xID, ToID: yID,
		Properties: map[string]any{"descriptor": "depends_on", "branch_id": branch.ID},
	}); err != nil {
		t.Fatalf("CreateRelationship references: %v", err)
	}

	result, err := m.QueryGraph(ctx, QueryGraphRequest{BranchID: branch.ID})
	if err != nil {
		t.Fatalf("QueryGraph edges test: %v", err)
	}
	nodeSet := make(map[string]bool)
	for _, n := range result.Nodes {
		nodeSet[n.ID] = true
	}
	for _, e := range result.Edges {
		if !nodeSet[e.FromID] || !nodeSet[e.ToID] {
			t.Errorf("edge %s references nodes outside result set", e.ID)
		}
	}
}
