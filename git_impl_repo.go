// git_impl_repo.go — Repository lifecycle implementation for [gitManager].
//
// Branch management is in git_impl_branch.go.
// Tag management is in git_impl_tag.go.
package mwanachamagit

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-git/gormstore"
	"github.com/aosanya/mwanachama-backend-git/models"
)

// ── Repository Lifecycle ──────────────────────────────────────────────────────

// InitRepo creates a new Repository row.
// Returns [ErrRepoAlreadyExists] if a repository with the same name already exists.
// Publishes [TopicRepoCreated] after a successful write.
func (m *gitManager) InitRepo(ctx context.Context, req CreateRepoRequest) (models.Repository, error) {
	var count int64
	if err := m.db.WithContext(ctx).Table(m.tables.Repositories).
		Where("name = ? AND NOT deleted", req.Name).Count(&count).Error; err != nil {
		return models.Repository{}, fmt.Errorf("InitRepo: check existing: %w", err)
	}
	if count > 0 {
		return models.Repository{}, ErrRepoAlreadyExists
	}

	agencyID, err := m.ensureAgencyEntity(ctx)
	if err != nil {
		return models.Repository{}, fmt.Errorf("InitRepo: ensure agency: %w", err)
	}

	defaultBranch := req.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	now := models.NowRFC3339()

	row := gormstore.RepositoryToRow(models.Repository{
		Name:          req.Name,
		Description:   req.Description,
		DefaultBranch: defaultBranch,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	row.AgencyID = gormstore.StringToNullable(agencyID)
	if err := m.db.WithContext(ctx).Table(m.tables.Repositories).Create(&row).Error; err != nil {
		return models.Repository{}, fmt.Errorf("InitRepo: create repository: %w", err)
	}

	branchRow := gormstore.BranchToRow(models.Branch{
		Name:      defaultBranch,
		IsDefault: true,
		CreatedAt: now,
		UpdatedAt: now,
	})
	branchRow.RepositoryID = gormstore.StringToNullable(row.ID)
	if err := m.db.WithContext(ctx).Table(m.tables.Branches).Create(&branchRow).Error; err != nil {
		return models.Repository{}, fmt.Errorf("InitRepo: create default branch: %w", err)
	}

	repo := gormstore.RepositoryFromRow(row)
	m.publish(ctx, TopicRepoCreated, RepoCreatedPayload{RepoID: repo.ID, Name: req.Name})
	return repo, nil
}

// ListRepositories returns all Repository rows.
func (m *gitManager) ListRepositories(ctx context.Context) ([]models.Repository, error) {
	var rows []gormstore.RepositoryRow
	if err := m.db.WithContext(ctx).Table(m.tables.Repositories).
		Where("NOT deleted").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("ListRepositories: %w", err)
	}
	out := make([]models.Repository, len(rows))
	for i, r := range rows {
		out[i] = gormstore.RepositoryFromRow(r)
	}
	return out, nil
}

// GetRepository retrieves a Repository row by its ID.
// Returns [ErrRepoNotInitialised] if no repository with that ID exists.
func (m *gitManager) GetRepository(ctx context.Context, repoID string) (models.Repository, error) {
	var row gormstore.RepositoryRow
	err := m.db.WithContext(ctx).Table(m.tables.Repositories).
		Where("id = ? AND NOT deleted", repoID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Repository{}, ErrRepoNotInitialised
		}
		return models.Repository{}, fmt.Errorf("GetRepository: %w", err)
	}
	return gormstore.RepositoryFromRow(row), nil
}

// GetRepositoryByName retrieves a Repository row by its human-readable name.
// Returns [ErrRepoNotInitialised] if no repository with that name exists.
func (m *gitManager) GetRepositoryByName(ctx context.Context, repoName string) (models.Repository, error) {
	var row gormstore.RepositoryRow
	err := m.db.WithContext(ctx).Table(m.tables.Repositories).
		Where("name = ? AND NOT deleted", repoName).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Repository{}, ErrRepoNotInitialised
		}
		return models.Repository{}, fmt.Errorf("GetRepositoryByName: %w", err)
	}
	return gormstore.RepositoryFromRow(row), nil
}

// DeleteRepo soft-deletes the specified repository row and all owned sub-rows.
// Returns [ErrRepoNotInitialised] if no repository with that ID exists.
func (m *gitManager) DeleteRepo(ctx context.Context, repoID string) error {
	repo, err := m.GetRepository(ctx, repoID)
	if err != nil {
		return fmt.Errorf("DeleteRepo: %w", err)
	}

	if err := m.db.WithContext(ctx).Table(m.tables.Branches).
		Where("repository_id = ?", repo.ID).Update("deleted", true).Error; err != nil {
		return fmt.Errorf("DeleteRepo: delete branches: %w", err)
	}
	if err := m.db.WithContext(ctx).Table(m.tables.Tags).
		Where("repository_id = ?", repo.ID).Update("deleted", true).Error; err != nil {
		return fmt.Errorf("DeleteRepo: delete tags: %w", err)
	}
	if err := m.db.WithContext(ctx).Table(m.tables.Repositories).
		Where("id = ?", repo.ID).Update("deleted", true).Error; err != nil {
		return fmt.Errorf("DeleteRepo: delete repo: %w", err)
	}
	return nil
}

// PurgeRepo is a no-op alias for DeleteRepo — soft deletion is the only
// supported deletion strategy.
// Returns [ErrRepoNotInitialised] if no repository with that ID exists.
func (m *gitManager) PurgeRepo(ctx context.Context, repoID string) error {
	return m.DeleteRepo(ctx, repoID)
}

// ── Repository internal helpers ───────────────────────────────────────────────

// ensureAgencyEntity returns the ID of the single Agency root row for this
// deployment, creating it if it does not yet exist. Each deployment is
// single-tenant, so there is at most one Agency row in the database.
func (m *gitManager) ensureAgencyEntity(ctx context.Context) (string, error) {
	var row gormstore.AgencyRow
	err := m.db.WithContext(ctx).Table(m.tables.Agencies).First(&row).Error
	if err == nil {
		return row.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	now := models.NowRFC3339()
	row = gormstore.AgencyToRow(models.Agency{Name: "default", CreatedAt: now, UpdatedAt: now})
	if err := m.db.WithContext(ctx).Table(m.tables.Agencies).Create(&row).Error; err != nil {
		return "", err
	}
	return row.ID, nil
}
