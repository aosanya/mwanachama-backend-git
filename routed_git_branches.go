package mwanachamagit

import (
	"context"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func (g *RoutedGitManager) GetBranchByName(ctx context.Context, repoID string, branchName string) (models.Branch, error) {
	return call(g.repo(ctx, repoID), func(f *RepoFile) (models.Branch, error) {
		return f.GetBranchByName(ctx, repoID, branchName)
	})
}

func (g *RoutedGitManager) CreateBranch(ctx context.Context, req CreateBranchRequest) (models.Branch, error) {
	return call(g.repo(ctx, req.RepositoryID), func(f *RepoFile) (models.Branch, error) {
		return f.CreateBranch(ctx, req)
	})
}

func (g *RoutedGitManager) GetBranch(ctx context.Context, branchID string) (models.Branch, error) {
	return call(g.branch(ctx, branchID), func(f *RepoFile) (models.Branch, error) {
		return f.GetBranch(ctx, branchID)
	})
}

func (g *RoutedGitManager) ListBranches(ctx context.Context, repoID string) ([]models.Branch, error) {
	return call(g.repo(ctx, repoID), func(f *RepoFile) ([]models.Branch, error) {
		return f.ListBranches(ctx, repoID)
	})
}

func (g *RoutedGitManager) ListBranchesFiltered(ctx context.Context, repoID string, filter BranchFilter) ([]models.Branch, error) {
	return call(g.repo(ctx, repoID), func(f *RepoFile) ([]models.Branch, error) {
		return f.ListBranchesFiltered(ctx, repoID, filter)
	})
}

func (g *RoutedGitManager) DeleteBranch(ctx context.Context, branchID string) error {
	return g.branch(ctx, branchID)(func(f *RepoFile) error { return f.DeleteBranch(ctx, branchID) })
}

func (g *RoutedGitManager) MergeBranch(ctx context.Context, branchID string) (models.Branch, error) {
	return call(g.branch(ctx, branchID), func(f *RepoFile) (models.Branch, error) {
		return f.MergeBranch(ctx, branchID)
	})
}

func (g *RoutedGitManager) RebaseBranch(ctx context.Context, req RebaseBranchRequest) (models.Branch, error) {
	return call(g.branch(ctx, req.BranchID), func(f *RepoFile) (models.Branch, error) {
		return f.RebaseBranch(ctx, req)
	})
}

func (g *RoutedGitManager) WriteFile(ctx context.Context, req WriteFileRequest) (models.Commit, error) {
	return call(g.branch(ctx, req.BranchID), func(f *RepoFile) (models.Commit, error) {
		return f.WriteFile(ctx, req)
	})
}

func (g *RoutedGitManager) ReadFile(ctx context.Context, branchID, path string) (models.Blob, error) {
	return call(g.branch(ctx, branchID), func(f *RepoFile) (models.Blob, error) {
		return f.ReadFile(ctx, branchID, path)
	})
}

func (g *RoutedGitManager) DeleteFile(ctx context.Context, req DeleteFileRequest) (models.Commit, error) {
	return call(g.branch(ctx, req.BranchID), func(f *RepoFile) (models.Commit, error) {
		return f.DeleteFile(ctx, req)
	})
}

func (g *RoutedGitManager) ListDirectory(ctx context.Context, branchID, path string) ([]FileEntry, error) {
	return call(g.branch(ctx, branchID), func(f *RepoFile) ([]FileEntry, error) {
		return f.ListDirectory(ctx, branchID, path)
	})
}

func (g *RoutedGitManager) Log(ctx context.Context, branchID string, filter LogFilter) ([]CommitEntry, error) {
	return call(g.branch(ctx, branchID), func(f *RepoFile) ([]CommitEntry, error) {
		return f.Log(ctx, branchID, filter)
	})
}
