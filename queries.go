package mwanachamagit

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-git/models"
)

type RawEdge struct {
	Name   string
	FromID string
	ToID   string
}

func CommitParentIDs(db *gorm.DB, t tableSet, commitID string) ([]string, error) {
	var ids []string
	err := db.Table(t.CommitParents).Where("commit_id = ?", commitID).
		Order("parent_index").Pluck("parent_id", &ids).Error
	return ids, err
}

func TreeBlobIDs(db *gorm.DB, t tableSet, treeID string) ([]string, error) {
	var ids []string
	err := db.Table(t.TreeBlobs).Where("tree_id = ?", treeID).Pluck("blob_id", &ids).Error
	return ids, err
}

func TreeSubtreeIDs(db *gorm.DB, t tableSet, treeID string) ([]string, error) {
	var ids []string
	err := db.Table(t.TreeSubtrees).Where("tree_id = ?", treeID).Pluck("subtree_id", &ids).Error
	return ids, err
}

func KeywordChildIDs(db *gorm.DB, t tableSet, parentID string) ([]string, error) {
	q := db.Table(t.Keywords).Where("NOT deleted")
	if parentID == "" {
		q = q.Where("parent_id = ?", "")
	} else {
		q = q.Where("parent_id = ?", parentID)
	}
	var ids []string
	err := q.Order("id").Pluck("id", &ids).Error
	return ids, err
}

func KeywordDescendantIDs(db *gorm.DB, t tableSet, keywordID string) ([]string, error) {
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

func BlobsAtCommit(db *gorm.DB, t tableSet, commitID string) ([]models.Blob, error) {
	q := fmt.Sprintf(`
WITH RECURSIVE tr(id, depth) AS (
    SELECT tree_id AS id, 0 AS depth FROM %[1]s WHERE id = ? AND tree_id <> ''
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
	var rows []models.Blob
	err := db.Raw(q, commitID).Scan(&rows).Error
	return rows, err
}

func CommitChainIDs(db *gorm.DB, t tableSet, startCommitID string, limit int) ([]string, error) {
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

func ResolveNodeType(db *gorm.DB, t tableSet, id string) (typeID string, found bool, err error) {
	checks := []struct{ typeID, table string }{
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

type edgeShape struct {
	table          string
	fromCol, toCol string
	label          string
}

func NeighborhoodEdges(db *gorm.DB, t tableSet, frontier []string) ([]RawEdge, error) {
	shapes := []edgeShape{
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

	refQ := fmt.Sprintf("SELECT name, from_blob_id AS from_id, to_blob_id AS to_id FROM %s WHERE from_blob_id IN ? OR to_blob_id IN ?", t.BlobReferences)
	var refRows []RawEdge
	if err := db.Raw(refQ, frontier, frontier).Scan(&refRows).Error; err != nil {
		return nil, fmt.Errorf("NeighborhoodEdges: blob_references: %w", err)
	}
	edges = append(edges, refRows...)

	return edges, nil
}
