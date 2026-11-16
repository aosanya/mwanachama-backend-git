package mwanachamagit

import (
	"context"
	"errors"
)

type repoFileScope func(use func(*RepoFile) error) error

type repoFileAttempt func(file *RepoFile) (found bool, err error)

func call[T any](scope repoFileScope, use func(*RepoFile) (T, error)) (T, error) {
	var out T
	err := scope(func(file *RepoFile) error {
		var inner error
		out, inner = use(file)
		return inner
	})
	return out, err
}

func (g *RoutedGitManager) repo(ctx context.Context, repoID string) repoFileScope {
	return func(use func(*RepoFile) error) error {
		entry, err := g.catalog.Lookup(ctx, repoID)
		if errors.Is(err, ErrRepositoryNotCatalogued) {
			return ErrRepoNotInitialised
		}
		if err != nil {
			return err
		}
		return g.files.With(ctx, entry.FileName, use)
	}
}

func (g *RoutedGitManager) named(ctx context.Context, name string) repoFileScope {
	return func(use func(*RepoFile) error) error {
		entries, err := g.catalog.List(ctx)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.Name == name {
				return g.files.With(ctx, entry.FileName, use)
			}
		}
		return ErrRepoNotInitialised
	}
}

func (g *RoutedGitManager) first(ctx context.Context) repoFileScope {
	return func(use func(*RepoFile) error) error {
		entries, err := g.catalog.List(ctx)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			return ErrRepoNotInitialised
		}
		return g.files.With(ctx, entries[0].FileName, use)
	}
}

func (g *RoutedGitManager) eachFile(ctx context.Context, use func(entry RepositoryCatalogEntry, file *RepoFile) error) error {
	entries, err := g.catalog.List(ctx)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		err := g.files.With(ctx, entry.FileName, func(file *RepoFile) error { return use(entry, file) })
		if errors.Is(err, ErrRepositoryNotRegistered) {
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (g *RoutedGitManager) attemptIn(ctx context.Context, fileName string, try repoFileAttempt) (bool, error) {
	var found bool
	err := g.files.With(ctx, fileName, func(file *RepoFile) error {
		var inner error
		found, inner = try(file)
		return inner
	})
	return found, err
}

func (g *RoutedGitManager) locate(ctx context.Context, key string, missing error, try repoFileAttempt) error {
	if fileName, ok := g.owners.get(key); ok {
		found, err := g.attemptIn(ctx, fileName, try)
		if found || (err != nil && !errors.Is(err, ErrRepositoryNotRegistered)) {
			return err
		}
		g.owners.forget(key)
	}
	entries, err := g.catalog.List(ctx)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		found, err := g.attemptIn(ctx, entry.FileName, try)
		if errors.Is(err, ErrRepositoryNotRegistered) {
			continue
		}
		if found && key != "" {
			g.owners.remember(key, entry.FileName)
		}
		if found || err != nil {
			return err
		}
	}
	return missing
}

type ownerProbe func(ctx context.Context, file *RepoFile, id string) error

func (g *RoutedGitManager) owning(ctx context.Context, kind, id string, missing error, probe ownerProbe) repoFileScope {
	return func(use func(*RepoFile) error) error {
		return g.locate(ctx, kind+":"+id, missing, func(file *RepoFile) (bool, error) {
			if err := probe(ctx, file, id); err != nil {
				if errors.Is(err, missing) {
					return false, nil
				}
				return false, err
			}
			return true, use(file)
		})
	}
}

func (g *RoutedGitManager) branch(ctx context.Context, id string) repoFileScope {
	return g.owning(ctx, "branch", id, ErrBranchNotFound, func(ctx context.Context, f *RepoFile, id string) error {
		_, err := f.GetBranch(ctx, id)
		return err
	})
}

func (g *RoutedGitManager) mergeRequest(ctx context.Context, id string) repoFileScope {
	return g.owning(ctx, "merge-request", id, ErrMergeRequestNotFound, func(ctx context.Context, f *RepoFile, id string) error {
		_, err := f.GetMergeRequest(ctx, id)
		return err
	})
}

func (g *RoutedGitManager) tag(ctx context.Context, id string) repoFileScope {
	return g.owning(ctx, "tag", id, ErrTagNotFound, func(ctx context.Context, f *RepoFile, id string) error {
		_, err := f.GetTag(ctx, id)
		return err
	})
}

func (g *RoutedGitManager) keyword(ctx context.Context, id string) repoFileScope {
	return g.owning(ctx, "keyword", id, ErrKeywordNotFound, func(ctx context.Context, f *RepoFile, id string) error {
		_, err := f.GetKeyword(ctx, id)
		return err
	})
}

func (g *RoutedGitManager) importJob(ctx context.Context, id string) repoFileScope {
	return g.owning(ctx, "import", id, ErrImportJobNotFound, func(ctx context.Context, f *RepoFile, id string) error {
		_, err := f.GetImportStatus(ctx, id)
		return err
	})
}

func (g *RoutedGitManager) fetchJob(ctx context.Context, id string) repoFileScope {
	return g.owning(ctx, "fetch", id, ErrImportJobNotFound, func(ctx context.Context, f *RepoFile, id string) error {
		_, err := f.GetFetchBranchStatus(ctx, id)
		return err
	})
}
