// git_impl_tag.go — Tag management implementation for [gitManager].
package mwanachamagit

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-git/gormstore"
	"github.com/aosanya/mwanachama-backend-git/models"
)

// ── Tag Management ────────────────────────────────────────────────────────────

// CreateTag creates an immutable Tag row pointing to the specified commit.
// Returns [ErrTagAlreadyExists] if a tag with the given name already exists.
// Returns [ErrBranchNotFound] if req.CommitID does not resolve to a Commit row.
func (m *gitManager) CreateTag(ctx context.Context, req CreateTagRequest) (models.Tag, error) {
	repo, err := m.GetRepository(ctx, req.RepositoryID)
	if err != nil {
		return models.Tag{}, fmt.Errorf("CreateTag: %w", err)
	}

	var count int64
	if err := m.db.WithContext(ctx).Table(m.tables.Tags).
		Where("repository_id = ? AND name = ? AND NOT deleted", repo.ID, req.Name).
		Count(&count).Error; err != nil {
		return models.Tag{}, fmt.Errorf("CreateTag: check existing: %w", err)
	}
	if count > 0 {
		return models.Tag{}, ErrTagAlreadyExists
	}

	var commitRow gormstore.CommitRow
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).
		Where("id = ?", req.CommitID).First(&commitRow).Error; err != nil {
		return models.Tag{}, fmt.Errorf("CreateTag: commit %s: %w", req.CommitID, ErrBranchNotFound)
	}

	now := models.NowRFC3339()
	row := gormstore.TagToRow(models.Tag{
		Name:       req.Name,
		SHA:        commitRow.SHA,
		Message:    req.Message,
		TaggerName: req.TaggerName,
		TaggerAt:   now,
		CreatedAt:  now,
	}, commitRow.ID)
	row.RepositoryID = gormstore.StringToNullable(repo.ID)
	if err := m.db.WithContext(ctx).Table(m.tables.Tags).Create(&row).Error; err != nil {
		return models.Tag{}, fmt.Errorf("CreateTag: create: %w", err)
	}
	return gormstore.TagFromRow(row), nil
}

// GetTag retrieves a Tag row by its ID.
// Returns [ErrTagNotFound] if no tag with that ID exists.
func (m *gitManager) GetTag(ctx context.Context, tagID string) (models.Tag, error) {
	var row gormstore.TagRow
	err := m.db.WithContext(ctx).Table(m.tables.Tags).
		Where("id = ? AND NOT deleted", tagID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Tag{}, ErrTagNotFound
		}
		return models.Tag{}, fmt.Errorf("GetTag: %w", err)
	}
	return gormstore.TagFromRow(row), nil
}

// ListTags returns all Tag rows for the specified repository.
// Returns [ErrRepoNotInitialised] if no repository with that ID exists.
func (m *gitManager) ListTags(ctx context.Context, repoID string) ([]models.Tag, error) {
	if _, err := m.GetRepository(ctx, repoID); err != nil {
		return nil, fmt.Errorf("ListTags: %w", err)
	}
	var rows []gormstore.TagRow
	if err := m.db.WithContext(ctx).Table(m.tables.Tags).
		Where("repository_id = ? AND NOT deleted", repoID).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("ListTags: %w", err)
	}
	out := make([]models.Tag, len(rows))
	for i, r := range rows {
		out[i] = gormstore.TagFromRow(r)
	}
	return out, nil
}

// DeleteTag removes a Tag row.
// Returns [ErrTagNotFound] if no tag with that ID exists.
func (m *gitManager) DeleteTag(ctx context.Context, tagID string) error {
	if _, err := m.GetTag(ctx, tagID); err != nil {
		return err
	}
	if err := m.db.WithContext(ctx).Table(m.tables.Tags).
		Where("id = ?", tagID).Update("deleted", true).Error; err != nil {
		return fmt.Errorf("DeleteTag: %w", err)
	}
	return nil
}
