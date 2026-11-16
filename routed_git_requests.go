package mwanachamagit

import (
	"context"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func (g *RoutedGitManager) CreateMergeRequest(ctx context.Context, req CreateMergeRequestRequest) (models.MergeRequest, error) {
	return call(g.repo(ctx, req.RepositoryID), func(f *RepoFile) (models.MergeRequest, error) {
		return f.CreateMergeRequest(ctx, req)
	})
}

func (g *RoutedGitManager) GetMergeRequest(ctx context.Context, mrID string) (models.MergeRequest, error) {
	return call(g.mergeRequest(ctx, mrID), func(f *RepoFile) (models.MergeRequest, error) {
		return f.GetMergeRequest(ctx, mrID)
	})
}

func (g *RoutedGitManager) ListMergeRequests(ctx context.Context, filter MergeRequestFilter) ([]models.MergeRequest, error) {
	if filter.RepositoryID != "" {
		return call(g.repo(ctx, filter.RepositoryID), func(f *RepoFile) ([]models.MergeRequest, error) {
			return f.ListMergeRequests(ctx, filter)
		})
	}
	out := []models.MergeRequest{}
	err := g.eachFile(ctx, func(_ RepositoryCatalogEntry, file *RepoFile) error {
		found, err := file.ListMergeRequests(ctx, filter)
		out = append(out, found...)
		return err
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, err
}

func (g *RoutedGitManager) CompleteMergeRequest(ctx context.Context, mrID, mergedBy string) (models.MergeRequest, error) {
	return call(g.mergeRequest(ctx, mrID), func(f *RepoFile) (models.MergeRequest, error) {
		return f.CompleteMergeRequest(ctx, mrID, mergedBy)
	})
}

func (g *RoutedGitManager) CloseMergeRequest(ctx context.Context, mrID string) (models.MergeRequest, error) {
	return call(g.mergeRequest(ctx, mrID), func(f *RepoFile) (models.MergeRequest, error) {
		return f.CloseMergeRequest(ctx, mrID)
	})
}

func (g *RoutedGitManager) RollbackByWorkflowRun(ctx context.Context, workflowRunID string) (RollbackResult, error) {
	if workflowRunID == "" {
		return RollbackResult{}, ErrWorkflowRunIDRequired
	}
	total := RollbackResult{WorkflowRunID: workflowRunID}
	err := g.eachFile(ctx, func(_ RepositoryCatalogEntry, file *RepoFile) error {
		part, err := file.RollbackByWorkflowRun(ctx, workflowRunID)
		total.BranchesDeleted += part.BranchesDeleted
		total.MergeRequestsRolledBack += part.MergeRequestsRolledBack
		total.DefaultBranchesSkipped += part.DefaultBranchesSkipped
		return err
	})
	return total, err
}

func (g *RoutedGitManager) CreateTag(ctx context.Context, req CreateTagRequest) (models.Tag, error) {
	return call(g.repo(ctx, req.RepositoryID), func(f *RepoFile) (models.Tag, error) {
		return f.CreateTag(ctx, req)
	})
}

func (g *RoutedGitManager) GetTag(ctx context.Context, tagID string) (models.Tag, error) {
	return call(g.tag(ctx, tagID), func(f *RepoFile) (models.Tag, error) {
		return f.GetTag(ctx, tagID)
	})
}

func (g *RoutedGitManager) ListTags(ctx context.Context, repoID string) ([]models.Tag, error) {
	return call(g.repo(ctx, repoID), func(f *RepoFile) ([]models.Tag, error) {
		return f.ListTags(ctx, repoID)
	})
}

func (g *RoutedGitManager) DeleteTag(ctx context.Context, tagID string) error {
	return g.tag(ctx, tagID)(func(f *RepoFile) error { return f.DeleteTag(ctx, tagID) })
}
