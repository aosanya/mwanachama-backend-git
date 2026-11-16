package mwanachamagit

import (
	"context"
	"errors"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func (g *RoutedGitManager) Diff(ctx context.Context, fromRef, toRef string) ([]FileDiff, error) {
	var out []FileDiff
	err := g.locate(ctx, "", ErrRefNotFound, func(file *RepoFile) (bool, error) {
		diffs, err := file.Diff(ctx, fromRef, toRef)
		if errors.Is(err, ErrRefNotFound) {
			return false, nil
		}
		out = diffs
		return err == nil, err
	})
	return out, err
}

func (g *RoutedGitManager) ImportRepo(ctx context.Context, req ImportRepoRequest) (models.ImportJob, error) {
	return models.ImportJob{}, ErrImportNeedsRepositoryFile
}

func (g *RoutedGitManager) GetImportStatus(ctx context.Context, jobID string) (models.ImportJob, error) {
	return call(g.importJob(ctx, jobID), func(f *RepoFile) (models.ImportJob, error) {
		return f.GetImportStatus(ctx, jobID)
	})
}

func (g *RoutedGitManager) CancelImport(ctx context.Context, jobID string) error {
	return g.importJob(ctx, jobID)(func(f *RepoFile) error { return f.CancelImport(ctx, jobID) })
}

func (g *RoutedGitManager) CreateKeyword(ctx context.Context, req CreateKeywordRequest) (models.Keyword, error) {
	scope := g.first(ctx)
	if req.ParentID != "" {
		scope = g.keyword(ctx, req.ParentID)
	}
	return call(scope, func(f *RepoFile) (models.Keyword, error) {
		return f.CreateKeyword(ctx, req)
	})
}

func (g *RoutedGitManager) GetKeyword(ctx context.Context, keywordID string) (models.Keyword, error) {
	return call(g.keyword(ctx, keywordID), func(f *RepoFile) (models.Keyword, error) {
		return f.GetKeyword(ctx, keywordID)
	})
}

func (g *RoutedGitManager) ListKeywords(ctx context.Context, filter KeywordFilter) ([]models.Keyword, error) {
	if filter.ParentID != "" {
		return call(g.keyword(ctx, filter.ParentID), func(f *RepoFile) ([]models.Keyword, error) {
			return f.ListKeywords(ctx, filter)
		})
	}
	out := []models.Keyword{}
	err := g.eachFile(ctx, func(_ RepositoryCatalogEntry, file *RepoFile) error {
		found, err := file.ListKeywords(ctx, filter)
		out = append(out, found...)
		return err
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, err
}

func (g *RoutedGitManager) GetKeywordTree(ctx context.Context, keywordID string) ([]KeywordTreeNode, error) {
	return call(g.keyword(ctx, keywordID), func(f *RepoFile) ([]KeywordTreeNode, error) {
		return f.GetKeywordTree(ctx, keywordID)
	})
}

func (g *RoutedGitManager) UpdateKeyword(ctx context.Context, keywordID string, req UpdateKeywordRequest) (models.Keyword, error) {
	return call(g.keyword(ctx, keywordID), func(f *RepoFile) (models.Keyword, error) {
		return f.UpdateKeyword(ctx, keywordID, req)
	})
}

func (g *RoutedGitManager) DeleteKeyword(ctx context.Context, keywordID string) error {
	return g.keyword(ctx, keywordID)(func(f *RepoFile) error { return f.DeleteKeyword(ctx, keywordID) })
}

func (g *RoutedGitManager) CreateEdge(ctx context.Context, req CreateEdgeRequest) error {
	return g.branch(ctx, req.BranchID)(func(f *RepoFile) error { return f.CreateEdge(ctx, req) })
}

func (g *RoutedGitManager) DeleteEdge(ctx context.Context, req DeleteEdgeRequest) error {
	return g.branch(ctx, req.BranchID)(func(f *RepoFile) error { return f.DeleteEdge(ctx, req) })
}

func (g *RoutedGitManager) GetNeighborhood(ctx context.Context, branchID, entityID string, depth int) (GraphResult, error) {
	return call(g.branch(ctx, branchID), func(f *RepoFile) (GraphResult, error) {
		return f.GetNeighborhood(ctx, branchID, entityID, depth)
	})
}

func (g *RoutedGitManager) SearchByKeywords(ctx context.Context, req SearchByKeywordsRequest) (GraphResult, error) {
	return call(g.branch(ctx, req.BranchID), func(f *RepoFile) (GraphResult, error) {
		return f.SearchByKeywords(ctx, req)
	})
}

func (g *RoutedGitManager) QueryGraph(ctx context.Context, req QueryGraphRequest) (GraphResult, error) {
	return call(g.branch(ctx, req.BranchID), func(f *RepoFile) (GraphResult, error) {
		return f.QueryGraph(ctx, req)
	})
}

func (g *RoutedGitManager) FetchBranch(ctx context.Context, req FetchBranchRequest) (models.FetchBranchJob, error) {
	return call(g.repo(ctx, req.RepoID), func(f *RepoFile) (models.FetchBranchJob, error) {
		return f.FetchBranch(ctx, req)
	})
}

func (g *RoutedGitManager) GetFetchBranchStatus(ctx context.Context, jobID string) (models.FetchBranchJob, error) {
	return call(g.fetchJob(ctx, jobID), func(f *RepoFile) (models.FetchBranchJob, error) {
		return f.GetFetchBranchStatus(ctx, jobID)
	})
}

func (g *RoutedGitManager) IndexPushedBranch(ctx context.Context, repoName, branchRef, oldSHA, newSHA string) error {
	return g.named(ctx, repoName)(func(f *RepoFile) error {
		return f.IndexPushedBranch(ctx, repoName, branchRef, oldSHA, newSHA)
	})
}

func (g *RoutedGitManager) SearchBlobs(ctx context.Context, req SearchBlobsRequest) ([]BlobSearchResult, error) {
	return call(g.named(ctx, req.RepositoryName), func(f *RepoFile) ([]BlobSearchResult, error) {
		return f.SearchBlobs(ctx, req)
	})
}
