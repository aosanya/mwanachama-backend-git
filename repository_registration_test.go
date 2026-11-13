package mwanachamagit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const (
	copiedFileName     = "copied-repository.db"
	repositoryNameInDB = "original-repository"
)

type registrySketch struct {
	names map[string]bool
}

func newRegistrySketch() *registrySketch {
	return &registrySketch{names: map[string]bool{}}
}

func (r *registrySketch) check(_ context.Context, fileName string) (bool, error) {
	return r.names[fileName], nil
}

type openedFixture struct {
	folder string
	opener *RegisteredFileOpener[*gorm.DB]
	opens  *int
}

func buildRepositoryFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "original.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	s, err := LoadSpec(filepath.Join("spec", "examples", "engineering.git.json"))
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if err := Provision(db, s); err != nil {
		t.Fatalf("Provision: %v", err)
	}
	m, err := NewGitManager(db, s, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewGitManager: %v", err)
	}
	if _, err := m.InitRepo(context.Background(), CreateRepoRequest{Name: repositoryNameInDB}); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return path
}

func copyIntoRepositoriesFolder(t *testing.T, source string) string {
	t.Helper()
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	folder := filepath.Join(t.TempDir(), "repositories")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, copiedFileName), content, 0o644); err != nil {
		t.Fatalf("write copy: %v", err)
	}
	return folder
}

func newOpenedFixture(t *testing.T, check RegistrationCheck) openedFixture {
	t.Helper()
	folder := copyIntoRepositoriesFolder(t, buildRepositoryFile(t))
	opens := 0
	opener := NewRegisteredFileOpener[*gorm.DB](folder, check, func(path string) (*gorm.DB, error) {
		opens++
		db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
		if err != nil {
			return nil, err
		}
		t.Cleanup(func() {
			if sqlDB, err := db.DB(); err == nil {
				_ = sqlDB.Close()
			}
		})
		return db, nil
	})
	return openedFixture{folder: folder, opener: opener, opens: &opens}
}

func managerOnRegisteredFile(ctx context.Context, f openedFixture, fileName string) (GitManager, error) {
	db, err := f.opener.Open(ctx, fileName)
	if err != nil {
		return nil, err
	}
	s, err := LoadSpec(filepath.Join("spec", "examples", "engineering.git.json"))
	if err != nil {
		return nil, err
	}
	return NewGitManager(db, s, nil, nil, nil)
}

func TestRegisteredFileOpener_CopiedFileWithNoCatalogEntryIsRefusedOnOpen(t *testing.T) {
	f := newOpenedFixture(t, newRegistrySketch().check)

	_, err := f.opener.Open(context.Background(), copiedFileName)

	if !errors.Is(err, ErrRepositoryNotRegistered) {
		t.Fatalf("want ErrRepositoryNotRegistered, got %v", err)
	}
	if *f.opens != 0 {
		t.Fatalf("open function called %d times for an unregistered file", *f.opens)
	}
}

func TestRegisteredFileOpener_CopiedFileWithNoCatalogEntryYieldsNoManagerAndNoOperations(t *testing.T) {
	f := newOpenedFixture(t, newRegistrySketch().check)
	ctx := context.Background()

	m, err := managerOnRegisteredFile(ctx, f, copiedFileName)

	if !errors.Is(err, ErrRepositoryNotRegistered) {
		t.Fatalf("want ErrRepositoryNotRegistered, got %v", err)
	}
	if m != nil {
		t.Fatal("a manager was built on an unregistered file")
	}
	if *f.opens != 0 {
		t.Fatalf("open function called %d times for an unregistered file", *f.opens)
	}
}

func TestRegisteredFileOpener_UnregisteredFileIsRefusedUnderEveryRepeatedAttempt(t *testing.T) {
	f := newOpenedFixture(t, newRegistrySketch().check)

	for attempt := 0; attempt < 3; attempt++ {
		if _, err := f.opener.Open(context.Background(), copiedFileName); !errors.Is(err, ErrRepositoryNotRegistered) {
			t.Fatalf("attempt %d: want ErrRepositoryNotRegistered, got %v", attempt, err)
		}
	}
	if *f.opens != 0 {
		t.Fatalf("open function called %d times for an unregistered file", *f.opens)
	}
}

func TestRegisteredFileOpener_UnregisteredFileIsNotListed(t *testing.T) {
	f := newOpenedFixture(t, newRegistrySketch().check)

	names, err := f.opener.ListRegistered(context.Background())

	if err != nil {
		t.Fatalf("ListRegistered: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("unregistered file listed: %v", names)
	}
	if *f.opens != 0 {
		t.Fatalf("listing opened %d files", *f.opens)
	}
}

func TestRegisteredFileOpener_NamesThatAreNotPlainRepositoryFilesAreRefusedWithoutBeingMistakenForUnregistered(t *testing.T) {
	registry := newRegistrySketch()
	for _, name := range []string{"../x.db", "sub/x.db", `sub\x.db`, "..db", "x", "x.sqlite", "", ".."} {
		registry.names[name] = true
	}
	f := newOpenedFixture(t, registry.check)

	for _, name := range []string{"../x.db", "sub/x.db", `sub\x.db`, "..db", "x", "x.sqlite", "", ".."} {
		_, err := f.opener.Open(context.Background(), name)
		if err == nil {
			t.Fatalf("%q: expected an error", name)
		}
		if errors.Is(err, ErrRepositoryNotRegistered) {
			t.Fatalf("%q: refused as unregistered instead of as a bad name", name)
		}
	}
	if *f.opens != 0 {
		t.Fatalf("open function called %d times for bad names", *f.opens)
	}
}

func TestRegisteredFileOpener_RegisteringTheCopiedFileLetsItOpenAndAnswerItsRepository(t *testing.T) {
	registry := newRegistrySketch()
	f := newOpenedFixture(t, registry.check)
	ctx := context.Background()
	registry.names[copiedFileName] = true

	m, err := managerOnRegisteredFile(ctx, f, copiedFileName)
	if err != nil {
		t.Fatalf("open registered file: %v", err)
	}
	repos, err := m.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repos) != 1 || repos[0].Name != repositoryNameInDB {
		t.Fatalf("want the one copied repository %q, got %+v", repositoryNameInDB, repos)
	}
	branches, err := m.ListBranches(ctx, repos[0].ID)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(branches) != 1 || branches[0].Name != "main" || !branches[0].IsDefault {
		t.Fatalf("want the one default branch main, got %+v", branches)
	}
	if *f.opens != 1 {
		t.Fatalf("want one open, got %d", *f.opens)
	}
}

func TestRegisteredFileOpener_RegisteringTheCopiedFileListsIt(t *testing.T) {
	registry := newRegistrySketch()
	f := newOpenedFixture(t, registry.check)
	registry.names[copiedFileName] = true

	names, err := f.opener.ListRegistered(context.Background())

	if err != nil {
		t.Fatalf("ListRegistered: %v", err)
	}
	if !reflect.DeepEqual(names, []string{copiedFileName}) {
		t.Fatalf("want [%s], got %v", copiedFileName, names)
	}
}

func TestRegisteredFileOpener_UnregisteringTheFileRefusesItAgainAndDropsItFromTheListing(t *testing.T) {
	registry := newRegistrySketch()
	f := newOpenedFixture(t, registry.check)
	ctx := context.Background()
	registry.names[copiedFileName] = true
	if _, err := f.opener.Open(ctx, copiedFileName); err != nil {
		t.Fatalf("open while registered: %v", err)
	}
	opensWhileRegistered := *f.opens

	delete(registry.names, copiedFileName)

	if _, err := f.opener.Open(ctx, copiedFileName); !errors.Is(err, ErrRepositoryNotRegistered) {
		t.Fatalf("want ErrRepositoryNotRegistered after unregistering, got %v", err)
	}
	if *f.opens != opensWhileRegistered {
		t.Fatalf("open function called again after unregistering")
	}
	names, err := f.opener.ListRegistered(ctx)
	if err != nil {
		t.Fatalf("ListRegistered: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("unregistered file still listed: %v", names)
	}
}

func TestRegisteredFileOpener_FailingRegistrationCheckPropagatesAndOpensNothing(t *testing.T) {
	checkFailure := errors.New("catalog unreachable")
	f := newOpenedFixture(t, func(context.Context, string) (bool, error) { return true, checkFailure })

	_, openErr := f.opener.Open(context.Background(), copiedFileName)
	_, listErr := f.opener.ListRegistered(context.Background())

	if !errors.Is(openErr, checkFailure) {
		t.Fatalf("Open: want the check failure, got %v", openErr)
	}
	if errors.Is(openErr, ErrRepositoryNotRegistered) {
		t.Fatal("Open: check failure reported as unregistered")
	}
	if !errors.Is(listErr, checkFailure) {
		t.Fatalf("ListRegistered: want the check failure, got %v", listErr)
	}
	if *f.opens != 0 {
		t.Fatalf("open function called %d times after a failing check", *f.opens)
	}
}
