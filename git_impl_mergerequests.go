// git_impl_mergerequests.go — MergeRequest CRUD implementation for [gitManager].
//
// A MergeRequest is the durable record of a request to merge a source branch
// into a target branch. The actual merge work is delegated to MergeBranch;
// this layer adds lifecycle (open -> merged | closed | failed), workflow_run_id
// propagation, and the git.merge.* event topics required by FEAT-20260602-001.
package mwanachamagit

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-git/gormstore"
	"github.com/aosanya/mwanachama-backend-git/models"
)

// CreateMergeRequest opens a new MergeRequest for the given source branch.
// Publishes [TopicMergeRequested] on success.
func (m *gitManager) CreateMergeRequest(ctx context.Context, req CreateMergeRequestRequest) (models.MergeRequest, error) {
	if req.Title == "" {
		return models.MergeRequest{}, fmt.Errorf("CreateMergeRequest: title is required")
	}
	if req.SourceBranchID == "" {
		return models.MergeRequest{}, fmt.Errorf("CreateMergeRequest: source_branch_id is required")
	}
	repo, err := m.GetRepository(ctx, req.RepositoryID)
	if err != nil {
		return models.MergeRequest{}, fmt.Errorf("CreateMergeRequest: %w", err)
	}
	source, err := m.GetBranch(ctx, req.SourceBranchID)
	if err != nil {
		return models.MergeRequest{}, fmt.Errorf("CreateMergeRequest: source branch: %w", err)
	}

	var target models.Branch
	if req.TargetBranchID != "" {
		target, err = m.GetBranch(ctx, req.TargetBranchID)
		if err != nil {
			return models.MergeRequest{}, fmt.Errorf("CreateMergeRequest: target branch: %w", err)
		}
	} else {
		target, err = m.defaultBranch(ctx, repo.ID)
		if err != nil {
			return models.MergeRequest{}, fmt.Errorf("CreateMergeRequest: default branch: %w", err)
		}
	}

	now := models.NowRFC3339()
	row := gormstore.MergeRequestToRow(models.MergeRequest{
		Title:            req.Title,
		Description:      req.Description,
		SourceBranchID:   source.ID,
		SourceBranchName: source.Name,
		TargetBranchID:   target.ID,
		TargetBranchName: target.Name,
		Status:           models.MergeRequestStatusOpen,
		AuthorName:       req.AuthorName,
		WorkflowRunID:    req.WorkflowRunID,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	row.RepositoryID = gormstore.StringToNullable(repo.ID)
	if err := m.db.WithContext(ctx).Table(m.tables.MergeRequests).Create(&row).Error; err != nil {
		return models.MergeRequest{}, fmt.Errorf("CreateMergeRequest: create: %w", err)
	}

	mr := gormstore.MergeRequestFromRow(row)
	m.publish(ctx, TopicMergeRequested, MergeRequestRequestedPayload{
		MergeRequestID: mr.ID,
		RepoID:         repo.ID,
		SourceBranchID: source.ID,
		TargetBranchID: target.ID,
		Title:          mr.Title,
		WorkflowRunID:  mr.WorkflowRunID,
	})
	return mr, nil
}

// GetMergeRequest retrieves a MergeRequest by ID.
func (m *gitManager) GetMergeRequest(ctx context.Context, mrID string) (models.MergeRequest, error) {
	var row gormstore.MergeRequestRow
	err := m.db.WithContext(ctx).Table(m.tables.MergeRequests).
		Where("id = ? AND NOT deleted", mrID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.MergeRequest{}, ErrMergeRequestNotFound
		}
		return models.MergeRequest{}, fmt.Errorf("GetMergeRequest: %w", err)
	}
	return gormstore.MergeRequestFromRow(row), nil
}

// ListMergeRequests returns MRs matching the filter. An empty filter returns
// every MR. Unlike the entitygraph era (which had to list every repository
// and concatenate their MRs, since there was no cross-repository "list all"
// query), this is a single filtered SELECT.
func (m *gitManager) ListMergeRequests(ctx context.Context, filter MergeRequestFilter) ([]models.MergeRequest, error) {
	q := m.db.WithContext(ctx).Table(m.tables.MergeRequests).Where("NOT deleted")
	if filter.RepositoryID != "" {
		q = q.Where("repository_id = ?", filter.RepositoryID)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.WorkflowRunID != "" {
		q = q.Where("workflow_run_id = ?", filter.WorkflowRunID)
	}
	var rows []gormstore.MergeRequestRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("ListMergeRequests: %w", err)
	}
	out := make([]models.MergeRequest, len(rows))
	for i, r := range rows {
		out[i] = gormstore.MergeRequestFromRow(r)
	}
	return out, nil
}

// CompleteMergeRequest performs the merge and transitions the MR to "merged".
// On failure transitions to "failed" and publishes [TopicMergeFailed].
func (m *gitManager) CompleteMergeRequest(ctx context.Context, mrID string) (models.MergeRequest, error) {
	mr, err := m.GetMergeRequest(ctx, mrID)
	if err != nil {
		return models.MergeRequest{}, err
	}
	if mr.Status != models.MergeRequestStatusOpen {
		return models.MergeRequest{}, ErrMergeRequestNotOpen
	}

	mergedBranch, mergeErr := m.MergeBranch(ctx, mr.SourceBranchID)
	if mergeErr != nil {
		updated, _ := m.transitionMRStatus(ctx, mr.ID, models.MergeRequestStatusFailed, "", mergeErr.Error())
		m.publish(ctx, TopicMergeFailed, MergeRequestFailedPayload{
			MergeRequestID: mr.ID,
			RepoID:         mr.RepositoryID,
			SourceBranchID: mr.SourceBranchID,
			ErrorMessage:   mergeErr.Error(),
			WorkflowRunID:  mr.WorkflowRunID,
		})
		if updated.ID == "" {
			return mr, fmt.Errorf("CompleteMergeRequest: %w", mergeErr)
		}
		return updated, fmt.Errorf("CompleteMergeRequest: %w", mergeErr)
	}

	updated, err := m.transitionMRStatus(ctx, mr.ID, models.MergeRequestStatusMerged, mergedBranch.SHA, "")
	if err != nil {
		return mr, fmt.Errorf("CompleteMergeRequest: persist merged status: %w", err)
	}
	m.publish(ctx, TopicMergeCompleted, MergeRequestCompletedPayload{
		MergeRequestID:  mr.ID,
		RepoID:          mr.RepositoryID,
		SourceBranchID:  mr.SourceBranchID,
		TargetBranchID:  mr.TargetBranchID,
		MergedCommitSHA: mergedBranch.SHA,
		WorkflowRunID:   mr.WorkflowRunID,
	})
	return updated, nil
}

// CloseMergeRequest transitions an open MR to "closed" without merging.
func (m *gitManager) CloseMergeRequest(ctx context.Context, mrID string) (models.MergeRequest, error) {
	mr, err := m.GetMergeRequest(ctx, mrID)
	if err != nil {
		return models.MergeRequest{}, err
	}
	if mr.Status != models.MergeRequestStatusOpen {
		return models.MergeRequest{}, ErrMergeRequestNotOpen
	}
	updated, err := m.transitionMRStatus(ctx, mr.ID, models.MergeRequestStatusClosed, "", "")
	if err != nil {
		return mr, fmt.Errorf("CloseMergeRequest: %w", err)
	}
	return updated, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// transitionMRStatus updates status, merged_commit_sha, error_message, and
// updated_at on the MR row and returns the re-read [models.MergeRequest].
func (m *gitManager) transitionMRStatus(ctx context.Context, mrID, status, mergedSHA, errMsg string) (models.MergeRequest, error) {
	updates := map[string]any{
		"status":     status,
		"updated_at": models.NowRFC3339(),
	}
	if mergedSHA != "" {
		updates["merged_commit_sha"] = mergedSHA
	}
	if errMsg != "" {
		updates["error_message"] = errMsg
	}
	if err := m.db.WithContext(ctx).Table(m.tables.MergeRequests).
		Where("id = ?", mrID).Updates(updates).Error; err != nil {
		return models.MergeRequest{}, err
	}
	return m.GetMergeRequest(ctx, mrID)
}
