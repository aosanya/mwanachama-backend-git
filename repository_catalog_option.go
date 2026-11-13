package mwanachamagit

import (
	"context"
	"errors"

	"github.com/aosanya/mwanachama-backend-git/models"
)

type RepositoryFile func(repoID string) (fileName string, sizeBytes int64, err error)

func WithRepositoryCatalog(catalog RepositoryCatalog, file RepositoryFile) Option {
	return func(m *gitManager) {
		m.catalog = catalog
		m.repositoryFile = file
	}
}

func (m *gitManager) registerInCatalog(ctx context.Context, repo models.Repository) error {
	if m.catalog == nil || m.repositoryFile == nil {
		return nil
	}
	fileName, sizeBytes, err := m.repositoryFile(repo.ID)
	if err != nil {
		return err
	}
	return m.catalog.Register(ctx, RepositoryCatalogEntry{
		ID:        repo.ID,
		Name:      repo.Name,
		FileName:  fileName,
		SizeBytes: sizeBytes,
	})
}

func (m *gitManager) recordCommitInCatalog(ctx context.Context, repoID string, commit models.Commit) {
	if m.catalog == nil || m.repositoryFile == nil {
		return
	}
	_, sizeBytes, err := m.repositoryFile(repoID)
	if err != nil {
		return
	}
	lastCommitAt := commit.CommittedAt
	if lastCommitAt == "" {
		lastCommitAt = commit.CreatedAt
	}
	_ = m.catalog.Update(ctx, repoID, sizeBytes, lastCommitAt)
}

func (m *gitManager) removeFromCatalog(ctx context.Context, repoID string) error {
	if m.catalog == nil {
		return nil
	}
	err := m.catalog.Remove(ctx, repoID)
	if errors.Is(err, ErrRepositoryNotCatalogued) {
		return nil
	}
	return err
}
