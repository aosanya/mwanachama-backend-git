package mwanachamagit

import (
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func (f *routedGitFixture) keywordInFileOf(t *testing.T, repo models.Repository, name string) models.Keyword {
	t.Helper()
	var keyword models.Keyword
	f.inFileOf(t, repo, func(file *RepoFile, _ *gitManager) {
		created, err := file.CreateKeyword(f.ctx, CreateKeywordRequest{Name: name})
		if err != nil {
			t.Fatalf("CreateKeyword %s in %s: %v", name, repo.Name, err)
		}
		keyword = created
	})
	return keyword
}

func TestRoutedGitManager_AKeywordIsReadUpdatedAndDeletedInTheFileThatHoldsItInEitherOrder(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	aaa, _ := f.initRepo(t, "aaa")
	zzz, _ := f.initRepo(t, "zzz")
	inA := f.keywordInFileOf(t, aaa, "in-aaa")
	inZ := f.keywordInFileOf(t, zzz, "in-zzz")
	for _, want := range []models.Keyword{inZ, inA, inZ, inA} {
		got, err := f.manager.GetKeyword(f.ctx, want.ID)
		if err != nil || got.ID != want.ID || got.Name != want.Name {
			t.Fatalf("GetKeyword(%s) = %+v, %v", want.ID, got, err)
		}
	}
	for _, want := range []models.Keyword{inZ, inA} {
		renamed, err := f.manager.UpdateKeyword(f.ctx, want.ID, UpdateKeywordRequest{Name: want.Name + "-renamed", Description: "changed"})
		if err != nil || renamed.ID != want.ID || renamed.Name != want.Name+"-renamed" || renamed.Description != "changed" {
			t.Fatalf("UpdateKeyword(%s) = %+v, %v", want.ID, renamed, err)
		}
		again, err := f.manager.GetKeyword(f.ctx, want.ID)
		if err != nil || again.Name != want.Name+"-renamed" {
			t.Fatalf("GetKeyword(%s) after update = %+v, %v", want.ID, again, err)
		}
	}
	if err := f.manager.DeleteKeyword(f.ctx, inZ.ID); err != nil {
		t.Fatalf("DeleteKeyword in the second file: %v", err)
	}
	if _, err := f.manager.GetKeyword(f.ctx, inZ.ID); !errors.Is(err, ErrKeywordNotFound) {
		t.Fatalf("GetKeyword of the deleted keyword = %v, want ErrKeywordNotFound", err)
	}
	if kept, err := f.manager.GetKeyword(f.ctx, inA.ID); err != nil || kept.ID != inA.ID {
		t.Fatalf("GetKeyword of the keyword in the first file after the other delete = %+v, %v", kept, err)
	}
	if err := f.manager.DeleteKeyword(f.ctx, inA.ID); err != nil {
		t.Fatalf("DeleteKeyword in the first file: %v", err)
	}
}

func TestRoutedGitManager_AnUnknownKeywordIdIsNotFoundInAnyFile(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	f.initRepo(t, "aaa")
	f.initRepo(t, "zzz")
	if _, err := f.manager.GetKeyword(f.ctx, "no-such-keyword"); !errors.Is(err, ErrKeywordNotFound) {
		t.Errorf("GetKeyword = %v, want ErrKeywordNotFound", err)
	}
	if _, err := f.manager.UpdateKeyword(f.ctx, "no-such-keyword", UpdateKeywordRequest{Name: "x"}); !errors.Is(err, ErrKeywordNotFound) {
		t.Errorf("UpdateKeyword = %v, want ErrKeywordNotFound", err)
	}
	if err := f.manager.DeleteKeyword(f.ctx, "no-such-keyword"); !errors.Is(err, ErrKeywordNotFound) {
		t.Errorf("DeleteKeyword = %v, want ErrKeywordNotFound", err)
	}
}

func TestRoutedGitManager_RootKeywordsOfEveryFileAreListedAndCutToTheLimit(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	aaa, _ := f.initRepo(t, "aaa")
	zzz, _ := f.initRepo(t, "zzz")
	f.keywordInFileOf(t, aaa, "in-aaa")
	f.keywordInFileOf(t, zzz, "in-zzz")
	all, err := f.manager.ListKeywords(f.ctx, KeywordFilter{})
	if err != nil || len(all) != 2 {
		t.Fatalf("ListKeywords with no limit = %+v, %v; want both roots", all, err)
	}
	cut, err := f.manager.ListKeywords(f.ctx, KeywordFilter{Limit: 1})
	if err != nil || len(cut) != 1 {
		t.Fatalf("ListKeywords with limit 1 = %+v, %v; want one", cut, err)
	}
}
