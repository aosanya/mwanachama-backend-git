// Package gormstore holds every GORM-specific piece of this repo: row
// structs, their conversion to/from the domain types in
// mwanachama-backend-git/models, and table migration. Nothing outside this
// package (and the root mwanachama-backend-git package's *_impl.go files,
// which call it) needs to know GORM exists.
package gormstore

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-git/models"
)

// AgencyRow is the GORM row for a [models.Agency].
type AgencyRow struct {
	ID          string `gorm:"primaryKey"`
	Name        string
	Description string
	CreatedAt   string
	UpdatedAt   string
	Deleted     bool `gorm:"index"`
}

// BeforeCreate mints an id via uuid.NewString() when the caller left one unset.
func (r *AgencyRow) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

// AgencyToRow converts a domain Agency to its row shape.
func AgencyToRow(a models.Agency) AgencyRow {
	return AgencyRow{
		ID:          a.ID,
		Name:        a.Name,
		Description: a.Description,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}
}

// AgencyFromRow converts a row back to the domain Agency.
func AgencyFromRow(r AgencyRow) models.Agency {
	return models.Agency{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}
