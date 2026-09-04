// git_impl_edgelifecycle.go implements the DR-010 documentation-edge lifecycle
// hooks for [gitManager]:
//
//   - [gitManager.replicateDocEdges]       — GIT-022a: copy branch-scoped
//     edges to the default branch after MergeBranch.
//
//   - [gitManager.deleteDocEdgesForBranch] — GIT-022b: remove branch-scoped
//     edges when a branch is deleted without merging.
//
//   - [gitManager.deleteDocEdgesForBlob]   — GIT-022c: remove branch-scoped
//     edges tied to a specific blob (called by DeleteFile before the
//     deletion commit).
//
// Only two edge types carry branch-scoped documentation semantics:
//
//   - "tagged_with"  (Blob -> Keyword): discovery labels; no descriptor.
//   - "references"   (Blob -> Blob): generic semantic edge; carries a required
//     "descriptor" property (e.g. "documents", "depends_on", "contradicts").
//
// Note this replicates only the "references" direction, never "referenced_by"
// — the entitygraph-era version's docEdgeTypes list never included
// "referenced_by" either, so an inverse edge auto-created by entitygraph on
// the source branch never made it to the default branch. Preserved as-is.
package mwanachamagit

import (
	"context"
	"fmt"
	"log"

	"gorm.io/gorm/clause"

	"github.com/aosanya/mwanachama-backend-git/gormstore"
	"github.com/aosanya/mwanachama-backend-git/models"
)

// ── GIT-022a: Replicate edges on MergeBranch ─────────────────────────────────

// replicateDocEdges copies all branch-scoped "tagged_with" and "references"
// rows from the source branch to the default branch, for every blob
// reachable from headCommitID.
func (m *gitManager) replicateDocEdges(ctx context.Context, sourceBranchID, defaultBranchID, headCommitID string) {
	if headCommitID == "" {
		return
	}
	blobs, err := m.allBlobsAtCommit(ctx, headCommitID)
	if err != nil {
		return
	}
	blobIDs := make([]string, len(blobs))
	for i, b := range blobs {
		blobIDs[i] = b.ID
	}
	if len(blobIDs) == 0 {
		return
	}

	m.replicateTaggedWith(ctx, blobIDs, sourceBranchID, defaultBranchID)
	m.replicateReferences(ctx, blobIDs, sourceBranchID, defaultBranchID)
}

// replicateTaggedWith copies tagged_with rows for blobIDs from sourceBranchID
// to defaultBranchID. Duplicate rows are silently ignored (ON CONFLICT DO
// NOTHING) — best-effort, matching the entitygraph-era "log and continue".
func (m *gitManager) replicateTaggedWith(ctx context.Context, blobIDs []string, sourceBranchID, defaultBranchID string) {
	var rows []gormstore.BlobKeywordTagRow
	if err := m.db.WithContext(ctx).Table(m.tables.BlobKeywordTags).
		Where("branch_id = ? AND blob_id IN ?", sourceBranchID, blobIDs).Find(&rows).Error; err != nil {
		return
	}
	now := models.NowRFC3339()
	for _, r := range rows {
		newRow := gormstore.BlobKeywordTagRow{
			BranchID: defaultBranchID, BlobID: r.BlobID, KeywordID: r.KeywordID,
			Signal: r.Signal, Note: r.Note, CreatedAt: now,
		}
		if err := m.db.WithContext(ctx).Table(m.tables.BlobKeywordTags).
			Clauses(clause.OnConflict{DoNothing: true}).Create(&newRow).Error; err != nil {
			log.Printf("[replicateDocEdges] tagged_with (%s→%s): %v", r.BlobID, r.KeywordID, err)
		}
	}
}

// replicateReferences copies "references" rows (never "referenced_by" — see
// this file's doc) for blobIDs from sourceBranchID to defaultBranchID.
func (m *gitManager) replicateReferences(ctx context.Context, blobIDs []string, sourceBranchID, defaultBranchID string) {
	var rows []gormstore.BlobReferenceRow
	if err := m.db.WithContext(ctx).Table(m.tables.BlobReferences).
		Where("branch_id = ? AND from_blob_id IN ? AND name = ?", sourceBranchID, blobIDs, "references").
		Find(&rows).Error; err != nil {
		return
	}
	now := models.NowRFC3339()
	for _, r := range rows {
		newRow := gormstore.BlobReferenceRow{
			BranchID: defaultBranchID, FromBlobID: r.FromBlobID, Name: r.Name, ToBlobID: r.ToBlobID,
			Descriptor: r.Descriptor, CreatedAt: now,
		}
		if err := m.db.WithContext(ctx).Table(m.tables.BlobReferences).
			Clauses(clause.OnConflict{DoNothing: true}).Create(&newRow).Error; err != nil {
			log.Printf("[replicateDocEdges] references (%s→%s): %v", r.FromBlobID, r.ToBlobID, err)
		}
	}
}

// ── GIT-022b: Delete edges on branch delete ───────────────────────────────────

// deleteDocEdgesForBranch removes all branch-scoped "tagged_with" and
// "references" rows on every blob reachable from headCommitID whose
// branch_id equals branchID.
func (m *gitManager) deleteDocEdgesForBranch(ctx context.Context, branchID, headCommitID string) {
	if headCommitID == "" {
		return
	}
	blobs, err := m.allBlobsAtCommit(ctx, headCommitID)
	if err != nil {
		return
	}
	blobIDs := make([]string, len(blobs))
	for i, b := range blobs {
		blobIDs[i] = b.ID
	}
	if len(blobIDs) == 0 {
		return
	}
	_ = m.db.WithContext(ctx).Table(m.tables.BlobKeywordTags).
		Where("branch_id = ? AND blob_id IN ?", branchID, blobIDs).
		Delete(&gormstore.BlobKeywordTagRow{}).Error
	_ = m.db.WithContext(ctx).Table(m.tables.BlobReferences).
		Where("branch_id = ? AND from_blob_id IN ? AND name = ?", branchID, blobIDs, "references").
		Delete(&gormstore.BlobReferenceRow{}).Error
}

// ── GIT-022c: Remove edges on file delete ────────────────────────────────────

// deleteDocEdgesForBlob removes all branch-scoped "tagged_with" and
// "references" rows tied to a specific blob whose branch_id equals branchID.
func (m *gitManager) deleteDocEdgesForBlob(ctx context.Context, blobID, branchID string) {
	_ = m.db.WithContext(ctx).Table(m.tables.BlobKeywordTags).
		Where("branch_id = ? AND blob_id = ?", branchID, blobID).
		Delete(&gormstore.BlobKeywordTagRow{}).Error
	_ = m.db.WithContext(ctx).Table(m.tables.BlobReferences).
		Where("branch_id = ? AND from_blob_id = ? AND name = ?", branchID, blobID, "references").
		Delete(&gormstore.BlobReferenceRow{}).Error
}

// ── helpers ───────────────────────────────────────────────────────────────────

// strMapProp retrieves a string value from a map[string]any, returning ""
// if the key is absent or the value is not a string.
func strMapProp(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// ── MigrateDocEdges — GIT-022c (rename/move path migration) ──────────────────

// MigrateDocEdges re-targets all branch-scoped "tagged_with" and "references"
// rows from the blob at oldBlobID to the blob at newBlobID on the given
// branch. Not yet wired into the interface — exported for future use by a
// RenameFile method, when one is implemented.
func (m *gitManager) MigrateDocEdges(ctx context.Context, branchID, headCommitID, oldBlobID, newBlobID string) error {
	if err := m.db.WithContext(ctx).Table(m.tables.BlobKeywordTags).
		Where("branch_id = ? AND blob_id = ?", branchID, oldBlobID).
		Update("blob_id", newBlobID).Error; err != nil {
		return fmt.Errorf("MigrateDocEdges tagged_with: %w", err)
	}
	if err := m.db.WithContext(ctx).Table(m.tables.BlobReferences).
		Where("branch_id = ? AND from_blob_id = ? AND name = ?", branchID, oldBlobID, "references").
		Update("from_blob_id", newBlobID).Error; err != nil {
		return fmt.Errorf("MigrateDocEdges references: %w", err)
	}
	return nil
}
