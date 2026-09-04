package gormstore

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-git/models"
)

// TreeRow is the GORM row for a [models.Tree]. No CommitID column — that's
// the inverse of CommitRow.TreeID, resolved by query only where a caller
// actually needs it. BlobIDs/SubtreeIDs have no columns — see
// [TreeBlobRow]/[TreeSubtreeRow].
type TreeRow struct {
	ID  string `gorm:"primaryKey"`
	SHA string `gorm:"index"`
	// Path is the directory path within the commit tree hierarchy. Empty
	// denotes the root tree of a commit.
	Path string
	// Entries is a JSON array of child entries in the form
	// [{"name":"","mode":"100644","sha":""}], serialised at write time.
	Entries string
	// Data is the base64-encoded raw git tree object, written by WriteFile only.
	Data      string
	Size      int64
	CreatedAt string
	Deleted   bool `gorm:"index"`
}

func (r *TreeRow) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

// TreeToRow converts a domain Tree to its row shape. BlobIDs/SubtreeIDs are
// ignored here — see [TreeBlobsToRows]/[TreeSubtreesToRows].
func TreeToRow(t models.Tree) TreeRow {
	return TreeRow{
		ID:        t.ID,
		SHA:       t.SHA,
		Path:      t.Path,
		CreatedAt: t.CreatedAt,
	}
}

// TreeFromRow converts a row back to the domain Tree. BlobIDs/SubtreeIDs/
// CommitID are left unset — see [TreeRow]'s doc.
func TreeFromRow(r TreeRow) models.Tree {
	return models.Tree{
		ID:        r.ID,
		SHA:       r.SHA,
		Path:      r.Path,
		CreatedAt: r.CreatedAt,
	}
}

// TreeBlobRow is the GORM row for one (tree, blob) membership, replacing the
// old has_blob/belongs_to_tree edge pair. Many-to-many: a blob deduplicated
// by SHA can be reachable from more than one tree.
type TreeBlobRow struct {
	TreeID string `gorm:"primaryKey"`
	BlobID string `gorm:"primaryKey;index"`
}

// TreeSubtreeRow is the GORM row for one (tree, subtree) edge, replacing the
// old has_subtree edge.
type TreeSubtreeRow struct {
	TreeID    string `gorm:"primaryKey"`
	SubtreeID string `gorm:"primaryKey;index"`
}
