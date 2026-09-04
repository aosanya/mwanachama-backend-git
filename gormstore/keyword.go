package gormstore

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-git/models"
)

// KeywordRow is the GORM row for a [models.Keyword]. ParentID replaces the
// whole has_child/belongs_to_parent bidirectional edge pair — NULL means a
// root keyword. Deliberately a plain indexed column, not a GORM
// association/foreign key: Migrate scopes AutoMigrate per db.Table(...)
// (see tables.go), but GORM resolves an association's target table from the
// row struct's default name, not that runtime override — same reasoning as
// mwanachama-backend-actor's GroupRow.ParentID.
type KeywordRow struct {
	ID          string  `gorm:"primaryKey"`
	ParentID    *string `gorm:"index"`
	Name        string
	Description string
	Scope       string `gorm:"index"`
	CreatedAt   string
	UpdatedAt   string
	Deleted     bool `gorm:"index"`
}

func (r *KeywordRow) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

// KeywordToRow converts a domain Keyword to its row shape. ChildIDs is
// ignored — it is always derived, never stored (see [models.Keyword]'s doc).
func KeywordToRow(k models.Keyword) KeywordRow {
	return KeywordRow{
		ID:          k.ID,
		ParentID:    StringToNullable(k.ParentID),
		Name:        k.Name,
		Description: k.Description,
		Scope:       k.Scope,
		CreatedAt:   k.CreatedAt,
		UpdatedAt:   k.UpdatedAt,
	}
}

// KeywordFromRow converts a row back to the domain Keyword. ChildIDs is left
// nil — the caller attaches it separately via a query on ParentID.
func KeywordFromRow(r KeywordRow) models.Keyword {
	return models.Keyword{
		ID:          r.ID,
		ParentID:    NullableToString(r.ParentID),
		Name:        r.Name,
		Description: r.Description,
		Scope:       r.Scope,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}
