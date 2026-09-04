package gormstore

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-git/models"
)

// MergeRequestRow is the GORM row for a [models.MergeRequest].
//
// RepositoryID/SourceBranchID/TargetBranchID replace the old
// belongs_to_repository/has_source_branch/has_target_branch edges.
type MergeRequestRow struct {
	ID               string  `gorm:"primaryKey"`
	RepositoryID     *string `gorm:"index"`
	SourceBranchID   *string `gorm:"index"`
	TargetBranchID   *string `gorm:"index"`
	Title            string
	Description      string
	SourceBranchName string
	TargetBranchName string
	Status           string `gorm:"index"`
	MergedCommitSHA  string
	AuthorName       string
	ErrorMessage     string
	WorkflowRunID    string `gorm:"index"`
	CreatedAt        string
	UpdatedAt        string
	Deleted          bool `gorm:"index"`
}

func (r *MergeRequestRow) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

// MergeRequestToRow converts a domain MergeRequest to its row shape.
func MergeRequestToRow(mr models.MergeRequest) MergeRequestRow {
	return MergeRequestRow{
		ID:               mr.ID,
		RepositoryID:     StringToNullable(mr.RepositoryID),
		SourceBranchID:   StringToNullable(mr.SourceBranchID),
		TargetBranchID:   StringToNullable(mr.TargetBranchID),
		Title:            mr.Title,
		Description:      mr.Description,
		SourceBranchName: mr.SourceBranchName,
		TargetBranchName: mr.TargetBranchName,
		Status:           mr.Status,
		MergedCommitSHA:  mr.MergedCommitSHA,
		AuthorName:       mr.AuthorName,
		ErrorMessage:     mr.ErrorMessage,
		WorkflowRunID:    mr.WorkflowRunID,
		CreatedAt:        mr.CreatedAt,
		UpdatedAt:        mr.UpdatedAt,
	}
}

// MergeRequestFromRow converts a row back to the domain MergeRequest.
func MergeRequestFromRow(r MergeRequestRow) models.MergeRequest {
	return models.MergeRequest{
		ID:               r.ID,
		RepositoryID:     NullableToString(r.RepositoryID),
		SourceBranchID:   NullableToString(r.SourceBranchID),
		TargetBranchID:   NullableToString(r.TargetBranchID),
		Title:            r.Title,
		Description:      r.Description,
		SourceBranchName: r.SourceBranchName,
		TargetBranchName: r.TargetBranchName,
		Status:           r.Status,
		MergedCommitSHA:  r.MergedCommitSHA,
		AuthorName:       r.AuthorName,
		ErrorMessage:     r.ErrorMessage,
		WorkflowRunID:    r.WorkflowRunID,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}
}
