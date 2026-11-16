package mwanachamagit

import (
	"errors"
	"testing"
)

func TestRoutedGitManager_ABlobSearchReachesTheFileOfTheNamedRepositoryInEitherOrder(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	f.initRepo(t, "aaa")
	f.initRepo(t, "zzz")
	for _, name := range []string{"zzz", "aaa", "zzz", "aaa"} {
		if _, err := f.manager.SearchBlobs(f.ctx, SearchBlobsRequest{RepositoryName: name}); !errors.Is(err, ErrQueryRequired) {
			t.Fatalf("SearchBlobs of %s with no query = %v, want ErrQueryRequired from its file", name, err)
		}
	}
}

func TestRoutedGitManager_ABlobSearchOfAnUnknownRepositoryNameAnswersErrRepoNotInitialised(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	f.initRepo(t, "aaa")
	f.initRepo(t, "zzz")
	for _, name := range []string{"missing", ""} {
		if _, err := f.manager.SearchBlobs(f.ctx, SearchBlobsRequest{Query: "auth", RepositoryName: name}); !errors.Is(err, ErrRepoNotInitialised) {
			t.Fatalf("SearchBlobs of %q = %v, want ErrRepoNotInitialised", name, err)
		}
	}
}

func TestRoutedGitManager_ABlobSearchOfARepositoryWithNoSearcherAnswersAnEmptyListWithoutError(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	f.initRepo(t, "aaa")
	repo, main := f.initRepo(t, "zzz")
	f.writeAndMerge(t, repo, main, "auth.md", "authentication notes")
	got, err := f.manager.SearchBlobs(f.ctx, SearchBlobsRequest{Query: "auth", RepositoryName: "zzz", Limit: 5})
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("SearchBlobs = %#v, %v; want an empty non-nil list", got, err)
	}
}

func TestRoutedGitManager_ABlobSearchWithNoRepositoryInTheCatalogAnswersErrRepoNotInitialised(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	if _, err := f.manager.SearchBlobs(f.ctx, SearchBlobsRequest{Query: "auth", RepositoryName: "zzz"}); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("SearchBlobs with an empty catalog = %v, want ErrRepoNotInitialised", err)
	}
}
