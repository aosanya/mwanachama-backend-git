package mwanachamagit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/aosanya/mwanachama-backend-git/models"
)

type RepositoryFolder struct {
	mu         sync.Mutex
	folder     string
	catalog    RepositoryCatalog
	open       func(path string) (*RepoFile, error)
	removeFile func(path string) error
}

type RepositoryFolderOption func(*RepositoryFolder)

func withFileRemover(remove func(path string) error) RepositoryFolderOption {
	return func(f *RepositoryFolder) { f.removeFile = remove }
}

func NewRepositoryFolder(folder string, catalog RepositoryCatalog, open func(path string) (*RepoFile, error), opts ...RepositoryFolderOption) *RepositoryFolder {
	f := &RepositoryFolder{folder: folder, catalog: catalog, open: open, removeFile: os.Remove}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

func (f *RepositoryFolder) Create(ctx context.Context, req CreateRepoRequest) (models.Repository, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	listed, err := f.catalog.List(ctx)
	if err != nil {
		return models.Repository{}, fmt.Errorf("RepositoryFolder.Create: list catalog: %w", err)
	}
	for _, entry := range listed {
		if entry.Name == req.Name {
			return models.Repository{}, ErrRepoAlreadyExists
		}
	}
	if err := os.MkdirAll(f.folder, 0o755); err != nil {
		return models.Repository{}, fmt.Errorf("RepositoryFolder.Create: make folder: %w", err)
	}

	fileName := newID() + repositoryFileSuffix
	path := filepath.Join(f.folder, fileName)
	repo, err := f.createFile(ctx, req, fileName, path)
	if err != nil {
		return models.Repository{}, f.withCleanup(path, fmt.Errorf("RepositoryFolder.Create: %w", err))
	}
	return repo, nil
}

func (f *RepositoryFolder) createFile(ctx context.Context, req CreateRepoRequest, fileName, path string) (models.Repository, error) {
	file, err := f.open(path)
	if err != nil {
		return models.Repository{}, fmt.Errorf("open %s: %w", fileName, err)
	}
	repo, err := file.InitRepo(ctx, req)
	if err != nil {
		return models.Repository{}, errors.Join(fmt.Errorf("init repository: %w", err), file.Close())
	}
	if err := file.Close(); err != nil {
		return models.Repository{}, fmt.Errorf("close %s: %w", fileName, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return models.Repository{}, fmt.Errorf("stat %s: %w", fileName, err)
	}
	entry := RepositoryCatalogEntry{ID: repo.ID, Name: repo.Name, FileName: fileName, SizeBytes: info.Size()}
	if err := f.catalog.Register(ctx, entry); err != nil {
		return models.Repository{}, fmt.Errorf("register in catalog: %w", err)
	}
	return repo, nil
}

func (f *RepositoryFolder) withCleanup(path string, cause error) error {
	if err := f.removeFile(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Join(cause, fmt.Errorf("remove %s: %w", filepath.Base(path), err))
	}
	return cause
}

func (f *RepositoryFolder) Delete(ctx context.Context, repoID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	entry, err := f.catalog.Lookup(ctx, repoID)
	if errors.Is(err, ErrRepositoryNotCatalogued) {
		return ErrRepoNotInitialised
	}
	if err != nil {
		return fmt.Errorf("RepositoryFolder.Delete: lookup %s: %w", repoID, err)
	}
	if err := f.removeFile(filepath.Join(f.folder, entry.FileName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("RepositoryFolder.Delete: remove %s: %w", entry.FileName, err)
	}
	if err := f.catalog.Remove(ctx, repoID); err != nil && !errors.Is(err, ErrRepositoryNotCatalogued) {
		return fmt.Errorf("RepositoryFolder.Delete: remove catalog entry %s: %w", repoID, err)
	}
	return nil
}

func (f *RepositoryFolder) Purge(ctx context.Context, repoID string) error {
	return f.Delete(ctx, repoID)
}
