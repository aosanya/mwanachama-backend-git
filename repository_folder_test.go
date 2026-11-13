package mwanachamagit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

type faultyRepositoryCatalog struct {
	*MemoryRepositoryCatalog
	registerErr error
	removeErr   error
}

func (c *faultyRepositoryCatalog) Register(ctx context.Context, entry RepositoryCatalogEntry) error {
	if c.registerErr != nil {
		return c.registerErr
	}
	return c.MemoryRepositoryCatalog.Register(ctx, entry)
}

func (c *faultyRepositoryCatalog) Remove(ctx context.Context, id string) error {
	if c.removeErr != nil {
		return c.removeErr
	}
	return c.MemoryRepositoryCatalog.Remove(ctx, id)
}

type repositoryFolderFixture struct {
	folder  string
	catalog *faultyRepositoryCatalog
	open    func(path string) (*RepoFile, error)
}

func newRepositoryFolderFixture(t *testing.T) repositoryFolderFixture {
	t.Helper()
	s, err := LoadSpec(filepath.Join("spec", "examples", "engineering.git.json"))
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	return repositoryFolderFixture{
		folder:  t.TempDir(),
		catalog: &faultyRepositoryCatalog{MemoryRepositoryCatalog: NewMemoryRepositoryCatalog()},
		open: func(path string) (*RepoFile, error) {
			return OpenRepoFile(path, s, nil, nil, nil)
		},
	}
}

func (f repositoryFolderFixture) repositoryFolder(opts ...RepositoryFolderOption) *RepositoryFolder {
	return NewRepositoryFolder(f.folder, f.catalog, f.open, opts...)
}

func (f repositoryFolderFixture) dbFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(f.folder)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".db") {
			names = append(names, entry.Name())
		}
	}
	return names
}

func (f repositoryFolderFixture) entries(t *testing.T) []RepositoryCatalogEntry {
	t.Helper()
	listed, err := f.catalog.List(context.Background())
	if err != nil {
		t.Fatalf("catalog List: %v", err)
	}
	return listed
}

func (f repositoryFolderFixture) create(t *testing.T, folder *RepositoryFolder, name string) models.Repository {
	t.Helper()
	repo, err := folder.Create(context.Background(), CreateRepoRequest{Name: name})
	if err != nil {
		t.Fatalf("Create %s: %v", name, err)
	}
	return repo
}

func failingRemover(err error) RepositoryFolderOption {
	return withFileRemover(func(string) error { return err })
}

func TestRepositoryFolderCreate_MakesTheFileAndOneCatalogEntryForIt(t *testing.T) {
	f := newRepositoryFolderFixture(t)
	folder := f.repositoryFolder()

	repo := f.create(t, folder, "widgets")

	files := f.dbFiles(t)
	if len(files) != 1 {
		t.Fatalf("db files in folder = %v, want exactly one", files)
	}
	entries := f.entries(t)
	if len(entries) != 1 {
		t.Fatalf("catalog entries = %+v, want exactly one", entries)
	}
	entry := entries[0]
	if entry.FileName != files[0] {
		t.Fatalf("entry FileName = %q, want the created file %q", entry.FileName, files[0])
	}
	if entry.ID != repo.ID {
		t.Fatalf("entry ID = %q, want the returned repository ID %q", entry.ID, repo.ID)
	}
	if entry.Name != "widgets" {
		t.Fatalf("entry Name = %q, want widgets", entry.Name)
	}
	info, err := os.Stat(filepath.Join(f.folder, files[0]))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if entry.SizeBytes != info.Size() {
		t.Fatalf("entry SizeBytes = %d, want the file's size on disk %d", entry.SizeBytes, info.Size())
	}

	rf, err := f.open(filepath.Join(f.folder, entry.FileName))
	if err != nil {
		t.Fatalf("open created file: %v", err)
	}
	defer rf.Close()
	repos, err := rf.ListRepositories(context.Background())
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repos) != 1 || repos[0].ID != repo.ID {
		t.Fatalf("ListRepositories = %+v, want the one repository %s", repos, repo.ID)
	}
}

func TestRepositoryFolderCreate_AFailedEntryWriteLeavesNoFileAndNoEntry(t *testing.T) {
	f := newRepositoryFolderFixture(t)
	registerFailure := errors.New("catalog register failed")
	f.catalog.registerErr = registerFailure

	_, err := f.repositoryFolder().Create(context.Background(), CreateRepoRequest{Name: "widgets"})

	if !errors.Is(err, registerFailure) {
		t.Fatalf("Create = %v, want the register error", err)
	}
	if files := f.dbFiles(t); len(files) != 0 {
		t.Fatalf("db files left behind = %v, want none", files)
	}
	if entries := f.entries(t); len(entries) != 0 {
		t.Fatalf("catalog entries = %+v, want none", entries)
	}
}

func TestRepositoryFolderCreate_AFailedEntryWriteWhoseCleanupAlsoFailsReportsBothErrors(t *testing.T) {
	f := newRepositoryFolderFixture(t)
	registerFailure := errors.New("catalog register failed")
	cleanupFailure := errors.New("file removal failed")
	f.catalog.registerErr = registerFailure

	_, err := f.repositoryFolder(failingRemover(cleanupFailure)).Create(context.Background(), CreateRepoRequest{Name: "widgets"})

	if !errors.Is(err, registerFailure) {
		t.Fatalf("Create = %v, want it to match the register error", err)
	}
	if !errors.Is(err, cleanupFailure) {
		t.Fatalf("Create = %v, want it to match the cleanup error", err)
	}
	if entries := f.entries(t); len(entries) != 0 {
		t.Fatalf("catalog entries = %+v, want none", entries)
	}
}

func TestRepositoryFolderCreate_ANameAlreadyCataloguedIsRefusedWithoutASecondFile(t *testing.T) {
	f := newRepositoryFolderFixture(t)
	folder := f.repositoryFolder()
	f.create(t, folder, "widgets")
	filesBefore := f.dbFiles(t)

	_, err := folder.Create(context.Background(), CreateRepoRequest{Name: "widgets"})

	if !errors.Is(err, ErrRepoAlreadyExists) {
		t.Fatalf("Create of a catalogued name = %v, want ErrRepoAlreadyExists", err)
	}
	filesAfter := f.dbFiles(t)
	if len(filesAfter) != len(filesBefore) || len(filesAfter) != 1 {
		t.Fatalf("db files after refusal = %v, want the original %v only", filesAfter, filesBefore)
	}
	if entries := f.entries(t); len(entries) != 1 {
		t.Fatalf("catalog entries = %+v, want the original only", entries)
	}
}

func TestRepositoryFolderCreate_AFailedOpenLeavesNoFileAndNoEntry(t *testing.T) {
	f := newRepositoryFolderFixture(t)
	openFailure := errors.New("open failed")
	folder := NewRepositoryFolder(f.folder, f.catalog, func(path string) (*RepoFile, error) {
		return nil, openFailure
	})

	_, err := folder.Create(context.Background(), CreateRepoRequest{Name: "widgets"})

	if !errors.Is(err, openFailure) {
		t.Fatalf("Create = %v, want the open error", err)
	}
	if files := f.dbFiles(t); len(files) != 0 {
		t.Fatalf("db files left behind = %v, want none", files)
	}
	if entries := f.entries(t); len(entries) != 0 {
		t.Fatalf("catalog entries = %+v, want none", entries)
	}
}

func TestRepositoryFolderDelete_RemovesTheFileAndTheEntry(t *testing.T) {
	f := newRepositoryFolderFixture(t)
	folder := f.repositoryFolder()
	repo := f.create(t, folder, "widgets")
	fileName := f.entries(t)[0].FileName

	if err := folder.Delete(context.Background(), repo.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := os.Stat(filepath.Join(f.folder, fileName)); !os.IsNotExist(err) {
		t.Fatalf("Stat after Delete = %v, want the file gone", err)
	}
	if files := f.dbFiles(t); len(files) != 0 {
		t.Fatalf("db files after Delete = %v, want none", files)
	}
	if entries := f.entries(t); len(entries) != 0 {
		t.Fatalf("catalog entries after Delete = %+v, want none", entries)
	}
}

func TestRepositoryFolderDelete_AFailedFileRemovalKeepsTheEntryAndTheFile(t *testing.T) {
	f := newRepositoryFolderFixture(t)
	removalFailure := errors.New("file removal failed")
	repo := f.create(t, f.repositoryFolder(), "widgets")
	fileName := f.entries(t)[0].FileName

	err := f.repositoryFolder(failingRemover(removalFailure)).Delete(context.Background(), repo.ID)

	if !errors.Is(err, removalFailure) {
		t.Fatalf("Delete = %v, want the removal error", err)
	}
	if _, statErr := os.Stat(filepath.Join(f.folder, fileName)); statErr != nil {
		t.Fatalf("Stat after failed Delete = %v, want the file kept", statErr)
	}
	entries := f.entries(t)
	if len(entries) != 1 || entries[0].ID != repo.ID {
		t.Fatalf("catalog entries after failed Delete = %+v, want the entry kept", entries)
	}
}

func TestRepositoryFolderDelete_AFailedEntryRemovalKeepsTheEntryAndASecondDeleteCompletes(t *testing.T) {
	f := newRepositoryFolderFixture(t)
	folder := f.repositoryFolder()
	repo := f.create(t, folder, "widgets")
	fileName := f.entries(t)[0].FileName
	entryRemovalFailure := errors.New("catalog remove failed")
	f.catalog.removeErr = entryRemovalFailure

	err := folder.Delete(context.Background(), repo.ID)

	if !errors.Is(err, entryRemovalFailure) {
		t.Fatalf("Delete = %v, want the entry removal error", err)
	}
	if _, statErr := os.Stat(filepath.Join(f.folder, fileName)); !os.IsNotExist(statErr) {
		t.Fatalf("Stat after failed entry removal = %v, want the file already gone", statErr)
	}
	entries := f.entries(t)
	if len(entries) != 1 || entries[0].ID != repo.ID {
		t.Fatalf("catalog entries after failed entry removal = %+v, want the entry kept", entries)
	}

	f.catalog.removeErr = nil
	if err := folder.Delete(context.Background(), repo.ID); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
	if entries := f.entries(t); len(entries) != 0 {
		t.Fatalf("catalog entries after second Delete = %+v, want none", entries)
	}
}

func TestRepositoryFolderDelete_AnUnknownIDIsRefusedAndTouchesNothing(t *testing.T) {
	f := newRepositoryFolderFixture(t)
	folder := f.repositoryFolder()
	repo := f.create(t, folder, "widgets")
	filesBefore := f.dbFiles(t)

	err := folder.Delete(context.Background(), "no-such-repository")

	if !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("Delete of an unknown id = %v, want ErrRepoNotInitialised", err)
	}
	filesAfter := f.dbFiles(t)
	if len(filesAfter) != 1 || filesAfter[0] != filesBefore[0] {
		t.Fatalf("db files after refused Delete = %v, want %v", filesAfter, filesBefore)
	}
	entries := f.entries(t)
	if len(entries) != 1 || entries[0].ID != repo.ID {
		t.Fatalf("catalog entries after refused Delete = %+v, want the existing entry", entries)
	}
}

func TestRepositoryFolderPurge_RemovesTheFileAndTheEntryLikeDelete(t *testing.T) {
	f := newRepositoryFolderFixture(t)
	folder := f.repositoryFolder()
	repo := f.create(t, folder, "widgets")
	fileName := f.entries(t)[0].FileName

	if err := folder.Purge(context.Background(), repo.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	if _, err := os.Stat(filepath.Join(f.folder, fileName)); !os.IsNotExist(err) {
		t.Fatalf("Stat after Purge = %v, want the file gone", err)
	}
	if entries := f.entries(t); len(entries) != 0 {
		t.Fatalf("catalog entries after Purge = %+v, want none", entries)
	}
}

func TestRepositoryFolderDelete_LeavesTheOtherRepositorysFileAndEntryIntact(t *testing.T) {
	f := newRepositoryFolderFixture(t)
	folder := f.repositoryFolder()
	doomed := f.create(t, folder, "widgets")
	kept := f.create(t, folder, "gadgets")
	var keptEntry RepositoryCatalogEntry
	for _, entry := range f.entries(t) {
		if entry.ID == kept.ID {
			keptEntry = entry
		}
	}

	if err := folder.Delete(context.Background(), doomed.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	files := f.dbFiles(t)
	if len(files) != 1 || files[0] != keptEntry.FileName {
		t.Fatalf("db files after Delete = %v, want only %q", files, keptEntry.FileName)
	}
	entries := f.entries(t)
	if len(entries) != 1 || entries[0].ID != kept.ID || entries[0].Name != "gadgets" {
		t.Fatalf("catalog entries after Delete = %+v, want only gadgets", entries)
	}
	rf, err := f.open(filepath.Join(f.folder, keptEntry.FileName))
	if err != nil {
		t.Fatalf("open kept file: %v", err)
	}
	defer rf.Close()
	repos, err := rf.ListRepositories(context.Background())
	if err != nil || len(repos) != 1 || repos[0].ID != kept.ID {
		t.Fatalf("kept file ListRepositories = %+v (err=%v), want the one repository %s", repos, err, kept.ID)
	}
}
