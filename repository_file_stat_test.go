package mwanachamagit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryFileAt_ReportsTheRealSizeOfTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "widgets.db")
	if err := os.WriteFile(path, make([]byte, 1234), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	fileName, size, err := RepositoryFileAt(path)("any-id-at-all")
	if err != nil {
		t.Fatalf("RepositoryFileAt: %v", err)
	}
	if fileName != "widgets.db" || size != 1234 {
		t.Fatalf("got %q, %d; want widgets.db, 1234", fileName, size)
	}
}

func TestRepositoryFileAt_ReportsAMissingFile(t *testing.T) {
	_, _, err := RepositoryFileAt(filepath.Join(t.TempDir(), "gone.db"))("id")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("got %v, want a not-exist error", err)
	}
}

func TestRepositoryFileInFolder_FindsTheFileThroughTheCatalog(t *testing.T) {
	catalog := NewMemoryRepositoryCatalog()
	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, "widgets.db"), make([]byte, 777), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	entry := RepositoryCatalogEntry{ID: "repo-widgets", Name: "widgets", FileName: "widgets.db"}
	if err := catalog.Register(context.Background(), entry); err != nil {
		t.Fatalf("Register: %v", err)
	}
	file := RepositoryFileInFolder(folder, catalog)
	fileName, size, err := file(entry.ID)
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	if fileName != "widgets.db" || size != 777 {
		t.Fatalf("got %q, %d; want widgets.db, 777", fileName, size)
	}
	if _, _, err := file("unregistered"); !errors.Is(err, ErrRepositoryNotCatalogued) {
		t.Fatalf("unregistered id = %v, want ErrRepositoryNotCatalogued", err)
	}
}
