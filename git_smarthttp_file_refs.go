package mwanachamagit

import (
	"context"

	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func (m *gitManager) defaultRepositoryRow(ctx context.Context) (models.Repository, error) {
	var repoRow models.Repository
	err := m.db.WithContext(ctx).Table(m.tables.Repositories).
		Where("NOT deleted").Order("created_at").First(&repoRow).Error
	if err != nil {
		return repoRow, ErrRepoNotInitialised
	}
	return repoRow, nil
}

func (m *gitManager) snapshotRefs(ctx context.Context) (*fileSnapshot, error) {
	repoRow, err := m.defaultRepositoryRow(ctx)
	if err != nil {
		return nil, err
	}
	snap := &fileSnapshot{storage: memory.NewStorage(), repo: repoRow, objectRow: m.objectRowLookup(ctx)}
	if err := m.loadRefs(ctx, snap); err != nil {
		return nil, err
	}
	return snap, nil
}

func (m *gitManager) objectRowLookup(ctx context.Context) func(sha string) bool {
	tables := []struct{ name, present string }{
		{m.tables.Commits, "data <> ''"},
		{m.tables.Trees, "data <> ''"},
		{m.tables.Blobs, "data <> '' OR size = 0"},
	}
	return func(sha string) bool {
		for _, table := range tables {
			var found []string
			err := m.db.WithContext(ctx).Table(table.name).
				Where("sha = ? AND ("+table.present+")", sha).Limit(1).Pluck("sha", &found).Error
			if err == nil && len(found) > 0 {
				return true
			}
		}
		return false
	}
}
