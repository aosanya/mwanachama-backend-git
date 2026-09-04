package gormstore

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-git/models"
)

// ImportJobRow is the GORM row for a [models.ImportJob]. ProgressSteps has
// no column — it lives only in the root package's in-process importJobs map
// while the goroutine is active.
type ImportJobRow struct {
	ID            string `gorm:"primaryKey"`
	Name          string
	SourceURL     string
	DefaultBranch string
	Status        string `gorm:"index"`
	ErrorMessage  string
	CreatedAt     string
	UpdatedAt     string
	Deleted       bool `gorm:"index"`
}

func (r *ImportJobRow) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

// ImportJobToRow converts a domain ImportJob to its row shape. ProgressSteps
// is dropped — see [ImportJobRow]'s doc.
func ImportJobToRow(j models.ImportJob) ImportJobRow {
	return ImportJobRow{
		ID:            j.ID,
		Name:          j.Name,
		SourceURL:     j.SourceURL,
		DefaultBranch: j.DefaultBranch,
		Status:        j.Status,
		ErrorMessage:  j.ErrorMessage,
		CreatedAt:     j.CreatedAt,
		UpdatedAt:     j.UpdatedAt,
	}
}

// ImportJobFromRow converts a row back to the domain ImportJob. ProgressSteps
// is left nil — the caller re-attaches it from the in-process map.
func ImportJobFromRow(r ImportJobRow) models.ImportJob {
	return models.ImportJob{
		ID:            r.ID,
		Name:          r.Name,
		SourceURL:     r.SourceURL,
		DefaultBranch: r.DefaultBranch,
		Status:        r.Status,
		ErrorMessage:  r.ErrorMessage,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}

// FetchBranchJobRow is the GORM row for a [models.FetchBranchJob].
type FetchBranchJobRow struct {
	ID           string `gorm:"primaryKey"`
	RepoID       string `gorm:"index"`
	BranchName   string
	Status       string
	ErrorMessage string
	CreatedAt    string
	UpdatedAt    string
	Deleted      bool `gorm:"index"`
}

func (r *FetchBranchJobRow) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

// FetchBranchJobToRow converts a domain FetchBranchJob to its row shape.
func FetchBranchJobToRow(j models.FetchBranchJob) FetchBranchJobRow {
	return FetchBranchJobRow{
		ID:           j.ID,
		RepoID:       j.RepoID,
		BranchName:   j.BranchName,
		Status:       j.Status,
		ErrorMessage: j.ErrorMessage,
		CreatedAt:    j.CreatedAt,
		UpdatedAt:    j.UpdatedAt,
	}
}

// FetchBranchJobFromRow converts a row back to the domain FetchBranchJob.
func FetchBranchJobFromRow(r FetchBranchJobRow) models.FetchBranchJob {
	return models.FetchBranchJob{
		ID:           r.ID,
		RepoID:       r.RepoID,
		BranchName:   r.BranchName,
		Status:       r.Status,
		ErrorMessage: r.ErrorMessage,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}
