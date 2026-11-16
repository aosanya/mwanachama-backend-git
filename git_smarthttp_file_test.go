package mwanachamagit

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fileGitServer struct {
	rf   *RepoFile
	repo string
	main string
	url  string
}

func newFileGitServer(t *testing.T) fileGitServer {
	t.Helper()
	transportRequireGit(t)
	rf, repo, mainBranch := newFeatureRepo(t)
	writeFeatureFiles(t, rf, mainBranch.ID, []featureFile{
		{Path: "README.md", Content: "# widgets\n"},
		{Path: "docs/guide.md", Content: "read me first\n"},
	})
	srv := httptest.NewServer(rf.GitHTTPHandler())
	t.Cleanup(srv.Close)
	return fileGitServer{rf: rf, repo: repo.ID, main: mainBranch.ID, url: srv.URL}
}

func (s fileGitServer) clone(t *testing.T, args ...string) string {
	t.Helper()
	parent := t.TempDir()
	transportGit(t, parent, append(append([]string{"clone"}, args...), s.url, "copy")...)
	return filepath.Join(parent, "copy")
}

func readCloned(t *testing.T, dir, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil {
		t.Fatalf("read cloned %s: %v", path, err)
	}
	return string(data)
}

func TestRepoFileGitHTTP_CloneCommitPushNewBranchCloneItAgain(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()

	first := s.clone(t)
	if got := readCloned(t, first, "README.md"); got != "# widgets\n" {
		t.Errorf("cloned README.md = %q", got)
	}
	if got := readCloned(t, first, "docs/guide.md"); got != "read me first\n" {
		t.Errorf("cloned docs/guide.md = %q", got)
	}

	transportGit(t, first, "checkout", "-b", "topic")
	transportCommit(t, first, "docs/extra.md", "extra\n", "add extra")
	tip := transportCommit(t, first, "src/app.txt", "app\n", "add app")
	transportGit(t, first, "push", "origin", "topic")

	topic, err := s.rf.GetBranchByName(ctx, s.repo, "topic")
	if err != nil {
		t.Fatalf("GetBranchByName topic: %v", err)
	}
	if topic.SHA != tip {
		t.Errorf("topic tip = %q, want %q", topic.SHA, tip)
	}

	second := s.clone(t, "--branch", "topic")
	if got := transportGit(t, second, "rev-parse", "HEAD"); got != tip {
		t.Errorf("second clone HEAD = %q, want %q", got, tip)
	}
	if got := transportGit(t, second, "log", "--format=%s"); got != "add app\nadd extra\nUpdate docs/guide.md\nUpdate README.md" {
		t.Errorf("second clone log = %q", got)
	}
	for path, want := range map[string]string{
		"README.md":     "# widgets\n",
		"docs/guide.md": "read me first\n",
		"docs/extra.md": "extra\n",
		"src/app.txt":   "app\n",
	} {
		if got := readCloned(t, second, path); got != want {
			t.Errorf("second clone %s = %q, want %q", path, got, want)
		}
	}

	blob, err := s.rf.ReadFile(ctx, topic.ID, "src/app.txt")
	if err != nil {
		t.Fatalf("ReadFile on the pushed branch: %v", err)
	}
	if blob.Content != "app\n" {
		t.Errorf("pushed file through the tools = %q", blob.Content)
	}
}

func TestRepoFileGitHTTP_PushedBranchSurvivesAToolWriteAndIsCloneable(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()

	work := s.clone(t)
	transportGit(t, work, "checkout", "-b", "topic")
	transportCommit(t, work, "a.txt", "alpha\n", "alpha")
	transportGit(t, work, "push", "origin", "topic")

	topic, err := s.rf.GetBranchByName(ctx, s.repo, "topic")
	if err != nil {
		t.Fatalf("GetBranchByName topic: %v", err)
	}
	written, err := s.rf.WriteFile(ctx, WriteFileRequest{BranchID: topic.ID, Path: "b.txt", Content: "beta\n"})
	if err != nil {
		t.Fatalf("WriteFile on the pushed branch: %v", err)
	}

	again := s.clone(t, "--branch", "topic")
	if got := transportGit(t, again, "rev-parse", "HEAD"); got != written.SHA {
		t.Errorf("clone HEAD = %q, want the tool commit %q", got, written.SHA)
	}
	for path, want := range map[string]string{"a.txt": "alpha\n", "b.txt": "beta\n", "README.md": "# widgets\n"} {
		if got := readCloned(t, again, path); got != want {
			t.Errorf("clone %s = %q, want %q", path, got, want)
		}
	}
}

func TestRepoFileGitHTTP_PushToTheDefaultBranchIsRefusedWithAPlainMessage(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()
	before, err := s.rf.GetBranch(ctx, s.main)
	if err != nil {
		t.Fatalf("GetBranch: %v", err)
	}

	work := s.clone(t)
	transportCommit(t, work, "sneaky.txt", "x\n", "straight to main")
	out, err := transportGitMayFail(work, "push", "origin", "main")
	if err == nil {
		t.Fatalf("push to the default branch succeeded:\n%s", out)
	}
	if !strings.Contains(out, "push to a merge request, not to the default branch") {
		t.Errorf("refusal did not carry the plain message:\n%s", out)
	}

	after, err := s.rf.GetBranch(ctx, s.main)
	if err != nil {
		t.Fatalf("GetBranch: %v", err)
	}
	if after.SHA != before.SHA {
		t.Errorf("default branch moved from %s to %s", before.SHA, after.SHA)
	}
	fresh := s.clone(t)
	if _, err := os.Stat(filepath.Join(fresh, "sneaky.txt")); err == nil {
		t.Error("the refused commit is visible in a new clone")
	}
}

func TestRepoFileGitHTTP_DeletingAPushedBranchWorksAndTheDefaultBranchCannotBeDeleted(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()

	work := s.clone(t)
	transportGit(t, work, "checkout", "-b", "topic")
	transportCommit(t, work, "a.txt", "alpha\n", "alpha")
	transportGit(t, work, "push", "origin", "topic")

	transportGit(t, work, "push", "origin", "--delete", "topic")
	if _, err := s.rf.GetBranchByName(ctx, s.repo, "topic"); err == nil {
		t.Error("topic still resolvable after a delete push")
	}

	out, err := transportGitMayFail(work, "push", "origin", "--delete", "main")
	if err == nil {
		t.Fatalf("deleting the default branch succeeded:\n%s", out)
	}
	if _, err := s.rf.GetBranch(ctx, s.main); err != nil {
		t.Errorf("default branch lost after a refused delete: %v", err)
	}
}
