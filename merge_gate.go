package mwanachamagit

import (
	"context"
	"fmt"

	"github.com/aosanya/mwanachama-backend-git/models"
)

type MergeGate func(ctx context.Context, caller, repositoryID, targetBranchID string) (bool, error)

func WithMergeGate(gate MergeGate) Option {
	return func(m *gitManager) { m.merging = gate }
}

func (m *gitManager) mayMergeBranch(ctx context.Context, caller, branchID string) error {
	if m.merging == nil {
		return nil
	}
	source, err := m.GetBranch(ctx, branchID)
	if err != nil {
		return err
	}
	target, err := m.defaultBranch(ctx, source.RepositoryID)
	if err != nil {
		return err
	}
	return m.mayMerge(ctx, caller, source.RepositoryID, target.ID)
}

func (m *gitManager) mayMerge(ctx context.Context, caller, repositoryID, targetBranchID string) error {
	if m.merging == nil {
		return nil
	}
	allowed, err := m.merging(ctx, caller, repositoryID, targetBranchID)
	if err != nil {
		return fmt.Errorf("merge gate: %w", err)
	}
	if !allowed {
		return ErrMergeNotPermitted
	}
	return nil
}

func (m *gitManager) recordMergedBy(ctx context.Context, mrID, mergedBy string) (models.MergeRequest, error) {
	if err := m.db.WithContext(ctx).Table(m.tables.MergeRequests).
		Where("id = ?", mrID).Updates(map[string]any{"merged_by": mergedBy}).Error; err != nil {
		return models.MergeRequest{}, err
	}
	return m.GetMergeRequest(ctx, mrID)
}
