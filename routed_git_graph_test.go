package mwanachamagit

import (
	"errors"
	"testing"
)

func TestRoutedGitManager_KeywordsLiveInTheFileOfTheirParent(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	f.initRepo(t, "aaa")
	f.initRepo(t, "zzz")
	root, err := f.manager.CreateKeyword(f.ctx, CreateKeywordRequest{Name: "topic"})
	if err != nil {
		t.Fatalf("CreateKeyword root: %v", err)
	}
	child, err := f.manager.CreateKeyword(f.ctx, CreateKeywordRequest{Name: "sub", ParentID: root.ID})
	if err != nil {
		t.Fatalf("CreateKeyword child: %v", err)
	}
	roots, err := f.manager.ListKeywords(f.ctx, KeywordFilter{})
	if err != nil || len(roots) != 1 || roots[0].ID != root.ID {
		t.Fatalf("root keywords = %+v, %v", roots, err)
	}
	children, err := f.manager.ListKeywords(f.ctx, KeywordFilter{ParentID: root.ID})
	if err != nil || len(children) != 1 || children[0].ID != child.ID {
		t.Fatalf("child keywords = %+v, %v", children, err)
	}
	tree, err := f.manager.GetKeywordTree(f.ctx, root.ID)
	if err != nil || len(tree) == 0 {
		t.Fatalf("GetKeywordTree = %+v, %v", tree, err)
	}
}

func TestRoutedGitManager_ACreatedKeywordWithNoRepositoryIsRefused(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	if _, err := f.manager.CreateKeyword(f.ctx, CreateKeywordRequest{Name: "topic"}); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("CreateKeyword with no repository = %v, want ErrRepoNotInitialised", err)
	}
}

func TestRoutedGitManager_ImportingARepositoryIsRefusedOnFiles(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	if _, err := f.manager.ImportRepo(f.ctx, ImportRepoRequest{Name: "x", SourceURL: "https://example.invalid/x.git"}); !errors.Is(err, ErrImportNeedsRepositoryFile) {
		t.Fatalf("ImportRepo = %v, want ErrImportNeedsRepositoryFile", err)
	}
}
