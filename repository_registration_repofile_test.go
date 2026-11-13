package mwanachamagit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type registeredRepoFileFixture struct {
	opener  *RegisteredFileOpener[*RepoFile]
	catalog RepositoryCatalog
}

func newRegisteredRepoFileFixture(t *testing.T) registeredRepoFileFixture {
	t.Helper()
	ctx := context.Background()
	s, err := LoadSpec(filepath.Join("spec", "examples", "engineering.git.json"))
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}

	sourcePath := filepath.Join(t.TempDir(), "source.db")
	source, err := OpenRepoFile(sourcePath, s, nil, nil, nil)
	if err != nil {
		t.Fatalf("OpenRepoFile source: %v", err)
	}
	if _, err := source.InitRepo(ctx, CreateRepoRequest{Name: "widgets"}); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	if err := source.Close(); err != nil {
		t.Fatalf("Close source: %v", err)
	}
	raw, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}

	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, "copied.db"), raw, 0o600); err != nil {
		t.Fatalf("copy into repositories folder: %v", err)
	}

	catalog := NewMemoryRepositoryCatalog()
	return registeredRepoFileFixture{
		opener:  NewRegisteredRepoFileOpener(folder, catalog, s, nil, nil, nil),
		catalog: catalog,
	}
}

func TestRegisteredRepoFile_ACopiedFileIsRefusedUntilItsCatalogEntryExists(t *testing.T) {
	ctx := context.Background()
	f := newRegisteredRepoFileFixture(t)

	if _, err := f.opener.Open(ctx, "copied.db"); !errors.Is(err, ErrRepositoryNotRegistered) {
		t.Fatalf("Open of unregistered copy = %v, want ErrRepositoryNotRegistered", err)
	}
	names, err := f.opener.ListRegistered(ctx)
	if err != nil || len(names) != 0 {
		t.Fatalf("ListRegistered = %v (err=%v), want empty", names, err)
	}

	if err := f.catalog.Register(ctx, RepositoryCatalogEntry{ID: "r1", Name: "widgets", FileName: "copied.db"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	rf, err := f.opener.Open(ctx, "copied.db")
	if err != nil {
		t.Fatalf("Open of registered copy: %v", err)
	}
	repos, err := rf.ListRepositories(ctx)
	if err != nil || len(repos) != 1 || repos[0].Name != "widgets" {
		t.Fatalf("ListRepositories = %+v (err=%v), want the one repository widgets", repos, err)
	}
	if err := rf.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	names, err = f.opener.ListRegistered(ctx)
	if err != nil || len(names) != 1 || names[0] != "copied.db" {
		t.Fatalf("ListRegistered = %v (err=%v), want [copied.db]", names, err)
	}

	if err := f.catalog.Remove(ctx, "r1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := f.opener.Open(ctx, "copied.db"); !errors.Is(err, ErrRepositoryNotRegistered) {
		t.Fatalf("Open after catalog removal = %v, want ErrRepositoryNotRegistered", err)
	}
}

func TestRegisteredRepoFile_ACatalogEntryForAnotherFileNameDoesNotRegisterTheCopy(t *testing.T) {
	ctx := context.Background()
	f := newRegisteredRepoFileFixture(t)

	if err := f.catalog.Register(ctx, RepositoryCatalogEntry{ID: "r2", Name: "other", FileName: "other.db"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := f.opener.Open(ctx, "copied.db"); !errors.Is(err, ErrRepositoryNotRegistered) {
		t.Fatalf("Open with only other.db catalogued = %v, want ErrRepositoryNotRegistered", err)
	}
}

func TestRegistrationCheckFromCatalog_MatchesOnFileNameOnly(t *testing.T) {
	ctx := context.Background()
	catalog := NewMemoryRepositoryCatalog()
	check := RegistrationCheckFromCatalog(catalog)

	if ok, err := check(ctx, "a.db"); err != nil || ok {
		t.Fatalf("empty catalog: ok=%v err=%v, want false", ok, err)
	}
	if err := catalog.Register(ctx, RepositoryCatalogEntry{ID: "r1", Name: "a.db", FileName: "b.db"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if ok, _ := check(ctx, "a.db"); ok {
		t.Fatalf("a.db matched on name instead of file name")
	}
	if ok, _ := check(ctx, "b.db"); !ok {
		t.Fatalf("b.db should match its entry's file name")
	}
}
