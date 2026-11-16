package mwanachamagit

import (
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

type routedTaggedRepository struct {
	repo   models.Repository
	commit models.Commit
	tag    models.Tag
}

func routedTagInEachRepository(t *testing.T, f *routedGitFixture) (first, second routedTaggedRepository) {
	t.Helper()
	tagged := make([]routedTaggedRepository, 0, 2)
	for _, name := range []string{"aaa", "zzz"} {
		repo, main := f.initRepo(t, name)
		commit, err := f.manager.WriteFile(f.ctx, WriteFileRequest{BranchID: main.ID, Path: name + ".md", Content: name})
		if err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
		tag, err := f.manager.CreateTag(f.ctx, CreateTagRequest{RepositoryID: repo.ID, Name: "v1", CommitID: commit.ID, TaggerName: "tagger-" + name})
		if err != nil {
			t.Fatalf("CreateTag %s: %v", name, err)
		}
		tagged = append(tagged, routedTaggedRepository{repo: repo, commit: commit, tag: tag})
	}
	return tagged[0], tagged[1]
}

func TestRoutedGitManager_ATagIsCreatedInTheFileOfItsRepositoryAndPointsAtThatFilesCommit(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	first, second := routedTagInEachRepository(t, f)
	for _, tagged := range []routedTaggedRepository{first, second} {
		if tagged.tag.RepositoryID != tagged.repo.ID || tagged.tag.SHA != tagged.commit.SHA {
			t.Fatalf("tag = %+v, want repository %s and sha %s", tagged.tag, tagged.repo.ID, tagged.commit.SHA)
		}
	}
	if first.tag.ID == second.tag.ID {
		t.Fatalf("both repositories answered the tag id %s", first.tag.ID)
	}
}

func TestRoutedGitManager_ATagIsReadByIdFromWhicheverFileOwnsItInEitherOrder(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	first, second := routedTagInEachRepository(t, f)
	for _, want := range []routedTaggedRepository{second, first, second, first} {
		got, err := f.manager.GetTag(f.ctx, want.tag.ID)
		if err != nil || got.ID != want.tag.ID || got.RepositoryID != want.repo.ID || got.TaggerName != want.tag.TaggerName {
			t.Fatalf("GetTag(%s) = %+v, %v; want %+v", want.tag.ID, got, err, want.tag)
		}
	}
}

func TestRoutedGitManager_TagsAreListedOnlyFromTheFileOfTheRepositoryAsked(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	first, second := routedTagInEachRepository(t, f)
	for _, want := range []routedTaggedRepository{second, first} {
		tags, err := f.manager.ListTags(f.ctx, want.repo.ID)
		if err != nil || len(tags) != 1 || tags[0].ID != want.tag.ID {
			t.Fatalf("ListTags(%s) = %+v, %v; want only %s", want.repo.ID, tags, err, want.tag.ID)
		}
	}
	if _, err := f.manager.ListTags(f.ctx, "no-such-repository"); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("ListTags of an unknown repository = %v, want ErrRepoNotInitialised", err)
	}
}

func TestRoutedGitManager_ATagNameTakenInOneFileIsFreeInTheOtherButNotTwiceInTheSame(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	first, _ := routedTagInEachRepository(t, f)
	_, err := f.manager.CreateTag(f.ctx, CreateTagRequest{RepositoryID: first.repo.ID, Name: "v1", CommitID: first.commit.ID})
	if !errors.Is(err, ErrTagAlreadyExists) {
		t.Fatalf("second v1 in the same repository = %v, want ErrTagAlreadyExists", err)
	}
}

func TestRoutedGitManager_ATagInTheSecondFileIsDeletedAndTheFirstFilesTagStays(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	first, second := routedTagInEachRepository(t, f)
	if err := f.manager.DeleteTag(f.ctx, second.tag.ID); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	if _, err := f.manager.GetTag(f.ctx, second.tag.ID); !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("GetTag after delete = %v, want ErrTagNotFound", err)
	}
	if tags, err := f.manager.ListTags(f.ctx, second.repo.ID); err != nil || len(tags) != 0 {
		t.Fatalf("ListTags of the second repository after delete = %+v, %v", tags, err)
	}
	if got, err := f.manager.GetTag(f.ctx, first.tag.ID); err != nil || got.ID != first.tag.ID {
		t.Fatalf("GetTag of the first repository's tag = %+v, %v", got, err)
	}
}

func TestRoutedGitManager_ATagInTheFirstFileIsDeletedAndTheSecondFilesTagStays(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	first, second := routedTagInEachRepository(t, f)
	if err := f.manager.DeleteTag(f.ctx, first.tag.ID); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	if _, err := f.manager.GetTag(f.ctx, first.tag.ID); !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("GetTag after delete = %v, want ErrTagNotFound", err)
	}
	if got, err := f.manager.GetTag(f.ctx, second.tag.ID); err != nil || got.ID != second.tag.ID {
		t.Fatalf("GetTag of the second repository's tag = %+v, %v", got, err)
	}
}

func TestRoutedGitManager_AnUnknownTagIdIsNotFoundOnReadAndOnDelete(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	routedTagInEachRepository(t, f)
	if _, err := f.manager.GetTag(f.ctx, "no-such-tag"); !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("GetTag unknown = %v, want ErrTagNotFound", err)
	}
	if err := f.manager.DeleteTag(f.ctx, "no-such-tag"); !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("DeleteTag unknown = %v, want ErrTagNotFound", err)
	}
}

func TestRoutedGitManager_ATagForAnUnknownRepositoryIsRefused(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	if _, err := f.manager.CreateTag(f.ctx, CreateTagRequest{RepositoryID: "no-such-repository", Name: "v1", CommitID: "x"}); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("CreateTag in an unknown repository = %v, want ErrRepoNotInitialised", err)
	}
}
