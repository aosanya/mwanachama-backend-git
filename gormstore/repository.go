package gormstore

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-git/models"
)

// RepositoryRow is the GORM row for a [models.Repository].
//
// AgencyID replaces the old has_repository/belongs_to_agency edge pair.
// BareClonePath is a row-only field (not on the domain type): the local
// filesystem path of the bare shallow clone created by lazy import v2,
// reused by FetchBranch/loadBlobContentFromBareClone.
type RepositoryRow struct {
	ID            string  `gorm:"primaryKey"`
	AgencyID      *string `gorm:"index"`
	Name          string  `gorm:"index"`
	Description   string
	DefaultBranch string
	BareClonePath string
	SourceURL     string
	CreatedAt     string
	UpdatedAt     string
	Deleted       bool `gorm:"index"`
}

func (r *RepositoryRow) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

// RepositoryToRow converts a domain Repository to its row shape.
func RepositoryToRow(r models.Repository) RepositoryRow {
	return RepositoryRow{
		ID:            r.ID,
		Name:          r.Name,
		Description:   r.Description,
		DefaultBranch: r.DefaultBranch,
		SourceURL:     r.SourceURL,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}

// RepositoryFromRow converts a row back to the domain Repository.
func RepositoryFromRow(row RepositoryRow) models.Repository {
	return models.Repository{
		ID:            row.ID,
		Name:          row.Name,
		Description:   row.Description,
		DefaultBranch: row.DefaultBranch,
		SourceURL:     row.SourceURL,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

// StringToNullable maps "" to a nil *string — the nullable-column spelling
// of "no value". Exported for impl files that write a nullable FK column
// directly rather than through a full row converter.
func StringToNullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// NullableToString maps a nil *string back to "".
func NullableToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
