// git_impl_graph.go implements the graph query methods on [gitManager]:
//
//   - [GitManager.GetNeighborhood] — bounded subgraph traversal (depth 1-3,
//     100-node hard cap) over the closed catalogue of sixteen relationship
//     shapes the flattened schema can express (see gormstore.NeighborhoodEdges).
//
//   - [GitManager.SearchByKeywords] — keyword-driven Blob discovery with
//     optional taxonomy cascade and AND/OR match modes.
//
//   - [GitManager.QueryGraph] — multi-filter, signal-sorted Blob graph query.
//
// These three methods are the ones most affected by the entitygraph->GORM
// migration: entitygraph's generic, any-type/any-direction relationships
// table is gone, replaced by an enumerated set of typed FK columns and join
// tables. See this repo's CLAUDE.md for what that costs (no generic "walk
// any edge from any entity" capability survives) and what stays possible
// (every edge shape declared in the old schema.go is still traversable).
package mwanachamagit

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-git/gormstore"
)

const queryGraphDefaultLimit = 50

// neighborhoodMaxNodes is the hard cap on vertices returned by GetNeighborhood.
const neighborhoodMaxNodes = 100

// ── GetNeighborhood ───────────────────────────────────────────────────────────

// GetNeighborhood returns the subgraph reachable from entityID within depth
// hops, capped at [neighborhoodMaxNodes] nodes. The starting entity is always
// included as the first node in the result.
//
// depth is clamped to [1, 3]. The branch must exist (verified before traversal).
func (m *gitManager) GetNeighborhood(ctx context.Context, branchID, entityID string, depth int) (GraphResult, error) {
	if _, err := m.GetBranch(ctx, branchID); err != nil {
		if errors.Is(err, ErrBranchNotFound) {
			return GraphResult{}, ErrBranchNotFound
		}
		return GraphResult{}, fmt.Errorf("GetNeighborhood: get branch %s: %w", branchID, err)
	}

	depth = clampDepth(depth)

	// Resolve entityID: callers may pass a file path (e.g. "README.md") instead
	// of the actual row ID. Try the raw ID first; on not-found fall back to a
	// Blob lookup by path.
	resolvedID, err := m.resolveEntityID(ctx, entityID)
	if err != nil {
		return GraphResult{}, fmt.Errorf("GetNeighborhood %s: resolve entity: %w", entityID, err)
	}

	order, edges, err := m.traverseNeighborhood(ctx, resolvedID, depth)
	if err != nil {
		return GraphResult{}, fmt.Errorf("GetNeighborhood %s: traverse: %w", entityID, err)
	}

	nodes, err := m.hydrateNodes(ctx, order)
	if err != nil {
		return GraphResult{}, fmt.Errorf("GetNeighborhood %s: hydrate: %w", entityID, err)
	}

	return buildGraphResult(nodes, edges, neighborhoodMaxNodes), nil
}

// traverseNeighborhood performs a bounded breadth-first walk from startID,
// following relationships in either direction, up to depth hops or
// [neighborhoodMaxNodes] visited vertices, whichever comes first. Returns
// vertex IDs in discovery order (startID first) and the deduplicated edge
// set touching them.
//
// One [gormstore.NeighborhoodEdges] query covers the ENTIRE frontier per
// level (not one query per frontier vertex), so every frontier member's own
// edges are always collected regardless of whether the node cap fills up
// mid-round — the cap only gates whether a newly-discovered neighbor is
// admitted into the visited set, never whether a query runs.
func (m *gitManager) traverseNeighborhood(ctx context.Context, startID string, depth int) ([]string, []GraphEdge, error) {
	visited := map[string]bool{startID: true}
	order := []string{startID}
	seenEdges := map[string]bool{}
	var edges []GraphEdge
	frontier := []string{startID}

	for level := 0; level < depth && len(frontier) > 0 && len(visited) < neighborhoodMaxNodes; level++ {
		rawEdges, err := gormstore.NeighborhoodEdges(m.db.WithContext(ctx), m.tables, frontier)
		if err != nil {
			return nil, nil, fmt.Errorf("traverseNeighborhood: level %d: %w", level, err)
		}
		var next []string
		for _, re := range rawEdges {
			key := re.Name + ":" + re.FromID + ":" + re.ToID
			if !seenEdges[key] {
				seenEdges[key] = true
				edges = append(edges, GraphEdge{ID: key, Name: re.Name, FromID: re.FromID, ToID: re.ToID})
			}
			for _, neighborID := range [2]string{re.FromID, re.ToID} {
				if neighborID == "" || visited[neighborID] || len(visited) >= neighborhoodMaxNodes {
					continue
				}
				visited[neighborID] = true
				order = append(order, neighborID)
				next = append(next, neighborID)
			}
		}
		frontier = next
	}

	return order, edges, nil
}

// resolveEntityID returns the canonical row ID. If entityID already names a
// row in one of the nine node tables it is returned as-is. Otherwise, the
// method attempts to find a Blob row whose Path matches entityID.
// Returns [ErrEntityNotFound] if neither resolves.
func (m *gitManager) resolveEntityID(ctx context.Context, entityID string) (string, error) {
	if _, found, err := gormstore.ResolveNodeType(m.db.WithContext(ctx), m.tables, entityID); err != nil {
		return "", err
	} else if found {
		return entityID, nil
	}

	var row gormstore.BlobRow
	err := m.db.WithContext(ctx).Table(m.tables.Blobs).
		Where("path = ? AND NOT deleted", entityID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrEntityNotFound
		}
		return "", fmt.Errorf("resolveEntityID: list blobs by path %q: %w", entityID, err)
	}
	return row.ID, nil
}

// clampDepth enforces the range [1, 3] for traversal depth.
func clampDepth(d int) int {
	if d < 1 {
		return 1
	}
	if d > 3 {
		return 3
	}
	return d
}

// buildGraphResult applies the node cap and drops edges whose endpoints fall
// outside it.
func buildGraphResult(nodes []GraphNode, edges []GraphEdge, cap int) GraphResult {
	if len(nodes) > cap {
		nodes = nodes[:cap]
	}
	included := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		included[n.ID] = true
	}
	filtered := make([]GraphEdge, 0, len(edges))
	for _, e := range edges {
		if included[e.FromID] && included[e.ToID] {
			filtered = append(filtered, e)
		}
	}
	return GraphResult{Nodes: nodes, Edges: filtered}
}

// hydrateNodes fetches full row data for ids across all nine node tables and
// returns them as [GraphNode]s in the same order as ids — a row missing by
// the time hydration runs (e.g. deleted between the edge scan and here) is
// silently dropped, matching the entitygraph-era race behavior.
func (m *gitManager) hydrateNodes(ctx context.Context, ids []string) ([]GraphNode, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	byID := make(map[string]GraphNode, len(ids))
	db := m.db.WithContext(ctx)

	var agencies []gormstore.AgencyRow
	if err := db.Table(m.tables.Agencies).Where("id IN ? AND NOT deleted", ids).Find(&agencies).Error; err != nil {
		return nil, err
	}
	for _, r := range agencies {
		byID[r.ID] = GraphNode{ID: r.ID, TypeID: "Agency", Properties: map[string]any{
			"name": r.Name, "description": r.Description, "created_at": r.CreatedAt, "updated_at": r.UpdatedAt,
		}}
	}

	var repos []gormstore.RepositoryRow
	if err := db.Table(m.tables.Repositories).Where("id IN ? AND NOT deleted", ids).Find(&repos).Error; err != nil {
		return nil, err
	}
	for _, r := range repos {
		byID[r.ID] = GraphNode{ID: r.ID, TypeID: "Repository", Properties: map[string]any{
			"name": r.Name, "description": r.Description, "default_branch": r.DefaultBranch,
			"source_url": r.SourceURL, "created_at": r.CreatedAt, "updated_at": r.UpdatedAt,
		}}
	}

	var branches []gormstore.BranchRow
	if err := db.Table(m.tables.Branches).Where("id IN ? AND NOT deleted", ids).Find(&branches).Error; err != nil {
		return nil, err
	}
	for _, r := range branches {
		byID[r.ID] = GraphNode{ID: r.ID, TypeID: "Branch", Properties: map[string]any{
			"name": r.Name, "is_default": r.IsDefault, "sha": r.SHA, "status": r.Status,
			"workflow_run_id": r.WorkflowRunID, "created_at": r.CreatedAt, "updated_at": r.UpdatedAt,
		}}
	}

	var mrs []gormstore.MergeRequestRow
	if err := db.Table(m.tables.MergeRequests).Where("id IN ? AND NOT deleted", ids).Find(&mrs).Error; err != nil {
		return nil, err
	}
	for _, r := range mrs {
		byID[r.ID] = GraphNode{ID: r.ID, TypeID: "MergeRequest", Properties: map[string]any{
			"title": r.Title, "status": r.Status, "merged_commit_sha": r.MergedCommitSHA,
			"created_at": r.CreatedAt, "updated_at": r.UpdatedAt,
		}}
	}

	var tags []gormstore.TagRow
	if err := db.Table(m.tables.Tags).Where("id IN ? AND NOT deleted", ids).Find(&tags).Error; err != nil {
		return nil, err
	}
	for _, r := range tags {
		byID[r.ID] = GraphNode{ID: r.ID, TypeID: "Tag", Properties: map[string]any{
			"name": r.Name, "sha": r.SHA, "message": r.Message, "created_at": r.CreatedAt,
		}}
	}

	var commits []gormstore.CommitRow
	if err := db.Table(m.tables.Commits).Where("id IN ? AND NOT deleted", ids).Find(&commits).Error; err != nil {
		return nil, err
	}
	for _, r := range commits {
		byID[r.ID] = GraphNode{ID: r.ID, TypeID: "Commit", Properties: map[string]any{
			"sha": r.SHA, "message": r.Message, "author_name": r.AuthorName,
			"committed_at": r.CommittedAt, "created_at": r.CreatedAt,
		}}
	}

	var trees []gormstore.TreeRow
	if err := db.Table(m.tables.Trees).Where("id IN ? AND NOT deleted", ids).Find(&trees).Error; err != nil {
		return nil, err
	}
	for _, r := range trees {
		byID[r.ID] = GraphNode{ID: r.ID, TypeID: "Tree", Properties: map[string]any{
			"sha": r.SHA, "path": r.Path, "created_at": r.CreatedAt,
		}}
	}

	var blobs []gormstore.BlobRow
	if err := db.Table(m.tables.Blobs).Where("id IN ? AND NOT deleted", ids).Find(&blobs).Error; err != nil {
		return nil, err
	}
	for _, r := range blobs {
		byID[r.ID] = GraphNode{ID: r.ID, TypeID: "Blob", Properties: blobProps(r)}
	}

	var keywords []gormstore.KeywordRow
	if err := db.Table(m.tables.Keywords).Where("id IN ? AND NOT deleted", ids).Find(&keywords).Error; err != nil {
		return nil, err
	}
	for _, r := range keywords {
		byID[r.ID] = GraphNode{ID: r.ID, TypeID: "Keyword", Properties: map[string]any{
			"name": r.Name, "description": r.Description, "scope": r.Scope,
			"created_at": r.CreatedAt, "updated_at": r.UpdatedAt,
		}}
	}

	out := make([]GraphNode, 0, len(ids))
	for _, id := range ids {
		if n, ok := byID[id]; ok {
			out = append(out, n)
		}
	}
	return out, nil
}

// blobProps builds the property map for a Blob row, matching the
// entitygraph-era JSONB key set.
func blobProps(r gormstore.BlobRow) map[string]any {
	return map[string]any{
		"sha": r.SHA, "path": r.Path, "name": r.Name, "extension": r.Extension,
		"size": r.Size, "encoding": r.Encoding, "content": r.Content, "created_at": r.CreatedAt,
	}
}

// ── SearchByKeywords ──────────────────────────────────────────────────────────

// SearchByKeywords returns all Blob rows tagged (via tagged_with rows) with
// the specified keywords. When Cascade is true each keyword is expanded to its
// full descendant set before matching. MatchMode controls AND/OR semantics.
//
// Result nodes are formally all-Blob — matching what "tagged_with" edges
// have always pointed at in practice, now made explicit by the flattened
// schema (see this repo's CLAUDE.md).
func (m *gitManager) SearchByKeywords(ctx context.Context, req SearchByKeywordsRequest) (GraphResult, error) {
	if _, err := m.GetBranch(ctx, req.BranchID); err != nil {
		if errors.Is(err, ErrBranchNotFound) {
			return GraphResult{}, ErrBranchNotFound
		}
		return GraphResult{}, fmt.Errorf("SearchByKeywords: get branch %s: %w", req.BranchID, err)
	}

	if len(req.Keywords) == 0 {
		return GraphResult{Nodes: []GraphNode{}, Edges: []GraphEdge{}}, nil
	}

	mode := req.MatchMode
	if mode == "" {
		mode = KeywordMatchModeOR
	}

	taggedPerSet := make([]map[string]bool, len(req.Keywords))
	for i, kwID := range req.Keywords {
		set, err := m.taggedBlobsForKeyword(ctx, kwID, req.Cascade)
		if err != nil {
			return GraphResult{}, fmt.Errorf("SearchByKeywords: expand keyword %s: %w", kwID, err)
		}
		taggedPerSet[i] = set
	}

	var matchedIDs map[string]bool
	switch mode {
	case KeywordMatchModeAND:
		matchedIDs = intersectSets(taggedPerSet)
	default:
		matchedIDs = unionSets(taggedPerSet)
	}

	if len(matchedIDs) == 0 {
		return GraphResult{Nodes: []GraphNode{}, Edges: []GraphEdge{}}, nil
	}

	ids := make([]string, 0, len(matchedIDs))
	for id := range matchedIDs {
		ids = append(ids, id)
	}

	var rows []gormstore.BlobRow
	if err := m.db.WithContext(ctx).Table(m.tables.Blobs).
		Where("id IN ? AND NOT deleted", ids).Find(&rows).Error; err != nil {
		return GraphResult{}, fmt.Errorf("SearchByKeywords: fetch blobs: %w", err)
	}
	nodes := make([]GraphNode, len(rows))
	for i, r := range rows {
		nodes[i] = GraphNode{ID: r.ID, TypeID: "Blob", Properties: blobProps(r)}
	}

	edges, err := m.edgesBetweenBlobs(ctx, matchedIDs)
	if err != nil {
		return GraphResult{}, fmt.Errorf("SearchByKeywords: edges between results: %w", err)
	}

	return GraphResult{Nodes: nodes, Edges: edges}, nil
}

// taggedBlobsForKeyword returns the set of blob IDs tagged with kwID (and, if
// cascade, any of its descendants). The cascade expansion is a recursive CTE
// ([gormstore.KeywordDescendantIDs]) replacing the old collectDescendants
// Go-side recursive walk.
func (m *gitManager) taggedBlobsForKeyword(ctx context.Context, kwID string, cascade bool) (map[string]bool, error) {
	kwIDs := []string{kwID}
	if cascade {
		descendants, err := gormstore.KeywordDescendantIDs(m.db.WithContext(ctx), m.tables, kwID)
		if err != nil {
			return nil, err
		}
		kwIDs = append(kwIDs, descendants...)
	}
	var blobIDs []string
	if err := m.db.WithContext(ctx).Table(m.tables.BlobKeywordTags).
		Where("keyword_id IN ?", kwIDs).Distinct().Pluck("blob_id", &blobIDs).Error; err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(blobIDs))
	for _, id := range blobIDs {
		set[id] = true
	}
	return set, nil
}

// edgesBetweenBlobs returns every blob_references row whose endpoints are
// both in ids.
func (m *gitManager) edgesBetweenBlobs(ctx context.Context, ids map[string]bool) ([]GraphEdge, error) {
	idList := make([]string, 0, len(ids))
	for id := range ids {
		idList = append(idList, id)
	}
	var rows []gormstore.BlobReferenceRow
	if err := m.db.WithContext(ctx).Table(m.tables.BlobReferences).
		Where("from_blob_id IN ? AND to_blob_id IN ?", idList, idList).Find(&rows).Error; err != nil {
		return nil, err
	}
	edges := make([]GraphEdge, len(rows))
	for i, r := range rows {
		edges[i] = GraphEdge{ID: r.Name + ":" + r.FromBlobID + ":" + r.ToBlobID, Name: r.Name, FromID: r.FromBlobID, ToID: r.ToBlobID}
	}
	return edges, nil
}

// ── Set helpers ───────────────────────────────────────────────────────────────

// unionSets returns the union of all sets in the slice.
func unionSets(sets []map[string]bool) map[string]bool {
	result := make(map[string]bool)
	for _, s := range sets {
		for k := range s {
			result[k] = true
		}
	}
	return result
}

// intersectSets returns the intersection of all sets in the slice.
// An empty slice returns an empty map.
func intersectSets(sets []map[string]bool) map[string]bool {
	if len(sets) == 0 {
		return map[string]bool{}
	}
	smallest := sets[0]
	for _, s := range sets[1:] {
		if len(s) < len(smallest) {
			smallest = s
		}
	}
	result := make(map[string]bool, len(smallest))
	for k := range smallest {
		inAll := true
		for _, s := range sets {
			if !s[k] {
				inAll = false
				break
			}
		}
		if inAll {
			result[k] = true
		}
	}
	return result
}

// ── QueryGraph ────────────────────────────────────────────────────────────────

// blobWithLayer is the destination shape for QueryGraph's aggregate query —
// a Blob row plus its computed max signal layer.
type blobWithLayer struct {
	gormstore.BlobRow
	MaxLayer int
}

// QueryGraph returns up to req.Limit Blob nodes filtered across five dimensions
// and sorted by descending signal layer. An empty request returns the top 50
// highest-signal Blob nodes with all their inter-node edges.
//
// Replaces the entitygraph-era "load every Blob + every tagged_with edge into
// memory, then filter/sort in Go" body with one aggregate SQL query.
func (m *gitManager) QueryGraph(ctx context.Context, req QueryGraphRequest) (GraphResult, error) {
	if _, err := m.GetBranch(ctx, req.BranchID); err != nil {
		if errors.Is(err, ErrBranchNotFound) {
			return GraphResult{}, ErrBranchNotFound
		}
		return GraphResult{}, fmt.Errorf("QueryGraph: get branch: %w", err)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = queryGraphDefaultLimit
	}

	q := m.db.WithContext(ctx).Table(m.tables.Blobs+" AS b").
		Select(`b.*, COALESCE(MAX(CASE t.signal ` +
			`WHEN 'surface' THEN 1 WHEN 'index' THEN 2 WHEN 'structural' THEN 3 ` +
			`WHEN 'contributor' THEN 4 WHEN 'authority' THEN 5 ELSE 0 END), 0) AS max_layer`).
		Joins("LEFT JOIN " + m.tables.BlobKeywordTags + " AS t ON t.blob_id = b.id").
		Where("NOT b.deleted")

	// file_types — suffix match on path, escaped so a caller-supplied value
	// containing % or _ is matched literally rather than as a wildcard.
	if len(req.FileTypes) > 0 {
		or := m.db.Session(&gorm.Session{NewDB: true})
		for _, ft := range req.FileTypes {
			or = or.Or("b.path LIKE ? ESCAPE '\\'", "%"+escapeLike(ft))
		}
		q = q.Where(or)
	}
	// folders — prefix match on path.
	if len(req.Folders) > 0 {
		or := m.db.Session(&gorm.Session{NewDB: true})
		for _, f := range req.Folders {
			or = or.Or("b.path LIKE ? ESCAPE '\\'", escapeLike(f)+"%")
		}
		q = q.Where(or)
	}
	if len(req.KeywordIDs) > 0 {
		q = q.Where("EXISTS (SELECT 1 FROM "+m.tables.BlobKeywordTags+" k WHERE k.blob_id = b.id AND k.keyword_id IN ?)", req.KeywordIDs)
	}
	if len(req.Signals) > 0 {
		q = q.Where("EXISTS (SELECT 1 FROM "+m.tables.BlobKeywordTags+" s WHERE s.blob_id = b.id AND s.signal IN ?)", req.Signals)
	}

	var results []blobWithLayer
	// Tie-break on b.id makes ordering deterministic — the old in-Go
	// sort.Slice by max layer alone was not stable.
	if err := q.Group("b.id").Order("max_layer DESC, b.id").Limit(limit).Find(&results).Error; err != nil {
		return GraphResult{}, fmt.Errorf("QueryGraph: %w", err)
	}

	nodeIDs := make(map[string]bool, len(results))
	nodes := make([]GraphNode, len(results))
	for i, r := range results {
		nodeIDs[r.ID] = true
		nodes[i] = GraphNode{ID: r.ID, TypeID: "Blob", Properties: blobProps(r.BlobRow)}
	}

	edges, err := m.queryGraphEdges(ctx, nodeIDs, req.Relationships)
	if err != nil {
		return GraphResult{}, fmt.Errorf("QueryGraph: collect edges: %w", err)
	}

	return GraphResult{Nodes: nodes, Edges: edges}, nil
}

// queryGraphEdges collects all blob_references rows between nodes in the
// given ID set, filtered by rel (edge name or descriptor). An empty rel
// passes all edges through.
func (m *gitManager) queryGraphEdges(ctx context.Context, nodeIDs map[string]bool, rel []string) ([]GraphEdge, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(nodeIDs))
	for id := range nodeIDs {
		ids = append(ids, id)
	}
	q := m.db.WithContext(ctx).Table(m.tables.BlobReferences).
		Where("from_blob_id IN ? AND to_blob_id IN ?", ids, ids)
	if len(rel) > 0 {
		q = q.Where("(name IN ? OR descriptor IN ?)", rel, rel)
	}
	var rows []gormstore.BlobReferenceRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	edges := make([]GraphEdge, len(rows))
	for i, r := range rows {
		edges[i] = GraphEdge{ID: r.Name + ":" + r.FromBlobID + ":" + r.ToBlobID, Name: r.Name, FromID: r.FromBlobID, ToID: r.ToBlobID}
	}
	return edges, nil
}

// escapeLike escapes SQL LIKE metacharacters (% and _) plus the escape
// character itself, so a caller-supplied FileTypes/Folders value is matched
// literally — pair with "ESCAPE '\\'" in the LIKE clause.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
