package gormstore

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-git/models"
)

// BlobRow is the GORM row for a [models.Blob]. No TreeID column: dead in
// the entitygraph era too (never populated, never read — see
// models.Blob.TreeID's doc) and structurally wrong besides, since a blob can
// be reachable from many trees (see [TreeBlobRow]). SHA is intentionally
// NOT unique — see [CommitRow]'s doc for why.
type BlobRow struct {
	ID        string `gorm:"primaryKey"`
	SHA       string `gorm:"index"`
	Path      string `gorm:"index"`
	Name      string
	Extension string
	Size      int64
	Encoding  string
	Content   string
	// Data is the base64-encoded raw git blob object, written by WriteFile only.
	Data      string
	CreatedAt string
	Deleted   bool `gorm:"index"`
}

func (r *BlobRow) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

// BlobToRow converts a domain Blob to its row shape.
func BlobToRow(b models.Blob) BlobRow {
	return BlobRow{
		ID:        b.ID,
		SHA:       b.SHA,
		Path:      b.Path,
		Name:      b.Name,
		Extension: b.Extension,
		Size:      b.Size,
		Encoding:  b.Encoding,
		Content:   b.Content,
		CreatedAt: b.CreatedAt,
	}
}

// BlobFromRow converts a row back to the domain Blob.
func BlobFromRow(r BlobRow) models.Blob {
	return models.Blob{
		ID:        r.ID,
		SHA:       r.SHA,
		Path:      r.Path,
		Name:      r.Name,
		Extension: r.Extension,
		Size:      r.Size,
		Encoding:  r.Encoding,
		Content:   r.Content,
		CreatedAt: r.CreatedAt,
	}
}
