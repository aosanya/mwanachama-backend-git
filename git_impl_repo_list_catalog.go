package mwanachamagit

import (
	"context"
	"fmt"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func (m *gitManager) listCataloguedRepositories(ctx context.Context) ([]models.Repository, error) {
	entries, err := m.catalog.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("ListRepositories: list catalog: %w", err)
	}
	rows, err := m.repositoryRowsByID(ctx, entries)
	if err != nil {
		return nil, err
	}
	out := make([]models.Repository, len(entries))
	for i, entry := range entries {
		out[i] = repositoryFromEntry(entry, rows[entry.ID])
	}
	return out, nil
}

func (m *gitManager) repositoryRowsByID(ctx context.Context, entries []RepositoryCatalogEntry) (map[string]models.Repository, error) {
	byID := make(map[string]models.Repository, len(entries))
	if len(entries) == 0 {
		return byID, nil
	}
	ids := make([]string, len(entries))
	for i, entry := range entries {
		ids[i] = entry.ID
	}
	var rows []models.Repository
	if err := m.db.WithContext(ctx).Table(m.tables.Repositories).
		Where("id IN ? AND NOT deleted", ids).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("ListRepositories: repository rows: %w", err)
	}
	for _, row := range rows {
		byID[row.ID] = row
	}
	return byID, nil
}

func repositoryFromEntry(entry RepositoryCatalogEntry, row models.Repository) models.Repository {
	row.ID = entry.ID
	row.Name = entry.Name
	if entry.LastCommitAt != "" {
		row.UpdatedAt = entry.LastCommitAt
	}
	return row
}
