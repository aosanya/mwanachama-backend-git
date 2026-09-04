package gormstore

import (
	"fmt"

	"gorm.io/gorm"
)

// RawEdge is one directed edge surfaced by [NeighborhoodEdges], mirroring
// the shape the root package's GraphEdge needs (Name/FromID/ToID) without
// this package depending on the root package's types.
type RawEdge struct {
	Name   string
	FromID string
	ToID   string
}

// CommitParentIDs returns commitID's parent commit IDs, in order (index 0 =
// first parent, 1+ = merge parents) — see [CommitParentRow].
func CommitParentIDs(db *gorm.DB, t TableNames, commitID string) ([]string, error) {
	var ids []string
	err := db.Table(t.CommitParents).Where("commit_id = ?", commitID).
		Order("parent_index").Pluck("parent_id", &ids).Error
	return ids, err
}

// TreeBlobIDs returns the direct Blob children of treeID.
func TreeBlobIDs(db *gorm.DB, t TableNames, treeID string) ([]string, error) {
	var ids []string
	err := db.Table(t.TreeBlobs).Where("tree_id = ?", treeID).Pluck("blob_id", &ids).Error
	return ids, err
}

// TreeSubtreeIDs returns the direct subtree children of treeID.
func TreeSubtreeIDs(db *gorm.DB, t TableNames, treeID string) ([]string, error) {
	var ids []string
	err := db.Table(t.TreeSubtrees).Where("tree_id = ?", treeID).Pluck("subtree_id", &ids).Error
	return ids, err
}

// KeywordChildIDs returns the direct children of parentID ("" = roots).
func KeywordChildIDs(db *gorm.DB, t TableNames, parentID string) ([]string, error) {
	q := db.Table(t.Keywords).Where("NOT deleted")
	if parentID == "" {
		q = q.Where("parent_id IS NULL")
	} else {
		q = q.Where("parent_id = ?", parentID)
	}
	var ids []string
	err := q.Order("id").Pluck("id", &ids).Error
	return ids, err
}

// KeywordDescendantIDs returns every descendant of keywordID (not including
// keywordID itself), following the ParentID self-reference. A recursive CTE
// (UNION, not UNION ALL) replaces the old collectDescendants Go-side
// recursive walk over has_child edges; UNION gives the same cycle guard the
// old visited-map had, and the depth<32 bound is a new backstop the
// recursive Go version lacked.
func KeywordDescendantIDs(db *gorm.DB, t TableNames, keywordID string) ([]string, error) {
	q := fmt.Sprintf(`
WITH RECURSIVE kw(id, depth) AS (
    SELECT ? AS id, 0 AS depth
  UNION
    SELECT k.id, kw.depth + 1
    FROM %s k
    JOIN kw ON k.parent_id = kw.id
    WHERE NOT k.deleted AND kw.depth < 32
)
SELECT id FROM kw WHERE depth > 0`, t.Keywords)
	var ids []string
	err := db.Raw(q, keywordID).Scan(&ids).Error
	return ids, err
}

// BlobsAtCommit returns every Blob reachable from commitID's tree: the root
// tree (commits.tree_id), then up to 3 further levels of subtree nesting
// (tree_subtrees), collecting blobs (tree_blobs) at each visited tree level.
// Replaces the old allBlobsAtCommit outbound BFS (has_tree/has_subtree/
// has_blob edges, capped at allBlobsAtCommitMaxDepth=5 total hops) with one
// recursive CTE; the depth<3 recursion guard below reproduces that same
// four-tree-level budget (root + 3 nested levels) — see this function's
// call sites for the hop-by-hop trace justifying the bound.
func BlobsAtCommit(db *gorm.DB, t TableNames, commitID string) ([]BlobRow, error) {
	q := fmt.Sprintf(`
WITH RECURSIVE tr(id, depth) AS (
    SELECT tree_id AS id, 0 AS depth FROM %[1]s WHERE id = ? AND tree_id IS NOT NULL
  UNION
    SELECT ts.subtree_id, tr.depth + 1
    FROM %[2]s ts
    JOIN tr ON ts.tree_id = tr.id
    WHERE tr.depth < 3
)
SELECT DISTINCT b.*
FROM %[3]s b
JOIN %[4]s tb ON tb.blob_id = b.id
JOIN tr ON tr.id = tb.tree_id
WHERE NOT b.deleted`, t.Commits, t.TreeSubtrees, t.Blobs, t.TreeBlobs)
	var rows []BlobRow
	err := db.Raw(q, commitID).Scan(&rows).Error
	return rows, err
}

// CommitChainIDs returns commitID and every ancestor reachable via
// CommitParentRow, ordered nearest-first (BFS depth order), replacing the
// old walkCommitChain queue-driven BFS with a recursive CTE. limit <= 0
// means no limit.
func CommitChainIDs(db *gorm.DB, t TableNames, startCommitID string, limit int) ([]string, error) {
	q := fmt.Sprintf(`
WITH RECURSIVE c(id, depth) AS (
    SELECT ? AS id, 0 AS depth
  UNION
    SELECT cp.parent_id, c.depth + 1
    FROM %s cp
    JOIN c ON cp.commit_id = c.id
)
SELECT id FROM c ORDER BY depth`, t.CommitParents)
	var ids []string
	if err := db.Raw(q, startCommitID).Scan(&ids).Error; err != nil {
		return nil, err
	}
	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	return ids, nil
}

// ResolveNodeType probes all nine node tables for id, in a fixed order, and
// returns the TypeID name of whichever table contains a non-deleted row
// with that id. Returns found=false if none do. Replaces the old
// shared-entities-table GetEntity lookup, which had one ID space across all
// types; here each type has its own table, so resolving "what type is this
// ID" costs up to nine indexed existence checks instead of one lookup — a
// GetNeighborhood/resolveEntityID-only cost, paid once per call, not once
// per BFS level.
func ResolveNodeType(db *gorm.DB, t TableNames, id string) (typeID string, found bool, err error) {
	checks := []struct{ typeID, table string }{
		{"Agency", t.Agencies},
		{"Repository", t.Repositories},
		{"Branch", t.Branches},
		{"MergeRequest", t.MergeRequests},
		{"Tag", t.Tags},
		{"Commit", t.Commits},
		{"Tree", t.Trees},
		{"Blob", t.Blobs},
		{"Keyword", t.Keywords},
	}
	for _, c := range checks {
		var count int64
		if cerr := db.Table(c.table).Where("id = ? AND NOT deleted", id).Count(&count).Error; cerr != nil {
			return "", false, fmt.Errorf("ResolveNodeType: %s: %w", c.table, cerr)
		}
		if count > 0 {
			return c.typeID, true, nil
		}
	}
	return "", false, nil
}

// edgeShape describes one of the sixteen fixed relationship shapes
// [NeighborhoodEdges] can surface — see this repo's CLAUDE.md for the full
// catalogue this flattens entitygraph's generic relationships table into.
type edgeShape struct {
	table          string
	fromCol, toCol string
	label          string
}

// NeighborhoodEdges returns every edge, across all sixteen known shapes,
// touching at least one ID in frontier — either as the FromID or the ToID.
// This is the flattened-schema replacement for entitygraph's generic
// "list relationships by FromID or ToID" query: since there is no longer one
// shared relationships table, this issues one bounded query per edge shape
// (small, fixed cost — not per-vertex) rather than a single generic lookup.
func NeighborhoodEdges(db *gorm.DB, t TableNames, frontier []string) ([]RawEdge, error) {
	shapes := []edgeShape{
		{t.Repositories, "agency_id", "id", "has_repository"},
		{t.Branches, "repository_id", "id", "has_branch"},
		{t.Tags, "repository_id", "id", "has_tag"},
		{t.Commits, "repository_id", "id", "has_commit"},
		{t.MergeRequests, "repository_id", "id", "has_merge_request"},
		{t.Branches, "id", "head_commit_id", "points_to"},
		{t.Tags, "id", "commit_id", "points_to"},
		{t.MergeRequests, "id", "source_branch_id", "has_source_branch"},
		{t.MergeRequests, "id", "target_branch_id", "has_target_branch"},
		{t.Commits, "id", "tree_id", "has_tree"},
		{t.CommitParents, "commit_id", "parent_id", "has_parent"},
		{t.TreeBlobs, "tree_id", "blob_id", "has_blob"},
		{t.TreeSubtrees, "tree_id", "subtree_id", "has_subtree"},
		{t.BlobKeywordTags, "blob_id", "keyword_id", "tagged_with"},
		{t.Keywords, "parent_id", "id", "has_child"},
	}

	var edges []RawEdge
	for _, s := range shapes {
		q := fmt.Sprintf("SELECT ? AS name, %s AS from_id, %s AS to_id FROM %s WHERE %s IN ? OR %s IN ?",
			s.fromCol, s.toCol, s.table, s.fromCol, s.toCol)
		var rows []RawEdge
		if err := db.Raw(q, s.label, frontier, frontier).Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("NeighborhoodEdges: %s: %w", s.label, err)
		}
		edges = append(edges, rows...)
	}

	// blob_references carries its own Name per row ("references" or
	// "referenced_by"), so it can't use the constant-label template above.
	refQ := fmt.Sprintf("SELECT name, from_blob_id AS from_id, to_blob_id AS to_id FROM %s WHERE from_blob_id IN ? OR to_blob_id IN ?", t.BlobReferences)
	var refRows []RawEdge
	if err := db.Raw(refQ, frontier, frontier).Scan(&refRows).Error; err != nil {
		return nil, fmt.Errorf("NeighborhoodEdges: blob_references: %w", err)
	}
	edges = append(edges, refRows...)

	return edges, nil
}
