package gormstore

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-git/models"
)

// TagRow is the GORM row for a [models.Tag]. No UpdatedAt column — tags are
// immutable once created. RepositoryID/CommitID replace the old
// belongs_to_repository/points_to edges.
type TagRow struct {
	ID           string  `gorm:"primaryKey"`
	RepositoryID *string `gorm:"index"`
	CommitID     *string `gorm:"index"`
	Name         string  `gorm:"index"`
	SHA          string
	Message      string
	TaggerName   string
	TaggerAt     string
	CreatedAt    string
	Deleted      bool `gorm:"index"`
}

func (r *TagRow) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

// TagToRow converts a domain Tag to its row shape. commitID is the resolved
// Commit row ID the tag points to (models.Tag carries no CommitID field).
func TagToRow(t models.Tag, commitID string) TagRow {
	return TagRow{
		ID:           t.ID,
		RepositoryID: StringToNullable(t.RepositoryID),
		CommitID:     StringToNullable(commitID),
		Name:         t.Name,
		SHA:          t.SHA,
		Message:      t.Message,
		TaggerName:   t.TaggerName,
		TaggerAt:     t.TaggerAt,
		CreatedAt:    t.CreatedAt,
	}
}

// TagFromRow converts a row back to the domain Tag.
func TagFromRow(r TagRow) models.Tag {
	return models.Tag{
		ID:           r.ID,
		RepositoryID: NullableToString(r.RepositoryID),
		Name:         r.Name,
		SHA:          r.SHA,
		Message:      r.Message,
		TaggerName:   r.TaggerName,
		TaggerAt:     r.TaggerAt,
		CreatedAt:    r.CreatedAt,
	}
}
