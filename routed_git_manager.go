package mwanachamagit

import (
	"github.com/aosanya/mwanachama-backend-shared/spec"
)

type RoutedGitManager struct {
	catalog RepositoryCatalog
	folder  *RepositoryFolder
	files   *RepoFiles
	owners  fileOwners
}

func NewRoutedGitManager(folder string, maxOpen int, s *spec.Spec, catalog RepositoryCatalog) (*RoutedGitManager, error) {
	openForCreate := func(path string) (*RepoFile, error) {
		return OpenRepoFile(path, s, nil, nil, nil)
	}
	openForUse := func(path string) (*RepoFile, error) {
		return OpenRepoFile(path, s, nil, nil, nil,
			WithRepositoryCatalog(catalog, RepositoryFileInFolder(folder, catalog)))
	}
	files, err := NewRepoFiles(folder, maxOpen, RegistrationCheckFromCatalog(catalog), openForUse)
	if err != nil {
		return nil, err
	}
	return &RoutedGitManager{
		catalog: catalog,
		folder:  NewRepositoryFolder(folder, catalog, openForCreate),
		files:   files,
	}, nil
}

func (g *RoutedGitManager) Close() error { return g.files.Close() }

func (g *RoutedGitManager) openFiles() int { return g.files.Open() }
