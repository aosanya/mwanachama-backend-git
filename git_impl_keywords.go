// git_impl_keywords.go implements the documentation layer methods on [gitManager]:
//
//   - Keyword CRUD ([GitManager.CreateKeyword], [GitManager.GetKeyword],
//     [GitManager.ListKeywords], [GitManager.GetKeywordTree],
//     [GitManager.UpdateKeyword], [GitManager.DeleteKeyword])
//
//   - Branch-scoped edge CRUD ([GitManager.CreateEdge], [GitManager.DeleteEdge])
//
// Edges are branch-scoped and follow the DR-010 lifecycle rules: replicated to
// main on merge, deleted on branch delete, migrated on file rename.
package mwanachamagit

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/aosanya/mwanachama-backend-git/gormstore"
	"github.com/aosanya/mwanachama-backend-git/models"
)

// validDocEdges is the set of allowed documentation relationship names.
// "tagged_with" and "references" are the two branch-scoped types that follow
// the DR-010 lifecycle (replicated on merge, deleted on branch delete).
var validDocEdges = map[string]bool{
	"tagged_with":   true,
	"references":    true,
	"referenced_by": true,
	"documents":     true,
	"documented_by": true,
	"depends_on":    true,
	"imported_by":   true,
}

// ── Keyword CRUD ──────────────────────────────────────────────────────────────

// CreateKeyword creates a new Keyword row in the taxonomy.
// If req.ParentID is set the keyword is added as a child of that parent.
// Returns [ErrKeywordAlreadyExists] if a keyword with the same name exists
// under the same parent. Returns [ErrKeywordNotFound] if req.ParentID does not
// resolve to a keyword.
func (m *gitManager) CreateKeyword(ctx context.Context, req CreateKeywordRequest) (models.Keyword, error) {
	if req.ParentID != "" {
		var count int64
		if err := m.db.WithContext(ctx).Table(m.tables.Keywords).
			Where("id = ? AND NOT deleted", req.ParentID).Count(&count).Error; err != nil {
			return models.Keyword{}, fmt.Errorf("CreateKeyword: get parent: %w", err)
		}
		if count == 0 {
			return models.Keyword{}, ErrKeywordNotFound
		}
	}

	siblingsQ := m.db.WithContext(ctx).Table(m.tables.Keywords).Where("NOT deleted")
	if req.ParentID == "" {
		siblingsQ = siblingsQ.Where("parent_id IS NULL")
	} else {
		siblingsQ = siblingsQ.Where("parent_id = ?", req.ParentID)
	}
	var siblings []gormstore.KeywordRow
	if err := siblingsQ.Find(&siblings).Error; err != nil {
		return models.Keyword{}, fmt.Errorf("CreateKeyword: list siblings: %w", err)
	}
	for _, s := range siblings {
		if s.Name == req.Name {
			return models.Keyword{}, ErrKeywordAlreadyExists
		}
	}

	now := models.NowRFC3339()
	row := gormstore.KeywordToRow(models.Keyword{
		Name:        req.Name,
		Description: req.Description,
		Scope:       req.Scope,
		ParentID:    req.ParentID,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err := m.db.WithContext(ctx).Table(m.tables.Keywords).Create(&row).Error; err != nil {
		return models.Keyword{}, fmt.Errorf("CreateKeyword: create: %w", err)
	}
	return gormstore.KeywordFromRow(row), nil
}

// GetKeyword retrieves a Keyword row by its ID, including its direct
// children's IDs.
// Returns [ErrKeywordNotFound] if no keyword with that ID exists.
func (m *gitManager) GetKeyword(ctx context.Context, keywordID string) (models.Keyword, error) {
	var row gormstore.KeywordRow
	err := m.db.WithContext(ctx).Table(m.tables.Keywords).
		Where("id = ? AND NOT deleted", keywordID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Keyword{}, ErrKeywordNotFound
		}
		return models.Keyword{}, fmt.Errorf("GetKeyword %s: %w", keywordID, err)
	}
	kw := gormstore.KeywordFromRow(row)
	childIDs, err := gormstore.KeywordChildIDs(m.db.WithContext(ctx), m.tables, keywordID)
	if err != nil {
		return models.Keyword{}, fmt.Errorf("GetKeyword %s: list children: %w", keywordID, err)
	}
	kw.ChildIDs = childIDs
	return kw, nil
}

// ListKeywords returns Keyword rows matching the given filter.
// When filter.ParentID is empty, root keywords (no parent) are returned.
// Set filter.ParentID to a keyword ID to list its direct children.
func (m *gitManager) ListKeywords(ctx context.Context, filter KeywordFilter) ([]models.Keyword, error) {
	q := m.db.WithContext(ctx).Table(m.tables.Keywords).Where("NOT deleted")
	if filter.ParentID == "" {
		q = q.Where("parent_id IS NULL")
	} else {
		q = q.Where("parent_id = ?", filter.ParentID)
	}
	if filter.Scope != "" {
		q = q.Where("scope = ?", filter.Scope)
	}
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit)
	}
	var rows []gormstore.KeywordRow
	if err := q.Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("ListKeywords: %w", err)
	}
	out := make([]models.Keyword, len(rows))
	for i, r := range rows {
		out[i] = gormstore.KeywordFromRow(r)
	}
	return out, nil
}

// GetKeywordTree returns the full taxonomy subtree rooted at the given
// keywordID, or the full forest of root keywords when keywordID is empty.
func (m *gitManager) GetKeywordTree(ctx context.Context, keywordID string) ([]KeywordTreeNode, error) {
	childIDs, err := gormstore.KeywordChildIDs(m.db.WithContext(ctx), m.tables, keywordID)
	if err != nil {
		return nil, fmt.Errorf("GetKeywordTree: %w", err)
	}
	nodes := make([]KeywordTreeNode, 0, len(childIDs))
	if len(childIDs) == 0 {
		return nodes, nil
	}
	var rows []gormstore.KeywordRow
	if err := m.db.WithContext(ctx).Table(m.tables.Keywords).
		Where("id IN ? AND NOT deleted", childIDs).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("GetKeywordTree: %w", err)
	}
	byID := make(map[string]gormstore.KeywordRow, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	for _, id := range childIDs {
		row, ok := byID[id]
		if !ok {
			continue
		}
		node, err := m.buildKeywordTreeNode(ctx, row)
		if err != nil {
			return nil, fmt.Errorf("GetKeywordTree: build node %s: %w", id, err)
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

// UpdateKeyword updates the mutable fields of a Keyword row.
// Returns [ErrKeywordNotFound] if no keyword with that ID exists.
func (m *gitManager) UpdateKeyword(ctx context.Context, keywordID string, req UpdateKeywordRequest) (models.Keyword, error) {
	result := m.db.WithContext(ctx).Table(m.tables.Keywords).Where("id = ? AND NOT deleted", keywordID).
		Updates(map[string]any{
			"name":        req.Name,
			"description": req.Description,
			"scope":       req.Scope,
			"updated_at":  models.NowRFC3339(),
		})
	if result.Error != nil {
		return models.Keyword{}, fmt.Errorf("UpdateKeyword %s: %w", keywordID, result.Error)
	}
	if result.RowsAffected == 0 {
		return models.Keyword{}, ErrKeywordNotFound
	}
	return m.GetKeyword(ctx, keywordID)
}

// DeleteKeyword removes a Keyword row and re-parents its children to
// the deleted keyword's parent (or promotes them to root if the deleted
// keyword had no parent). Also removes any tagged_with rows referencing the
// deleted keyword — the entitygraph-era version left these dangling; a
// deleted keyword can no longer be a valid tag target, so this cascade is a
// deliberate fix made along the way, not a mechanical port.
// Returns [ErrKeywordNotFound] if no keyword with that ID exists.
func (m *gitManager) DeleteKeyword(ctx context.Context, keywordID string) error {
	kw, err := m.GetKeyword(ctx, keywordID)
	if err != nil {
		return fmt.Errorf("DeleteKeyword %s: %w", keywordID, err)
	}

	if err := m.db.WithContext(ctx).Table(m.tables.Keywords).
		Where("parent_id = ?", keywordID).
		Update("parent_id", gormstore.StringToNullable(kw.ParentID)).Error; err != nil {
		return fmt.Errorf("DeleteKeyword %s: reparent children: %w", keywordID, err)
	}

	if err := m.db.WithContext(ctx).Table(m.tables.BlobKeywordTags).
		Where("keyword_id = ?", keywordID).Delete(&gormstore.BlobKeywordTagRow{}).Error; err != nil {
		return fmt.Errorf("DeleteKeyword %s: remove tagged_with rows: %w", keywordID, err)
	}

	if err := m.db.WithContext(ctx).Table(m.tables.Keywords).
		Where("id = ?", keywordID).Update("deleted", true).Error; err != nil {
		return fmt.Errorf("DeleteKeyword %s: delete row: %w", keywordID, err)
	}
	return nil
}

// buildKeywordTreeNode recursively builds a [KeywordTreeNode] for row.
func (m *gitManager) buildKeywordTreeNode(ctx context.Context, row gormstore.KeywordRow) (KeywordTreeNode, error) {
	kw := gormstore.KeywordFromRow(row)
	childIDs, err := gormstore.KeywordChildIDs(m.db.WithContext(ctx), m.tables, row.ID)
	if err != nil {
		return KeywordTreeNode{}, err
	}
	kw.ChildIDs = childIDs

	childNodes := make([]KeywordTreeNode, 0, len(childIDs))
	if len(childIDs) > 0 {
		var childRows []gormstore.KeywordRow
		if err := m.db.WithContext(ctx).Table(m.tables.Keywords).
			Where("id IN ? AND NOT deleted", childIDs).Find(&childRows).Error; err != nil {
			return KeywordTreeNode{}, err
		}
		byID := make(map[string]gormstore.KeywordRow, len(childRows))
		for _, r := range childRows {
			byID[r.ID] = r
		}
		for _, id := range childIDs {
			childRow, ok := byID[id]
			if !ok {
				continue
			}
			node, err := m.buildKeywordTreeNode(ctx, childRow)
			if err != nil {
				return KeywordTreeNode{}, err
			}
			childNodes = append(childNodes, node)
		}
	}
	return KeywordTreeNode{Keyword: kw, Children: childNodes}, nil
}

// ── Branch-Scoped Edge CRUD ───────────────────────────────────────────────────

// CreateEdge creates a documentation edge between two entities on the
// specified branch. Supported relationship names: "tagged_with", "documents",
// "documented_by", "depends_on", "imported_by", "references", "referenced_by".
// Returns [ErrBranchNotFound] if the branch does not exist.
// Returns [ErrInvalidRelationship] if the relationship name is not valid.
func (m *gitManager) CreateEdge(ctx context.Context, req CreateEdgeRequest) error {
	if !validDocEdges[req.RelationshipName] {
		return fmt.Errorf("CreateEdge: %w: %q", ErrInvalidRelationship, req.RelationshipName)
	}
	if _, err := m.GetBranch(ctx, req.BranchID); err != nil {
		if errors.Is(err, ErrBranchNotFound) {
			return ErrBranchNotFound
		}
		return fmt.Errorf("CreateEdge: get branch %s: %w", req.BranchID, err)
	}

	now := models.NowRFC3339()
	if req.RelationshipName == "tagged_with" {
		row := gormstore.BlobKeywordTagRow{
			BranchID:  req.BranchID,
			BlobID:    req.FromEntityID,
			KeywordID: req.ToEntityID,
			Signal:    strMapProp(req.Properties, "signal"),
			Note:      strMapProp(req.Properties, "note"),
			CreatedAt: now,
		}
		if err := m.db.WithContext(ctx).Table(m.tables.BlobKeywordTags).
			Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return fmt.Errorf("CreateEdge %s (%s→%s): %w", req.RelationshipName, req.FromEntityID, req.ToEntityID, err)
		}
		return nil
	}

	row := gormstore.BlobReferenceRow{
		BranchID:   req.BranchID,
		FromBlobID: req.FromEntityID,
		Name:       req.RelationshipName,
		ToBlobID:   req.ToEntityID,
		Descriptor: strMapProp(req.Properties, "descriptor"),
		CreatedAt:  now,
	}
	if err := m.db.WithContext(ctx).Table(m.tables.BlobReferences).
		Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return fmt.Errorf("CreateEdge %s (%s→%s): %w", req.RelationshipName, req.FromEntityID, req.ToEntityID, err)
	}
	return nil
}

// DeleteEdge removes a documentation edge between two entities.
// Returns [ErrBranchNotFound] if the branch does not exist.
// Returns [ErrEdgeNotFound] if no matching edge exists.
// Returns [ErrInvalidRelationship] if the relationship name is invalid.
func (m *gitManager) DeleteEdge(ctx context.Context, req DeleteEdgeRequest) error {
	if !validDocEdges[req.RelationshipName] {
		return fmt.Errorf("DeleteEdge: %w: %q", ErrInvalidRelationship, req.RelationshipName)
	}
	if _, err := m.GetBranch(ctx, req.BranchID); err != nil {
		if errors.Is(err, ErrBranchNotFound) {
			return ErrBranchNotFound
		}
		return fmt.Errorf("DeleteEdge: get branch %s: %w", req.BranchID, err)
	}

	var result *gorm.DB
	if req.RelationshipName == "tagged_with" {
		result = m.db.WithContext(ctx).Table(m.tables.BlobKeywordTags).
			Where("branch_id = ? AND blob_id = ? AND keyword_id = ?", req.BranchID, req.FromEntityID, req.ToEntityID).
			Delete(&gormstore.BlobKeywordTagRow{})
	} else {
		result = m.db.WithContext(ctx).Table(m.tables.BlobReferences).
			Where("branch_id = ? AND from_blob_id = ? AND name = ? AND to_blob_id = ?",
				req.BranchID, req.FromEntityID, req.RelationshipName, req.ToEntityID).
			Delete(&gormstore.BlobReferenceRow{})
	}
	if result.Error != nil {
		return fmt.Errorf("DeleteEdge %s (%s→%s): %w", req.RelationshipName, req.FromEntityID, req.ToEntityID, result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrEdgeNotFound
	}
	return nil
}
