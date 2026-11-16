package mwanachamagit

import (
	"context"
	"strings"
	"testing"
)

const (
	tagsMadeOnceRefusal  = "tags are made once; delete one with the file tools"
	tagNotACommitRefusal = "a tag must point at a commit"
)

func tagRowFor(t *testing.T, s fileGitServer, name string) (found bool, sha, message, tagger string) {
	t.Helper()
	tags, err := s.rf.ListTags(context.Background(), s.repo)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	for _, tag := range tags {
		if tag.Name == name {
			return true, tag.SHA, tag.Message, tag.TaggerName
		}
	}
	return false, "", "", ""
}

func TestGitPushTags_ALightweightTagOnANewCommitIsAcceptedListedAndServedToANewClone(t *testing.T) {
	s := newFileGitServer(t)
	work := s.clone(t)
	tip := transportCommit(t, work, "tagged.txt", "x\n", "to be tagged")
	transportGit(t, work, "tag", "v1")

	transportGit(t, work, "push", "origin", "v1")

	found, sha, _, _ := tagRowFor(t, s, "v1")
	if !found || sha != tip {
		t.Fatalf("tag row v1 found=%v sha=%q, want the pushed commit %s", found, sha, tip)
	}
	fresh := s.clone(t)
	transportGit(t, fresh, "fetch", "--tags", "origin")
	if got := transportGit(t, fresh, "tag", "--list"); got != "v1" {
		t.Errorf("tags in a fresh clone = %q, want v1", got)
	}
	if got := transportGit(t, fresh, "rev-parse", "v1"); got != tip {
		t.Errorf("v1 in a fresh clone = %s, want %s", got, tip)
	}
	if got := readCloned(t, fresh, "README.md"); got != "# widgets\n" {
		t.Errorf("README.md = %q", got)
	}
	if got := transportGit(t, fresh, "show", "v1:tagged.txt"); got != "x" {
		t.Errorf("tagged.txt at v1 = %q", got)
	}
}

func TestGitPushTags_ALightweightTagOnTheDefaultBranchTipIsAccepted(t *testing.T) {
	s := newFileGitServer(t)
	work := s.clone(t)
	tip := transportGit(t, work, "rev-parse", "HEAD")
	transportGit(t, work, "tag", "release-1")

	transportGit(t, work, "push", "origin", "release-1")

	found, sha, _, _ := tagRowFor(t, s, "release-1")
	if !found || sha != tip {
		t.Errorf("tag row found=%v sha=%q, want %s", found, sha, tip)
	}
}

func TestGitPushTags_AnAnnotatedTagIsAcceptedAndKeepsItsMessageAndTaggerOnTheRow(t *testing.T) {
	s := newFileGitServer(t)
	work := s.clone(t)
	tip := transportCommit(t, work, "tagged.txt", "x\n", "to be tagged")
	transportGit(t, work, "tag", "-a", "v2", "-m", "release two")

	transportGit(t, work, "push", "origin", "v2")

	found, sha, message, tagger := tagRowFor(t, s, "v2")
	if !found || sha != tip {
		t.Fatalf("tag row v2 found=%v sha=%q, want %s", found, sha, tip)
	}
	if message != "release two" || tagger != "Transport Test" {
		t.Errorf("message=%q tagger=%q", message, tagger)
	}
	fresh := s.clone(t)
	transportGit(t, fresh, "fetch", "--tags", "origin")
	if got := transportGit(t, fresh, "rev-parse", "v2"); got != tip {
		t.Errorf("v2 in a fresh clone = %s, want the commit %s", got, tip)
	}
}

func TestGitPushTags_ATagAndABranchInOnePushAreBothAccepted(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()
	work := s.clone(t)
	transportGit(t, work, "checkout", "-b", "topic")
	tip := transportCommit(t, work, "a.txt", "alpha\n", "alpha")
	transportGit(t, work, "tag", "v3")

	transportGit(t, work, "push", "origin", "topic", "v3")

	branch, err := s.rf.GetBranchByName(ctx, s.repo, "topic")
	if err != nil || branch.SHA != tip {
		t.Errorf("topic = %v, %v", branch.SHA, err)
	}
	if found, _, _, _ := tagRowFor(t, s, "v3"); !found {
		t.Error("v3 has no row")
	}
}

func TestGitPushTags_ATagThatAlreadyExistsCannotBeMovedAndKeepsItsCommit(t *testing.T) {
	s := newFileGitServer(t)
	work := s.clone(t)
	first := transportCommit(t, work, "one.txt", "1\n", "one")
	transportGit(t, work, "tag", "v1")
	transportGit(t, work, "push", "origin", "v1")
	transportCommit(t, work, "two.txt", "2\n", "two")
	transportGit(t, work, "tag", "-f", "v1")

	out, err := transportGitMayFail(work, "push", "--force", "origin", "v1")

	if err == nil {
		t.Fatalf("moving a tag succeeded:\n%s", out)
	}
	if !strings.Contains(out, tagsMadeOnceRefusal) {
		t.Errorf("the refusal did not carry the plain message:\n%s", out)
	}
	if _, sha, _, _ := tagRowFor(t, s, "v1"); sha != first {
		t.Errorf("v1 = %s, want it left at %s", sha, first)
	}
}

func TestGitPushTags_ATagMadeWithTheToolsCannotBeCreatedAgainByPush(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()
	main, err := s.rf.GetBranch(ctx, s.main)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.rf.CreateTag(ctx, CreateTagRequest{RepositoryID: s.repo, Name: "v9", CommitID: main.HeadCommitID}); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	work := s.clone(t)
	transportCommit(t, work, "other.txt", "o\n", "other")
	transportGit(t, work, "tag", "-f", "v9")

	out, err := transportGitMayFail(work, "push", "--force", "origin", "v9")

	if err == nil {
		t.Fatalf("a tag push over an existing name succeeded:\n%s", out)
	}
	if !strings.Contains(out, tagsMadeOnceRefusal) {
		t.Errorf("the refusal did not carry the plain message:\n%s", out)
	}
	if _, sha, _, _ := tagRowFor(t, s, "v9"); sha != main.SHA {
		t.Errorf("v9 = %s, want %s", sha, main.SHA)
	}
}

func TestGitPushTags_DeletingATagByPushIsRefusedAndTheTagStays(t *testing.T) {
	s := newFileGitServer(t)
	work := s.clone(t)
	transportCommit(t, work, "one.txt", "1\n", "one")
	transportGit(t, work, "tag", "v1")
	transportGit(t, work, "push", "origin", "v1")

	out, err := transportGitMayFail(work, "push", "origin", ":refs/tags/v1")

	if err == nil {
		t.Fatalf("deleting a tag succeeded:\n%s", out)
	}
	if !strings.Contains(out, tagsMadeOnceRefusal) {
		t.Errorf("the refusal did not carry the plain message:\n%s", out)
	}
	if found, _, _, _ := tagRowFor(t, s, "v1"); !found {
		t.Error("v1 was deleted")
	}
}

func TestGitPushTags_ATagOnATreeIsRefusedBecauseATagMustPointAtACommit(t *testing.T) {
	s := newFileGitServer(t)
	work := s.clone(t)
	transportGit(t, work, "tag", "tree-tag", "HEAD^{tree}")

	out, err := transportGitMayFail(work, "push", "origin", "tree-tag")

	if err == nil {
		t.Fatalf("a tag on a tree was accepted:\n%s", out)
	}
	if !strings.Contains(out, tagNotACommitRefusal) {
		t.Errorf("the refusal did not carry the plain message:\n%s", out)
	}
	if found, _, _, _ := tagRowFor(t, s, "tree-tag"); found {
		t.Error("a row was filed for a tag on a tree")
	}
}

func TestGitPushTags_ARefOutsideHeadsAndTagsIsStillRefusedWithThePlainMessage(t *testing.T) {
	s := newFileGitServer(t)
	work := s.clone(t)
	tip := transportCommit(t, work, "n.txt", "n\n", "note")

	out, err := transportGitMayFail(work, "push", "origin", tip+":refs/notes/review")

	if err == nil {
		t.Fatalf("a push to refs/notes succeeded:\n%s", out)
	}
	if !strings.Contains(out, notABranchRefusal) {
		t.Errorf("the refusal did not carry the plain message:\n%s", out)
	}
}
