package mwanachamagit

import (
	"context"
	"fmt"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func (m *gitManager) nodesAtBranchHead(ctx context.Context, branch models.Branch) (map[string]bool, error) {
	allowed := map[string]bool{}
	if branch.HeadCommitID == "" {
		return allowed, nil
	}
	db := m.db.WithContext(ctx)

	var commitIDs []string
	if err := db.Table(m.tables.Commits).Where("repository_id = ?", branch.RepositoryID).Pluck("id", &commitIDs).Error; err != nil {
		return nil, fmt.Errorf("nodesAtBranchHead: commits: %w", err)
	}
	for _, id := range commitIDs {
		allowed[id] = true
	}
	allowed[branch.HeadCommitID] = true

	var head models.Commit
	if err := db.Table(m.tables.Commits).Where("id = ?", branch.HeadCommitID).Limit(1).Find(&head).Error; err != nil {
		return nil, fmt.Errorf("nodesAtBranchHead: head commit: %w", err)
	}
	frontier := []string{}
	if head.TreeID != "" {
		frontier = append(frontier, head.TreeID)
		allowed[head.TreeID] = true
	}
	for len(frontier) > 0 {
		var subtrees []string
		if err := db.Table(m.tables.TreeSubtrees).Where("tree_id IN ?", frontier).Pluck("subtree_id", &subtrees).Error; err != nil {
			return nil, fmt.Errorf("nodesAtBranchHead: subtrees: %w", err)
		}
		var next []string
		for _, id := range subtrees {
			if !allowed[id] {
				allowed[id] = true
				next = append(next, id)
			}
		}
		frontier = next
	}

	blobs, err := m.allBlobsAtCommit(ctx, branch.HeadCommitID)
	if err != nil {
		return nil, fmt.Errorf("nodesAtBranchHead: blobs: %w", err)
	}
	for _, b := range blobs {
		allowed[b.ID] = true
	}

	var keywordIDs []string
	if err := db.Table(m.tables.Keywords).Pluck("id", &keywordIDs).Error; err != nil {
		return nil, fmt.Errorf("nodesAtBranchHead: keywords: %w", err)
	}
	for _, id := range keywordIDs {
		allowed[id] = true
	}
	return allowed, nil
}
