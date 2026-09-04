// git_impl_branch.go — Branch management implementation for [gitManager].
package mwanachamagit

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-git/gormstore"
	"github.com/aosanya/mwanachama-backend-git/models"
)

// ── Branch Management ─────────────────────────────────────────────────────────

// CreateBranch creates a new Branch row from the specified source branch.
// If req.FromBranchID is empty, the repository default branch is used.
// Returns [ErrRepoNotInitialised] if no repository with that ID exists.
// Returns [ErrBranchExists] if a branch with the given name already exists.
func (m *gitManager) CreateBranch(ctx context.Context, req CreateBranchRequest) (models.Branch, error) {
	repo, err := m.GetRepository(ctx, req.RepositoryID)
	if err != nil {
		return models.Branch{}, fmt.Errorf("CreateBranch: %w", err)
	}

	var sourceBranch models.Branch
	if req.FromBranchID != "" {
		sourceBranch, err = m.GetBranch(ctx, req.FromBranchID)
		if err != nil {
			return models.Branch{}, fmt.Errorf("CreateBranch: source branch: %w", err)
		}
	} else {
		sourceBranch, err = m.defaultBranch(ctx, repo.ID)
		if err != nil {
			return models.Branch{}, fmt.Errorf("CreateBranch: default branch: %w", err)
		}
	}

	var count int64
	if err := m.db.WithContext(ctx).Table(m.tables.Branches).
		Where("repository_id = ? AND name = ? AND NOT deleted", repo.ID, req.Name).
		Count(&count).Error; err != nil {
		return models.Branch{}, fmt.Errorf("CreateBranch: check existing: %w", err)
	}
	if count > 0 {
		return models.Branch{}, ErrBranchExists
	}

	now := models.NowRFC3339()
	row := gormstore.BranchToRow(models.Branch{
		Name:          req.Name,
		IsDefault:     false,
		HeadCommitID:  sourceBranch.HeadCommitID,
		SHA:           sourceBranch.SHA,
		WorkflowRunID: req.WorkflowRunID,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	row.RepositoryID = gormstore.StringToNullable(repo.ID)
	if err := m.db.WithContext(ctx).Table(m.tables.Branches).Create(&row).Error; err != nil {
		return models.Branch{}, fmt.Errorf("CreateBranch: create: %w", err)
	}
	return gormstore.BranchFromRow(row), nil
}

// GetBranch retrieves a Branch row by its ID.
// Returns [ErrBranchNotFound] if no branch with that ID exists.
func (m *gitManager) GetBranch(ctx context.Context, branchID string) (models.Branch, error) {
	var row gormstore.BranchRow
	err := m.db.WithContext(ctx).Table(m.tables.Branches).
		Where("id = ? AND NOT deleted", branchID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Branch{}, ErrBranchNotFound
		}
		return models.Branch{}, fmt.Errorf("GetBranch: %w", err)
	}
	return gormstore.BranchFromRow(row), nil
}

// ListBranches returns all Branch rows for the specified repository.
// Returns [ErrRepoNotInitialised] if no repository with that ID exists.
func (m *gitManager) ListBranches(ctx context.Context, repoID string) ([]models.Branch, error) {
	if _, err := m.GetRepository(ctx, repoID); err != nil {
		return nil, fmt.Errorf("ListBranches: %w", err)
	}
	rows, err := m.listBranchesByRepo(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("ListBranches: %w", err)
	}
	out := make([]models.Branch, len(rows))
	for i, r := range rows {
		out[i] = gormstore.BranchFromRow(r)
	}
	return out, nil
}

// GetBranchByName retrieves a Branch row by its human-readable name.
// Returns [ErrBranchNotFound] if no branch with that name exists for the
// specified repository.
func (m *gitManager) GetBranchByName(ctx context.Context, repoID string, branchName string) (models.Branch, error) {
	var row gormstore.BranchRow
	err := m.db.WithContext(ctx).Table(m.tables.Branches).
		Where("repository_id = ? AND name = ? AND NOT deleted", repoID, branchName).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Branch{}, ErrBranchNotFound
		}
		return models.Branch{}, fmt.Errorf("GetBranchByName: %w", err)
	}
	return gormstore.BranchFromRow(row), nil
}

// DeleteBranch removes a Branch row.
// Returns [ErrBranchNotFound] if no branch with that ID exists.
// Returns [ErrDefaultBranchDeleteForbidden] if branchID is the default branch.
func (m *gitManager) DeleteBranch(ctx context.Context, branchID string) error {
	branch, err := m.GetBranch(ctx, branchID)
	if err != nil {
		return err
	}
	if branch.IsDefault {
		return fmt.Errorf("DeleteBranch: %w", ErrDefaultBranchDeleteForbidden)
	}

	// GIT-022b: Delete branch-scoped documentation edges before removing the
	// branch row so no dangling edges are left behind.
	m.deleteDocEdgesForBranch(ctx, branchID, branch.HeadCommitID)

	if err := m.db.WithContext(ctx).Table(m.tables.Branches).
		Where("id = ?", branchID).Update("deleted", true).Error; err != nil {
		return fmt.Errorf("DeleteBranch: %w", err)
	}
	return nil
}

// MergeBranch merges the given branch into the repository's default branch by
// forwarding the default branch's HEAD commit pointer to the source branch's
// HEAD commit. Returns the updated default [models.Branch].
//
// The entire advance-head operation runs inside a [RefLocker] lock
// so that two concurrent MergeBranch calls cannot produce a lost update. The
// CAS guard in [advanceBranchHead] provides a second layer
// of protection: if the default branch HEAD changed between the read and the
// write, [ErrMergeConcurrencyConflict] is returned and the caller may retry.
//
// Returns [ErrBranchNotFound] if no branch with that ID exists.
// Returns [ErrRepoNotInitialised] if no repository entity exists.
// Returns [ErrMergeConcurrencyConflict] if a concurrent merge advanced the
// default branch HEAD before this one could complete.
func (m *gitManager) MergeBranch(ctx context.Context, branchID string) (models.Branch, error) {
	sourceBranch, err := m.GetBranch(ctx, branchID)
	if err != nil {
		return models.Branch{}, fmt.Errorf("MergeBranch: %w", err)
	}
	repo, err := m.GetRepository(ctx, sourceBranch.RepositoryID)
	if err != nil {
		return models.Branch{}, fmt.Errorf("MergeBranch: %w", err)
	}
	defaultBranchEntity, err := m.defaultBranch(ctx, repo.ID)
	if err != nil {
		return models.Branch{}, fmt.Errorf("MergeBranch: default branch: %w", err)
	}
	if sourceBranch.HeadCommitID == "" {
		return defaultBranchEntity, nil
	}

	var updated models.Branch
	lockErr := m.locker.WithMergeLock(ctx, func() error {
		// Re-read the default branch inside the lock so we hold the freshest
		// HeadCommitID as the CAS guard. This ensures two sequential merges
		// both succeed; the CAS only fires if the row was modified by an
		// out-of-band write (e.g. another service instance).
		currentDefault, readErr := m.defaultBranch(ctx, repo.ID)
		if readErr != nil {
			return fmt.Errorf("re-read default branch: %w", readErr)
		}
		var advErr error
		updated, advErr = m.advanceBranchHead(ctx, currentDefault.ID, sourceBranch.HeadCommitID, currentDefault.HeadCommitID)
		return advErr
	})
	if lockErr != nil {
		return models.Branch{}, fmt.Errorf("MergeBranch: advance default head: %w", lockErr)
	}

	// GIT-022a: Replicate branch-scoped documentation edges (tagged_with,
	// references) from source branch blobs to the default branch.
	m.replicateDocEdges(ctx, sourceBranch.ID, defaultBranchEntity.ID, sourceBranch.HeadCommitID)

	// FEAT-20260602-001: emit TopicBranchMerged so the closure aggregator can
	// observe the merge. workflow_run_id is read off the source branch — the
	// originating WorkflowRun is the run that produced the branch's commits.
	m.publish(ctx, TopicBranchMerged, BranchMergedPayload{
		BranchID:      sourceBranch.ID,
		RepoID:        repo.ID,
		WorkflowRunID: sourceBranch.WorkflowRunID,
	})

	return updated, nil
}

// ListBranchesFiltered returns Branch rows for the specified repository
// filtered by [BranchFilter]. When filter.WorkflowRunID is non-empty only
// branches with the matching workflow_run_id column are returned.
// When repoID is empty and filter.WorkflowRunID is set the query is
// cross-repository (used by mwanachama-backend-taskmanager's WorkflowRun closure
// aggregator).
func (m *gitManager) ListBranchesFiltered(ctx context.Context, repoID string, filter BranchFilter) ([]models.Branch, error) {
	if repoID == "" && filter.WorkflowRunID != "" {
		return m.listBranchesByWorkflowRunID(ctx, filter.WorkflowRunID)
	}
	branches, err := m.ListBranches(ctx, repoID)
	if err != nil {
		return nil, err
	}
	if filter.WorkflowRunID == "" {
		return branches, nil
	}
	out := branches[:0]
	for _, b := range branches {
		if b.WorkflowRunID == filter.WorkflowRunID {
			out = append(out, b)
		}
	}
	return out, nil
}

// listBranchesByWorkflowRunID returns all Branch rows across all
// repositories whose workflow_run_id column matches runID. Used by the
// closure aggregator path where no repository is specified.
func (m *gitManager) listBranchesByWorkflowRunID(ctx context.Context, runID string) ([]models.Branch, error) {
	var rows []gormstore.BranchRow
	if err := m.db.WithContext(ctx).Table(m.tables.Branches).
		Where("workflow_run_id = ? AND NOT deleted", runID).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]models.Branch, len(rows))
	for i, r := range rows {
		out[i] = gormstore.BranchFromRow(r)
	}
	return out, nil
}

// ── Branch internal helpers ───────────────────────────────────────────────────

// listBranchesByRepo returns all Branch rows linked to the given
// repositoryID.
func (m *gitManager) listBranchesByRepo(ctx context.Context, repositoryID string) ([]gormstore.BranchRow, error) {
	var rows []gormstore.BranchRow
	err := m.db.WithContext(ctx).Table(m.tables.Branches).
		Where("repository_id = ? AND NOT deleted", repositoryID).Find(&rows).Error
	return rows, err
}

// defaultBranch returns the Branch row whose is_default column is true for
// the given repository.
func (m *gitManager) defaultBranch(ctx context.Context, repositoryID string) (models.Branch, error) {
	var row gormstore.BranchRow
	err := m.db.WithContext(ctx).Table(m.tables.Branches).
		Where("repository_id = ? AND is_default AND NOT deleted", repositoryID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Branch{}, ErrBranchNotFound
		}
		return models.Branch{}, err
	}
	return gormstore.BranchFromRow(row), nil
}

// advanceBranchHead updates a branch's head_commit_id and sha columns to
// point at newCommitID. The sha is copied from the Commit row so callers can
// read the branch tip SHA directly off the Branch row without an extra
// query. Returns the updated Branch.
//
// expectedHeadCommitID is a CAS guard: if non-empty, the update only applies
// when the branch's current head_commit_id still matches it — a single
// conditional UPDATE, strictly stronger than the old read-then-write check
// (no window between the read and the write for a concurrent writer to land
// in). Returns [ErrMergeConcurrencyConflict] if the row didn't match.
// Pass "" to skip the check (used by IndexPushedBranch).
func (m *gitManager) advanceBranchHead(ctx context.Context, branchID, newCommitID, expectedHeadCommitID string) (models.Branch, error) {
	var commitRow gormstore.CommitRow
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).
		Where("id = ?", newCommitID).First(&commitRow).Error; err != nil {
		return models.Branch{}, fmt.Errorf("advanceBranchHead: get commit: %w", err)
	}

	now := models.NowRFC3339()
	updates := map[string]any{
		"head_commit_id": newCommitID,
		"updated_at":     now,
	}
	if commitRow.SHA != "" {
		updates["sha"] = commitRow.SHA
	}

	q := m.db.WithContext(ctx).Table(m.tables.Branches).Where("id = ?", branchID)
	if expectedHeadCommitID != "" {
		q = q.Where("head_commit_id = ?", expectedHeadCommitID)
	}
	result := q.Updates(updates)
	if result.Error != nil {
		return models.Branch{}, fmt.Errorf("advanceBranchHead: update: %w", result.Error)
	}
	if expectedHeadCommitID != "" && result.RowsAffected == 0 {
		return models.Branch{}, ErrMergeConcurrencyConflict
	}

	return m.GetBranch(ctx, branchID)
}
