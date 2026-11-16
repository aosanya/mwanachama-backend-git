package mwanachamagit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func RepositoryFileAt(path string) RepositoryFile {
	return func(string) (string, int64, error) {
		info, err := os.Stat(path)
		if err != nil {
			return "", 0, fmt.Errorf("RepositoryFileAt %s: %w", filepath.Base(path), err)
		}
		return filepath.Base(path), info.Size(), nil
	}
}

func RepositoryFileInFolder(folder string, catalog RepositoryCatalog) RepositoryFile {
	return func(repoID string) (string, int64, error) {
		entry, err := catalog.Lookup(context.Background(), repoID)
		if err != nil {
			return "", 0, err
		}
		return RepositoryFileAt(filepath.Join(folder, entry.FileName))(repoID)
	}
}
