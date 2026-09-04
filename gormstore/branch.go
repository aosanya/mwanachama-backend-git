package gormstore

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-git/models"
)

// BranchRow is the GORM row for a [models.Branch].
//
// RepositoryID replaces the old has_branch/belongs_to_repository edge pair
// (and its dual-direction self-healing fallback — a single FK column cannot
// disagree with itself). HeadCommitID replaces the points_to edge. Status,
// SourceURL, and ErrorMessage were entitygraph properties read via raw
// property lookups; they are real columns here.
type BranchRow struct {
	ID            string  `gorm:"primaryKey"`
	RepositoryID  *string `gorm:"index"`
	Name          string  `gorm:"index"`
	IsDefault     bool
	HeadCommitID  *string `gorm:"index"`
	SHA           string
	Status        string
	SourceURL     string
	ErrorMessage  string
	WorkflowRunID string `gorm:"index"`
	CreatedAt     string
	UpdatedAt     string
	Deleted       bool `gorm:"index"`
}

func (r *BranchRow) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

// BranchToRow converts a domain Branch to its row shape.
func BranchToRow(b models.Branch) BranchRow {
	return BranchRow{
		ID:            b.ID,
		RepositoryID:  StringToNullable(b.RepositoryID),
		Name:          b.Name,
		IsDefault:     b.IsDefault,
		HeadCommitID:  StringToNullable(b.HeadCommitID),
		SHA:           b.SHA,
		WorkflowRunID: b.WorkflowRunID,
		CreatedAt:     b.CreatedAt,
		UpdatedAt:     b.UpdatedAt,
	}
}

// BranchFromRow converts a row back to the domain Branch.
func BranchFromRow(r BranchRow) models.Branch {
	return models.Branch{
		ID:            r.ID,
		RepositoryID:  NullableToString(r.RepositoryID),
		Name:          r.Name,
		IsDefault:     r.IsDefault,
		HeadCommitID:  NullableToString(r.HeadCommitID),
		SHA:           r.SHA,
		WorkflowRunID: r.WorkflowRunID,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}
