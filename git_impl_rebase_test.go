package mwanachamagit

import (
	"context"
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

const (
	rebaseAuthorName  = "Ada Writer"
	rebaseAuthorEmail = "ada@example.com"
)

func rebaseCommit(t *testing.T, rf *RepoFile, branchID, path, content, message string) models.Commit {
	t.Helper()
	commit, err := rf.WriteFile(context.Background(), WriteFileRequest{
		BranchID:    branchID,
		Path:        path,
		Content:     content,
		Message:     message,
		AuthorName:  rebaseAuthorName,
		AuthorEmail: rebaseAuthorEmail,
	})
	if err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
	return commit
}

func rebaseLog(t *testing.T, rf *RepoFile, branchID string) []CommitEntry {
	t.Helper()
	entries, err := rf.Log(context.Background(), branchID, LogFilter{})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	return entries
}

func rebaseOnto(t *testing.T, rf *RepoFile, branchID, ontoID string) models.Branch {
	t.Helper()
	branch, err := rf.RebaseBranch(context.Background(), RebaseBranchRequest{BranchID: branchID, OntoBranchID: ontoID})
	if err != nil {
		t.Fatalf("RebaseBranch: %v", err)
	}
	return branch
}

func TestRebaseBranch_CleanRebaseReplaysOwnCommitsOntoDefaultHead(t *testing.T) {
	rf, repo, def := conflictOpen(t, nil)
	conflictWrite(t, rf, def.ID, "base.txt", "base\n")
	feature := conflictBranchFrom(t, rf, repo.ID, def.ID, "feature/a")
	first := rebaseCommit(t, rf, feature.ID, "A.txt", "a one\n", "add A one")
	second := rebaseCommit(t, rf, feature.ID, "A.txt", "a two\n", "add A two")
	defaultTip := rebaseCommit(t, rf, def.ID, "B.txt", "b\n", "add B")
	defaultHeadBefore := conflictHead(t, rf, def.ID)

	rebased := rebaseOnto(t, rf, feature.ID, "")

	if got := conflictRead(t, rf, feature.ID, "A.txt"); got != "a two\n" {
		t.Fatalf("A.txt = %q, want the branch's last content", got)
	}
	if got := conflictRead(t, rf, feature.ID, "B.txt"); got != "b\n" {
		t.Fatalf("B.txt = %q, want the default branch's file", got)
	}
	entries := rebaseLog(t, rf, feature.ID)
	if len(entries) < 4 {
		t.Fatalf("rebased log has %d entries, want at least 4", len(entries))
	}
	if entries[0].Message != "add A two" || entries[1].Message != "add A one" {
		t.Fatalf("replayed messages = %q, %q, want %q then %q", entries[0].Message, entries[1].Message, "add A two", "add A one")
	}
	if entries[2].SHA != defaultTip.SHA {
		t.Fatalf("first replayed commit sits on %s, want the default head %s", entries[2].SHA, defaultTip.SHA)
	}
	if entries[0].SHA == second.SHA || entries[1].SHA == first.SHA {
		t.Fatalf("replayed commits reuse the original SHAs")
	}
	if entries[0].Author != rebaseAuthorName || entries[1].Author != rebaseAuthorName {
		t.Fatalf("replayed authors = %q, %q, want %q", entries[0].Author, entries[1].Author, rebaseAuthorName)
	}
	if rebased.HeadCommitID == second.ID || rebased.HeadCommitID == "" {
		t.Fatalf("returned head %q is not a new commit", rebased.HeadCommitID)
	}
	if got := conflictHead(t, rf, feature.ID); got != rebased.HeadCommitID {
		t.Fatalf("stored head %s differs from returned head %s", got, rebased.HeadCommitID)
	}
	if got := conflictHead(t, rf, def.ID); got != defaultHeadBefore {
		t.Fatalf("default head moved from %s to %s", defaultHeadBefore, got)
	}
}

func TestRebaseBranch_ConflictingRebaseLeavesBranchUntouched(t *testing.T) {
	rf, _, def, feature := conflictDiverged(t, nil)
	headBefore := conflictHead(t, rf, feature.ID)
	logBefore := len(rebaseLog(t, rf, feature.ID))

	_, err := rf.RebaseBranch(context.Background(), RebaseBranchRequest{BranchID: feature.ID})

	files := conflictFiles(t, err)
	found := false
	for _, f := range files {
		if f == conflictPath {
			found = true
		}
	}
	if !found {
		t.Fatalf("ConflictingFiles = %v, want it to name %s", files, conflictPath)
	}
	if got := conflictHead(t, rf, feature.ID); got != headBefore {
		t.Fatalf("branch head moved from %s to %s despite the conflict", headBefore, got)
	}
	if got := len(rebaseLog(t, rf, feature.ID)); got != logBefore {
		t.Fatalf("branch log grew from %d to %d entries despite the conflict", logBefore, got)
	}
	if got := conflictRead(t, rf, feature.ID, conflictPath); got != conflictReplaceLine(conflictBase, 2, "from branch") {
		t.Fatalf("branch content changed by the failed rebase: %q", got)
	}
	if got := conflictRead(t, rf, def.ID, conflictPath); got != conflictReplaceLine(conflictBase, 2, "from default") {
		t.Fatalf("default content changed by the failed rebase: %q", got)
	}
}

func TestRebaseBranch_BranchAlreadyOnDefaultHeadIsNoOp(t *testing.T) {
	rf, repo, def := conflictOpen(t, nil)
	conflictWrite(t, rf, def.ID, "base.txt", "base\n")
	feature := conflictBranchFrom(t, rf, repo.ID, def.ID, "feature/a")
	rebaseCommit(t, rf, feature.ID, "A.txt", "a\n", "add A")
	headBefore := conflictHead(t, rf, feature.ID)

	rebased := rebaseOnto(t, rf, feature.ID, "")

	if rebased.HeadCommitID != headBefore {
		t.Fatalf("returned head %s, want unchanged %s", rebased.HeadCommitID, headBefore)
	}
	if got := conflictHead(t, rf, feature.ID); got != headBefore {
		t.Fatalf("stored head %s, want unchanged %s", got, headBefore)
	}
}

func TestRebaseBranch_SecondRebaseAfterCleanRebaseIsNoOp(t *testing.T) {
	rf, repo, def := conflictOpen(t, nil)
	conflictWrite(t, rf, def.ID, "base.txt", "base\n")
	feature := conflictBranchFrom(t, rf, repo.ID, def.ID, "feature/a")
	rebaseCommit(t, rf, feature.ID, "A.txt", "a\n", "add A")
	rebaseCommit(t, rf, def.ID, "B.txt", "b\n", "add B")

	first := rebaseOnto(t, rf, feature.ID, "")
	logAfterFirst := len(rebaseLog(t, rf, feature.ID))
	second := rebaseOnto(t, rf, feature.ID, "")

	if second.HeadCommitID != first.HeadCommitID {
		t.Fatalf("second rebase moved head from %s to %s", first.HeadCommitID, second.HeadCommitID)
	}
	if got := conflictHead(t, rf, feature.ID); got != first.HeadCommitID {
		t.Fatalf("stored head %s, want %s", got, first.HeadCommitID)
	}
	if got := len(rebaseLog(t, rf, feature.ID)); got != logAfterFirst {
		t.Fatalf("second rebase grew the log from %d to %d", logAfterFirst, got)
	}
}

func TestRebaseBranch_BranchWithoutOwnCommitsFastForwards(t *testing.T) {
	rf, repo, def := conflictOpen(t, nil)
	conflictWrite(t, rf, def.ID, "base.txt", "base\n")
	feature := conflictBranchFrom(t, rf, repo.ID, def.ID, "feature/a")
	rebaseCommit(t, rf, def.ID, "B.txt", "b\n", "add B")
	defaultHead := conflictHead(t, rf, def.ID)
	defaultLog := len(rebaseLog(t, rf, def.ID))

	rebased := rebaseOnto(t, rf, feature.ID, "")

	if rebased.HeadCommitID != defaultHead {
		t.Fatalf("returned head %s, want the default head %s", rebased.HeadCommitID, defaultHead)
	}
	if got := conflictHead(t, rf, feature.ID); got != defaultHead {
		t.Fatalf("stored head %s, want the default head %s", got, defaultHead)
	}
	if got := len(rebaseLog(t, rf, feature.ID)); got != defaultLog {
		t.Fatalf("fast-forwarded log has %d entries, want %d", got, defaultLog)
	}
	if got := conflictRead(t, rf, feature.ID, "B.txt"); got != "b\n" {
		t.Fatalf("B.txt = %q after the fast-forward", got)
	}
}

func TestRebaseBranch_OntoNamedBranchTakesThatBranchsFiles(t *testing.T) {
	rf, repo, def := conflictOpen(t, nil)
	conflictWrite(t, rf, def.ID, "base.txt", "base\n")
	feature := conflictBranchFrom(t, rf, repo.ID, def.ID, "feature/a")
	target := conflictBranchFrom(t, rf, repo.ID, def.ID, "feature/b")
	rebaseCommit(t, rf, feature.ID, "A.txt", "a\n", "add A")
	targetTip := rebaseCommit(t, rf, target.ID, "T.txt", "t\n", "add T")
	targetHeadBefore := conflictHead(t, rf, target.ID)
	defaultHeadBefore := conflictHead(t, rf, def.ID)

	rebaseOnto(t, rf, feature.ID, target.ID)

	if got := conflictRead(t, rf, feature.ID, "T.txt"); got != "t\n" {
		t.Fatalf("T.txt = %q, want the target branch's file", got)
	}
	if got := conflictRead(t, rf, feature.ID, "A.txt"); got != "a\n" {
		t.Fatalf("A.txt = %q, want the branch's own file", got)
	}
	entries := rebaseLog(t, rf, feature.ID)
	if len(entries) < 2 || entries[0].Message != "add A" || entries[1].SHA != targetTip.SHA {
		t.Fatalf("rebased log does not sit on the target head %s: %+v", targetTip.SHA, entries)
	}
	if got := conflictHead(t, rf, target.ID); got != targetHeadBefore {
		t.Fatalf("target head moved from %s to %s", targetHeadBefore, got)
	}
	if got := conflictHead(t, rf, def.ID); got != defaultHeadBefore {
		t.Fatalf("default head moved from %s to %s", defaultHeadBefore, got)
	}
}

func TestRebaseBranch_UnknownBranchReturnsBranchNotFound(t *testing.T) {
	rf, _, _ := conflictOpen(t, nil)

	_, err := rf.RebaseBranch(context.Background(), RebaseBranchRequest{BranchID: "no-such-branch"})

	if !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("RebaseBranch error = %v, want ErrBranchNotFound", err)
	}
}
