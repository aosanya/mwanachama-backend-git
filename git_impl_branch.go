// git_impl_branch.go — Branch management implementation for [gitManager].
package mwanachamagit

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

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
	row := models.Branch{
		Name:          req.Name,
		IsDefault:     false,
		HeadCommitID:  sourceBranch.HeadCommitID,
		SHA:           sourceBranch.SHA,
		WorkflowRunID: req.WorkflowRunID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	row.RepositoryID = repo.ID
	ensureID(&row)
	if err := m.db.WithContext(ctx).Table(m.tables.Branches).Create(&row).Error; err != nil {
		return models.Branch{}, fmt.Errorf("CreateBranch: create: %w", err)
	}
	return row, nil
}

// GetBranch retrieves a Branch row by its ID.
// Returns [ErrBranchNotFound] if no branch with that ID exists.
func (m *gitManager) GetBranch(ctx context.Context, branchID string) (models.Branch, error) {
	var row models.Branch
	err := m.db.WithContext(ctx).Table(m.tables.Branches).
		Where("id = ? AND NOT deleted", branchID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Branch{}, ErrBranchNotFound
		}
		return models.Branch{}, fmt.Errorf("GetBranch: %w", err)
	}
	return row, nil
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
		out[i] = r
	}
	return out, nil
}

// GetBranchByName retrieves a Branch row by its human-readable name.
// Returns [ErrBranchNotFound] if no branch with that name exists for the
// specified repository.
func (m *gitManager) GetBranchByName(ctx context.Context, repoID string, branchName string) (models.Branch, error) {
	var row models.Branch
	err := m.db.WithContext(ctx).Table(m.tables.Branches).
		Where("repository_id = ? AND name = ? AND NOT deleted", repoID, branchName).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Branch{}, ErrBranchNotFound
		}
		return models.Branch{}, fmt.Errorf("GetBranchByName: %w", err)
	}
	return row, nil
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

func (m *gitManager) MergeBranch(ctx context.Context, branchID string) (models.Branch, error) {
	if err := m.mayMergeBranch(ctx, "", branchID); err != nil {
		return models.Branch{}, fmt.Errorf("MergeBranch: %w", err)
	}
	return m.mergeBranch(ctx, branchID)
}

func (m *gitManager) mergeBranch(ctx context.Context, branchID string) (models.Branch, error) {
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
		currentDefault, readErr := m.defaultBranch(ctx, repo.ID)
		if readErr != nil {
			return fmt.Errorf("re-read default branch: %w", readErr)
		}
		var mergeErr error
		updated, mergeErr = m.mergeIntoDefault(ctx, sourceBranch, currentDefault)
		return mergeErr
	})
	if lockErr != nil {
		return models.Branch{}, fmt.Errorf("MergeBranch: advance default head: %w", lockErr)
	}

	m.replicateDocEdges(ctx, sourceBranch.ID, defaultBranchEntity.ID, sourceBranch.HeadCommitID)

	m.publish(ctx, TopicBranchMerged, BranchMergedPayload{
		BranchID:      sourceBranch.ID,
		RepoID:        repo.ID,
		WorkflowRunID: sourceBranch.WorkflowRunID,
	})

	return updated, nil
}

func (m *gitManager) mergeIntoDefault(ctx context.Context, source, expectedDefault models.Branch) (models.Branch, error) {
	if expectedDefault.HeadCommitID == source.HeadCommitID {
		return expectedDefault, nil
	}
	if expectedDefault.HeadCommitID == "" {
		return m.advanceBranchHead(ctx, expectedDefault.ID, source.HeadCommitID, expectedDefault.HeadCommitID)
	}
	base, err := m.mergeBase(ctx, expectedDefault.HeadCommitID, source.HeadCommitID)
	if err != nil {
		return models.Branch{}, fmt.Errorf("merge base: %w", err)
	}
	if base == source.HeadCommitID {
		return expectedDefault, nil
	}
	if base == expectedDefault.HeadCommitID {
		return m.advanceBranchHead(ctx, expectedDefault.ID, source.HeadCommitID, expectedDefault.HeadCommitID)
	}

	baseTree, err := m.mergeTreeAt(ctx, base)
	if err != nil {
		return models.Branch{}, err
	}
	oursTree, err := m.mergeTreeAt(ctx, expectedDefault.HeadCommitID)
	if err != nil {
		return models.Branch{}, err
	}
	theirsTree, err := m.mergeTreeAt(ctx, source.HeadCommitID)
	if err != nil {
		return models.Branch{}, err
	}
	merged, overlapping := mergeTrees(baseTree, oursTree, theirsTree)
	_ = overlapping

	message := fmt.Sprintf("Merge branch '%s' into %s", source.Name, expectedDefault.Name)
	mergeCommit, err := m.writeMergeCommit(ctx, source.RepositoryID, merged,
		[]string{expectedDefault.HeadCommitID, source.HeadCommitID}, message)
	if err != nil {
		return models.Branch{}, err
	}
	return m.advanceBranchHead(ctx, expectedDefault.ID, mergeCommit.ID, expectedDefault.HeadCommitID)
}

func (m *gitManager) mergeTreeAt(ctx context.Context, commitID string) (mergeTree, error) {
	tree := mergeTree{}
	if commitID == "" {
		return tree, nil
	}
	blobs, err := m.allBlobsAtCommit(ctx, commitID)
	if err != nil {
		return nil, err
	}
	for _, b := range blobs {
		if b.Path != "" && b.SHA != "" {
			tree[b.Path] = mergeEntry{BlobID: b.ID, SHA: b.SHA}
		}
	}
	return tree, nil
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
	var rows []models.Branch
	if err := m.db.WithContext(ctx).Table(m.tables.Branches).
		Where("workflow_run_id = ? AND NOT deleted", runID).Limit(maxListPage).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]models.Branch, len(rows))
	for i, r := range rows {
		out[i] = r
	}
	return out, nil
}

// ── Branch internal helpers ───────────────────────────────────────────────────

// listBranchesByRepo returns all Branch rows linked to the given
// repositoryID.
func (m *gitManager) listBranchesByRepo(ctx context.Context, repositoryID string) ([]models.Branch, error) {
	var rows []models.Branch
	err := m.db.WithContext(ctx).Table(m.tables.Branches).
		Where("repository_id = ? AND NOT deleted", repositoryID).Limit(maxListPage).Find(&rows).Error
	return rows, err
}

// defaultBranch returns the Branch row whose is_default column is true for
// the given repository.
func (m *gitManager) defaultBranch(ctx context.Context, repositoryID string) (models.Branch, error) {
	var row models.Branch
	err := m.db.WithContext(ctx).Table(m.tables.Branches).
		Where("repository_id = ? AND is_default AND NOT deleted", repositoryID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Branch{}, ErrBranchNotFound
		}
		return models.Branch{}, err
	}
	return row, nil
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
	var commitRow models.Commit
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

	branch, err := m.GetBranch(ctx, branchID)
	if err != nil {
		return models.Branch{}, err
	}
	m.recordCommitInCatalog(ctx, branch.RepositoryID, commitRow)
	return branch, nil
}
