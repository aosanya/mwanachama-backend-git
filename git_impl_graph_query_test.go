// git_impl_graph_query_test.go — G9 fidelity port of CodeValdGit's
// git_impl_graph_query_test.go: GitManager.QueryGraph (GIT-026) and
// GitManager.GetNeighborhood (GIT-020).
//
// GetNeighborhood's fixtures were originally built from arbitrary "Node"
// entities linked by "next" edges — a capability the flattened GORM schema
// deliberately no longer offers (see this repo's CLAUDE.md: no generic
// polymorphic edge table survives the entitygraph->GORM migration). The
// same BFS properties (both directions followed, depth clamp, node cap,
// edge-survives-cap) are now exercised via real Commit rows linked by
// git_commit_parents — a genuinely many-to-many, self-referencing relation
// that supports the same arbitrary fan-out/depth shapes the old fixtures did.
package mwanachamagit

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/gormstore"
	"github.com/aosanya/mwanachama-backend-git/models"
)

// seedTaggedWith creates a tagged_with row directly so the signal/note/
// branch_id columns are set exactly as a real sync would write them.
func seedTaggedWith(t *testing.T, m *gitManager, blobID, kwID, signal, branchID string) {
	t.Helper()
	row := gormstore.BlobKeywordTagRow{
		BranchID: branchID, BlobID: blobID, KeywordID: kwID, Signal: signal, CreatedAt: models.NowRFC3339(),
	}
	if err := m.db.WithContext(context.Background()).Table(m.tables.BlobKeywordTags).Create(&row).Error; err != nil {
		t.Fatalf("seedTaggedWith: %v", err)
	}
}

// listBlobIDs returns the IDs of all Blob rows.
func listBlobIDs(t *testing.T, m *gitManager) []string {
	t.Helper()
	var ids []string
	if err := m.db.WithContext(context.Background()).Table(m.tables.Blobs).Pluck("id", &ids).Error; err != nil {
		t.Fatalf("listBlobIDs: %v", err)
	}
	return ids
}

func TestQueryGraph_EmptyBody_ReturnsTaggedBlobs(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
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
	m := newTestManager(t)
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
	m := newTestManager(t)
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
	m := newTestManager(t)
	_, err := m.QueryGraph(context.Background(), QueryGraphRequest{BranchID: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent branch, got nil")
	}
}

func TestQueryGraph_LimitEnforced(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
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
	m := newTestManager(t)
	repo, _ := m.InitRepo(ctx, CreateRepoRequest{Name: "r"})
	branches, _ := m.ListBranches(ctx, repo.ID)
	branch := branches[0]
	writeTestFile(t, m, branch.ID, "high.go", "package p")
	writeTestFile(t, m, branch.ID, "low.go", "package p")

	kw, _ := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "kw4", Scope: "agency"})

	pathSignal := map[string]string{"high.go": "authority", "low.go": "surface"}
	blobsByID := map[string]string{}
	var allBlobs []gormstore.BlobRow
	m.db.WithContext(ctx).Table(m.tables.Blobs).Find(&allBlobs)
	for _, b := range allBlobs {
		sig := pathSignal[b.Path]
		if sig == "" {
			sig = "surface"
		}
		blobsByID[b.ID] = b.Path
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
	m := newTestManager(t)
	repo, _ := m.InitRepo(ctx, CreateRepoRequest{Name: "r"})
	branches, _ := m.ListBranches(ctx, repo.ID)
	branch := branches[0]
	writeTestFile(t, m, branch.ID, "a.go", "package a")
	writeTestFile(t, m, branch.ID, "b.go", "package b")

	kwA, _ := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "kwA", Scope: "agency"})
	kwB, _ := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "kwB", Scope: "agency"})

	var allBlobs []gormstore.BlobRow
	m.db.WithContext(ctx).Table(m.tables.Blobs).Find(&allBlobs)
	var aID, bID string
	for _, b := range allBlobs {
		switch b.Path {
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
	m := newTestManager(t)
	repo, _ := m.InitRepo(ctx, CreateRepoRequest{Name: "r"})
	branches, _ := m.ListBranches(ctx, repo.ID)
	branch := branches[0]
	writeTestFile(t, m, branch.ID, "x.go", "package x")
	writeTestFile(t, m, branch.ID, "y.go", "package y")

	kw, _ := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "kwE", Scope: "agency"})
	var allBlobs []gormstore.BlobRow
	m.db.WithContext(ctx).Table(m.tables.Blobs).Find(&allBlobs)
	var xID, yID string
	for _, b := range allBlobs {
		switch b.Path {
		case "x.go":
			xID = b.ID
		case "y.go":
			yID = b.ID
		}
		seedTaggedWith(t, m, b.ID, kw.ID, "surface", branch.ID)
	}
	if err := m.db.WithContext(ctx).Table(m.tables.BlobReferences).Create(&gormstore.BlobReferenceRow{
		BranchID: branch.ID, FromBlobID: xID, Name: "references", ToBlobID: yID,
		Descriptor: "depends_on", CreatedAt: models.NowRFC3339(),
	}).Error; err != nil {
		t.Fatalf("create BlobReferenceRow: %v", err)
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

// ── GetNeighborhood (GIT-020) ─────────────────────────────────────────────────
//
// These tests cover the behavior traversal must preserve: the starting
// entity always included, both edge directions followed, the depth clamp
// to [1, 3], and the 100-node hard cap — now exercised over real Commit rows
// linked by git_commit_parents (a genuinely many-to-many, self-referencing
// relation) instead of the entitygraph-era generic "Node"/"next" fixtures.

// neighborhoodTestNode creates a bare Commit row for use as a graph-traversal
// fixture — GetNeighborhood doesn't care what a node's own fields are, only
// how it's connected via git_commit_parents.
func neighborhoodTestNode(t *testing.T, m *gitManager, name string) gormstore.CommitRow {
	t.Helper()
	row := gormstore.CommitToRow(models.Commit{SHA: name, Message: name, CreatedAt: models.NowRFC3339()})
	if err := m.db.WithContext(context.Background()).Table(m.tables.Commits).Create(&row).Error; err != nil {
		t.Fatalf("create node %q: %v", name, err)
	}
	return row
}

// neighborhoodTestEdge links two commits with a has_parent row (fromID is
// the "commit", toID is its "parent" — direction is arbitrary for these
// tests, which only care that both directions get traversed).
func neighborhoodTestEdge(t *testing.T, m *gitManager, fromID, toID string) {
	t.Helper()
	if err := m.db.WithContext(context.Background()).Table(m.tables.CommitParents).
		Create(&gormstore.CommitParentRow{CommitID: fromID, ParentID: toID}).Error; err != nil {
		t.Fatalf("link %s -> %s: %v", fromID, toID, err)
	}
}

// neighborhoodTestBranch returns a branch ID satisfying GetNeighborhood's
// branch-existence check; the traversal itself is branch-agnostic.
func neighborhoodTestBranch(t *testing.T, m *gitManager) string {
	t.Helper()
	ctx := context.Background()
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "neighborhood-fixture"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, err := m.ListBranches(ctx, repo.ID)
	if err != nil || len(branches) == 0 {
		t.Fatalf("ListBranches: %+v (err=%v)", branches, err)
	}
	return branches[0].ID
}

func TestGetNeighborhood_IncludesStartAndBothDirections(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	branchID := neighborhoodTestBranch(t, m)

	a := neighborhoodTestNode(t, m, "a")
	b := neighborhoodTestNode(t, m, "b")
	c := neighborhoodTestNode(t, m, "c")
	neighborhoodTestEdge(t, m, a.ID, b.ID) // outbound from a
	neighborhoodTestEdge(t, m, c.ID, a.ID) // inbound to a

	result, err := m.GetNeighborhood(ctx, branchID, a.ID, 1)
	if err != nil {
		t.Fatalf("GetNeighborhood: %v", err)
	}

	gotIDs := make(map[string]bool, len(result.Nodes))
	for _, n := range result.Nodes {
		gotIDs[n.ID] = true
	}
	if !gotIDs[a.ID] {
		t.Error("expected starting entity a in result")
	}
	if !gotIDs[b.ID] {
		t.Error("expected outbound neighbor b in result")
	}
	if !gotIDs[c.ID] {
		t.Error("expected inbound neighbor c in result")
	}
	if len(result.Nodes) != 3 {
		t.Errorf("expected exactly 3 nodes (a, b, c), got %d: %+v", len(result.Nodes), result.Nodes)
	}
	if len(result.Edges) != 2 {
		t.Errorf("expected exactly 2 edges, got %d: %+v", len(result.Edges), result.Edges)
	}
}

func TestGetNeighborhood_DepthClamp(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	branchID := neighborhoodTestBranch(t, m)

	// Chain: n0 -> n1 -> n2 -> n3 -> n4 (5 nodes, 4 hops).
	nodes := make([]gormstore.CommitRow, 5)
	for i := range nodes {
		nodes[i] = neighborhoodTestNode(t, m, fmt.Sprintf("n%d", i))
	}
	for i := 0; i < len(nodes)-1; i++ {
		neighborhoodTestEdge(t, m, nodes[i].ID, nodes[i+1].ID)
	}

	cases := []struct {
		name      string
		depth     int
		wantNodes int // starting node counts as 1
	}{
		{"zero clamps to 1", 0, 2},      // n0, n1
		{"negative clamps to 1", -5, 2}, // n0, n1
		{"one stays one", 1, 2},         // n0, n1
		{"two stays two", 2, 3},         // n0, n1, n2
		{"three stays three", 3, 4},     // n0, n1, n2, n3
		{"four clamps to 3", 4, 4},      // n0, n1, n2, n3 (n4 excluded)
		{"large clamps to 3", 1000, 4},  // n0, n1, n2, n3 (n4 excluded)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := m.GetNeighborhood(ctx, branchID, nodes[0].ID, tc.depth)
			if err != nil {
				t.Fatalf("GetNeighborhood depth=%d: %v", tc.depth, err)
			}
			if len(result.Nodes) != tc.wantNodes {
				t.Errorf("depth=%d: got %d nodes, want %d: %+v", tc.depth, len(result.Nodes), tc.wantNodes, result.Nodes)
			}
		})
	}
}

func TestGetNeighborhood_NodeCapEnforced(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	branchID := neighborhoodTestBranch(t, m)

	center := neighborhoodTestNode(t, m, "center")
	// One more leaf than the cap so the cap is the binding constraint, not
	// the graph's actual size.
	const leafCount = neighborhoodMaxNodes + 50
	for i := 0; i < leafCount; i++ {
		leaf := neighborhoodTestNode(t, m, fmt.Sprintf("leaf%d", i))
		neighborhoodTestEdge(t, m, center.ID, leaf.ID)
	}

	result, err := m.GetNeighborhood(ctx, branchID, center.ID, 3)
	if err != nil {
		t.Fatalf("GetNeighborhood: %v", err)
	}
	if len(result.Nodes) != neighborhoodMaxNodes {
		t.Errorf("expected exactly the %d-node cap, got %d", neighborhoodMaxNodes, len(result.Nodes))
	}
	for _, e := range result.Edges {
		if e.FromID != center.ID {
			t.Errorf("unexpected edge not rooted at center: %+v", e)
		}
	}
}

// TestGetNeighborhood_EdgeBetweenIncludedNodesSurvivesCap guards against a
// regression where the node-cap short-circuit skipped querying a frontier
// node's own relationships once the cap filled up earlier in the same BFS
// round. That node (and its edges' other endpoint) still ended up in the
// returned vertex set — since it was already marked visited in a prior
// round — but the edge between them silently vanished from the result
// because neither endpoint's own edges were ever queried.
//
// Shape: center -> hub1, center -> hub2, center -> witness (round 1, so all
// three are admitted before any cap pressure exists). hub1 additionally fans
// out to far more leaves than the remaining node budget (100 - 4 already
// visited = 96), so processing hub1's own edges exhausts the cap by itself.
// hub2's only edge (hub2 -> witness) must still be discoverable since
// [gormstore.NeighborhoodEdges] queries the whole frontier in one shot per
// level, not per-node.
func TestGetNeighborhood_EdgeBetweenIncludedNodesSurvivesCap(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	branchID := neighborhoodTestBranch(t, m)

	center := neighborhoodTestNode(t, m, "center")
	hub1 := neighborhoodTestNode(t, m, "hub1")
	hub2 := neighborhoodTestNode(t, m, "hub2")
	witness := neighborhoodTestNode(t, m, "witness")
	neighborhoodTestEdge(t, m, center.ID, hub1.ID)
	neighborhoodTestEdge(t, m, center.ID, hub2.ID)
	neighborhoodTestEdge(t, m, center.ID, witness.ID)

	neighborhoodTestEdge(t, m, hub1.ID, witness.ID)
	const fanout = 150 // > 96 remaining budget, so hub1 alone fills the cap
	for i := 0; i < fanout; i++ {
		leaf := neighborhoodTestNode(t, m, fmt.Sprintf("hub1-leaf%d", i))
		neighborhoodTestEdge(t, m, hub1.ID, leaf.ID)
	}

	neighborhoodTestEdge(t, m, hub2.ID, witness.ID)

	result, err := m.GetNeighborhood(ctx, branchID, center.ID, 3)
	if err != nil {
		t.Fatalf("GetNeighborhood: %v", err)
	}
	if len(result.Nodes) != neighborhoodMaxNodes {
		t.Fatalf("expected the %d-node cap to bind, got %d", neighborhoodMaxNodes, len(result.Nodes))
	}

	gotIDs := make(map[string]bool, len(result.Nodes))
	for _, n := range result.Nodes {
		gotIDs[n.ID] = true
	}
	if !gotIDs[hub1.ID] || !gotIDs[hub2.ID] || !gotIDs[witness.ID] {
		t.Fatalf("expected hub1, hub2 and witness all included (admitted in round 1, before any cap pressure existed): %+v", result.Nodes)
	}

	hasEdge := func(fromID, toID string) bool {
		for _, e := range result.Edges {
			if e.FromID == fromID && e.ToID == toID {
				return true
			}
		}
		return false
	}
	if !hasEdge(hub1.ID, witness.ID) {
		t.Error("expected hub1 -> witness edge in result (both endpoints included; hub1 is never skipped)")
	}
	if !hasEdge(hub2.ID, witness.ID) {
		t.Error("expected hub2 -> witness edge in result (both endpoints included) — this is the edge the cap short-circuit used to drop")
	}
}

func TestGetNeighborhood_BranchNotFound(t *testing.T) {
	m := newTestManager(t)
	a := neighborhoodTestNode(t, m, "a")
	_, err := m.GetNeighborhood(context.Background(), "nonexistent-branch", a.ID, 1)
	if !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("expected ErrBranchNotFound, got %v", err)
	}
}

func TestGetNeighborhood_EntityNotFound(t *testing.T) {
	m := newTestManager(t)
	branchID := neighborhoodTestBranch(t, m)
	_, err := m.GetNeighborhood(context.Background(), branchID, "nonexistent-entity-and-not-a-path", 1)
	if !errors.Is(err, ErrEntityNotFound) {
		t.Fatalf("expected ErrEntityNotFound, got %v", err)
	}
}

func TestGetNeighborhood_NoOutboundOrInboundEdges(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	branchID := neighborhoodTestBranch(t, m)
	isolated := neighborhoodTestNode(t, m, "isolated")

	result, err := m.GetNeighborhood(ctx, branchID, isolated.ID, 3)
	if err != nil {
		t.Fatalf("GetNeighborhood: %v", err)
	}
	if len(result.Nodes) != 1 || result.Nodes[0].ID != isolated.ID {
		t.Fatalf("expected only the isolated starting node, got %+v", result.Nodes)
	}
	if len(result.Edges) != 0 {
		t.Errorf("expected no edges, got %+v", result.Edges)
	}
}
