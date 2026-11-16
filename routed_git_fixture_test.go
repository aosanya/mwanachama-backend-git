package mwanachamagit

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
	"github.com/aosanya/mwanachama-backend-shared/spec"
)

type routedGitFixture struct {
	folder  string
	catalog *MemoryRepositoryCatalog
	spec    *spec.Spec
	manager *RoutedGitManager
	ctx     context.Context
}

func newRoutedGitFixture(t *testing.T, maxOpen int) *routedGitFixture {
	t.Helper()
	s, err := LoadSpec(filepath.Join("spec", "examples", "engineering.git.json"))
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	folder := t.TempDir()
	catalog := NewMemoryRepositoryCatalog()
	manager, err := NewRoutedGitManager(folder, maxOpen, s, catalog)
	if err != nil {
		t.Fatalf("NewRoutedGitManager: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	return &routedGitFixture{folder: folder, catalog: catalog, spec: s, manager: manager, ctx: context.Background()}
}

func (f *routedGitFixture) reopened(t *testing.T) *RoutedGitManager {
	t.Helper()
	again, err := NewRoutedGitManager(f.folder, 4, f.spec, f.catalog)
	if err != nil {
		t.Fatalf("NewRoutedGitManager: %v", err)
	}
	t.Cleanup(func() { _ = again.Close() })
	return again
}

func (f *routedGitFixture) initRepo(t *testing.T, name string) (models.Repository, models.Branch) {
	t.Helper()
	repo, err := f.manager.InitRepo(f.ctx, CreateRepoRequest{Name: name})
	if err != nil {
		t.Fatalf("InitRepo %s: %v", name, err)
	}
	main, err := f.manager.GetBranchByName(f.ctx, repo.ID, repo.DefaultBranch)
	if err != nil {
		t.Fatalf("GetBranchByName %s: %v", name, err)
	}
	return repo, main
}

func (f *routedGitFixture) writeAndMerge(t *testing.T, repo models.Repository, main models.Branch, path, content string) {
	t.Helper()
	work, err := f.manager.CreateBranch(f.ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "work-" + path, FromBranchID: main.ID})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if _, err := f.manager.WriteFile(f.ctx, WriteFileRequest{BranchID: work.ID, Path: path, Content: content}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := f.manager.MergeBranch(f.ctx, work.ID); err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
}
