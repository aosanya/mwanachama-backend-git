package mwanachamagit

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm"
)

func failBlobWrites(t *testing.T, m *gitManager) {
	t.Helper()
	statement := fmt.Sprintf("CREATE TRIGGER failblob BEFORE UPDATE ON `%s` BEGIN SELECT RAISE(ABORT,'disk full'); END", m.tables.Blobs)
	if err := m.db.Exec(statement).Error; err != nil {
		t.Fatal(err)
	}
}

func commitRowCount(t *testing.T, m *gitManager, sha string) int64 {
	t.Helper()
	var count int64
	if err := m.db.Table(m.tables.Commits).Where("sha = ?", sha).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func TestAtomicPush_ANewBranchWhoseBlobsCannotBeStoredLeavesNoBranchNoCommitAndNoRef(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()
	m := transportManager(t, s.rf)
	work := s.clone(t)
	transportGit(t, work, "checkout", "-b", "topic")
	tip := transportCommit(t, work, "a.txt", "alpha\n", "alpha")
	failBlobWrites(t, m)

	out, err := transportGitMayFail(work, "push", "origin", "topic")

	if err == nil {
		t.Fatalf("push succeeded: %s", out)
	}
	if _, err := s.rf.GetBranchByName(ctx, s.repo, "topic"); err == nil {
		t.Errorf("a branch row exists for the rejected push")
	}
	if n := commitRowCount(t, m, tip); n != 0 {
		t.Errorf("%d commit row(s) left for the rejected push", n)
	}
	refs := transportGit(t, work, "ls-remote", "origin")
	if strings.Contains(refs, "refs/heads/topic") {
		t.Errorf("ls-remote lists the rejected branch:\n%s", refs)
	}
	if !strings.Contains(refs, "refs/heads/main") {
		t.Errorf("ls-remote lost main:\n%s", refs)
	}
}

func TestAtomicPush_AnExistingBranchWhoseNextPushCannotBeStoredStaysWhereItWas(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()
	m := transportManager(t, s.rf)
	work, pushed := pushedTopic(t, s)
	next := transportCommit(t, work, "b.txt", "beta\n", "beta")
	failBlobWrites(t, m)

	out, err := transportGitMayFail(work, "push", "origin", "topic")

	if err == nil {
		t.Fatalf("push succeeded: %s", out)
	}
	topic, err := s.rf.GetBranchByName(ctx, s.repo, "topic")
	if err != nil {
		t.Fatal(err)
	}
	if topic.SHA != pushed {
		t.Errorf("topic = %s, want %s", topic.SHA, pushed)
	}
	file, err := s.rf.ReadFile(ctx, topic.ID, "a.txt")
	if err != nil || file.Content != "alpha\n" {
		t.Errorf("a.txt = %q, %v", file.Content, err)
	}
	if n := commitRowCount(t, m, next); n != 0 {
		t.Errorf("%d commit row(s) left for the rejected push", n)
	}
	refs := transportGit(t, work, "ls-remote", "origin")
	if !strings.Contains(refs, pushed+"\trefs/heads/topic") {
		t.Errorf("ls-remote does not list topic at %s:\n%s", pushed, refs)
	}
}

func TestAtomicPush_ARefThatFailsDoesNotUndoTheRefBeforeIt(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()
	m := transportManager(t, s.rf)
	work := s.clone(t)
	transportGit(t, work, "checkout", "-b", "good")
	good := transportCommit(t, work, "g.txt", "good\n", "good")
	transportGit(t, work, "push", "origin", "good")
	transportGit(t, work, "checkout", "-b", "bad")
	transportCommit(t, work, "x.txt", "bad\n", "bad")
	failBlobWrites(t, m)

	_, err := transportGitMayFail(work, "push", "origin", "bad")

	if err == nil {
		t.Fatalf("push of bad succeeded")
	}
	branch, err := s.rf.GetBranchByName(ctx, s.repo, "good")
	if err != nil || branch.SHA != good {
		t.Errorf("good = %q, %v; want %s", branch.SHA, err, good)
	}
}

func TestAtomicPush_AClientThatLeavesWhileTheHubIndexesDoesNotLeaveAHalfRecordedPush(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()
	m := transportManager(t, s.rf)
	work := s.clone(t)
	transportGit(t, work, "checkout", "-b", "topic")
	tip := transportCommit(t, work, "a.txt", "alpha\n", "alpha")
	mainTip := transportGit(t, work, "rev-parse", "origin/main")
	body := receivePackBody(t, work, mainTip, pushCommand{"refs/heads/topic", transportZeroSHA, tip})

	requestCtx, leave := context.WithCancel(ctx)
	defer leave()
	var once sync.Once
	callbacks := m.db.Callback().Create()
	if err := callbacks.Before("gorm:create").Register("client_leaves", func(*gorm.DB) { once.Do(leave) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = callbacks.Remove("client_leaves") })

	req := httptest.NewRequest("POST", "/git-receive-pack", body).WithContext(requestCtx)
	rec := httptest.NewRecorder()
	s.rf.GitHTTPHandler().ServeHTTP(rec, req)

	statuses := reportStatuses(t, rec.Code, rec.Result().Status, rec.Body)
	if statuses["refs/heads/topic"] != "ok" {
		t.Fatalf("a client leaving mid-index made the push fail: %v", statuses)
	}
	topic, err := s.rf.GetBranchByName(ctx, s.repo, "topic")
	if err != nil || topic.SHA != tip {
		t.Fatalf("branch = %q, %v; want %s", topic.SHA, err, tip)
	}
	file, err := s.rf.ReadFile(ctx, topic.ID, "a.txt")
	if err != nil || file.Content != "alpha\n" {
		t.Errorf("a.txt = %q, %v", file.Content, err)
	}
}
