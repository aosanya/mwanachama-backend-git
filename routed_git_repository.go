package mwanachamagit

import (
	"context"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func (g *RoutedGitManager) InitRepo(ctx context.Context, req CreateRepoRequest) (models.Repository, error) {
	return g.folder.Create(ctx, req)
}

func (g *RoutedGitManager) DeleteRepo(ctx context.Context, repoID string) error {
	defer g.owners.clear()
	return g.folder.Delete(ctx, repoID)
}

func (g *RoutedGitManager) PurgeRepo(ctx context.Context, repoID string) error {
	defer g.owners.clear()
	return g.folder.Purge(ctx, repoID)
}

func (g *RoutedGitManager) ListRepositories(ctx context.Context) ([]models.Repository, error) {
	out := []models.Repository{}
	err := g.eachFile(ctx, func(entry RepositoryCatalogEntry, file *RepoFile) error {
		out = append(out, g.repositoryOf(ctx, entry, file))
		return nil
	})
	return out, err
}

func (g *RoutedGitManager) repositoryOf(ctx context.Context, entry RepositoryCatalogEntry, file *RepoFile) models.Repository {
	repo, err := file.GetRepository(ctx, entry.ID)
	if err != nil {
		repo = models.Repository{}
	}
	repo.ID = entry.ID
	repo.Name = entry.Name
	if entry.LastCommitAt != "" {
		repo.UpdatedAt = entry.LastCommitAt
	}
	return repo
}

func (g *RoutedGitManager) ListRepositorySummaries(ctx context.Context) ([]models.RepositorySummary, error) {
	out := []models.RepositorySummary{}
	err := g.eachFile(ctx, func(entry RepositoryCatalogEntry, file *RepoFile) error {
		repo := g.repositoryOf(ctx, entry, file)
		summary := models.RepositorySummary{ID: repo.ID, Name: repo.Name, DefaultBranch: repo.DefaultBranch, LastCommitAt: entry.LastCommitAt}
		if branch, err := file.GetBranchByName(ctx, repo.ID, repo.DefaultBranch); err == nil {
			summary.DefaultBranchID = branch.ID
		}
		out = append(out, summary)
		return nil
	})
	return out, err
}

func (g *RoutedGitManager) GetRepository(ctx context.Context, repoID string) (models.Repository, error) {
	return call(g.repo(ctx, repoID), func(f *RepoFile) (models.Repository, error) {
		return f.GetRepository(ctx, repoID)
	})
}

func (g *RoutedGitManager) GetRepositoryByName(ctx context.Context, repoName string) (models.Repository, error) {
	return call(g.named(ctx, repoName), func(f *RepoFile) (models.Repository, error) {
		return f.GetRepositoryByName(ctx, repoName)
	})
}
