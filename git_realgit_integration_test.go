//go:build integration

// git_realgit_integration_test.go drives the real `git` binary against
// [GitManager.SmartHTTPHandler] over HTTP — actual clone/fetch/pull/push/
// rebase traffic, not go-git talking to itself.
//
// That independence is the point of this file. git_smarthttp_test.go drives
// go-git's client against go-git's server, so both ends share one
// implementation of the wire protocol and agree with each other about
// anything they get wrong together. Real git is a second, independent
// implementation, and it found two defects on its first run that the go-git
// tests could not see: a bare repo whose HEAD stayed at refs/heads/master
// however the branch was actually named, which made every `git clone` come
// out with an empty working tree, and a pushed tag filed as a Branch row
// literally named "refs/tags/v1.0.0".
//
// Each test runs a real git operation and then asserts what GitManager's own
// API reports about it, so one run covers both halves — the protocol on the
// wire and the rows it indexes.
//
// Tagged `integration` alongside postgres_integration_test.go, and skipped
// when no git binary is on PATH. It needs no POSTGRES_URL: the manager is
// the usual in-memory sqlite one, since what is under test here is the
// protocol and the indexing, not the storage engine.
package mwanachamagit

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/gormstore"
)

// ── Harness ───────────────────────────────────────────────────────────────────

// requireGit skips the calling test when there is no git binary to drive.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git binary on PATH — this file tests the real client")
	}
}

// runGit runs git in dir and returns its combined output. Config is pinned to
// the command itself rather than read from the machine: a developer's global
// gitconfig (a default branch name, commit.gpgsign, an insteadOf rewrite)
// would otherwise change what these tests actually exercise.
func runGit(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=Integration Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Integration Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_TERMINAL_PROMPT=0",
	)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// mustGit is runGit, failing the test on a non-zero exit.
func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := runGit(t, dir, args...)
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

// newGitServer starts SmartHTTPHandler over HTTP and returns the manager
// behind it with the base URL to push to. Bare clones land under
// pushClonesRoot, which is deliberately persistent in production, so they are
// removed on cleanup here rather than left behind by every run.
func newGitServer(t *testing.T) (*gitManager, string) {
	t.Helper()
	requireGit(t)
	m := newTestManager(t)
	srv := httptest.NewServer(m.SmartHTTPHandler())
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		var rows []gormstore.RepositoryRow
		if err := m.db.Table(m.tables.Repositories).Find(&rows).Error; err != nil {
			return
		}
		for _, r := range rows {
			if r.BareClonePath != "" && strings.HasPrefix(r.BareClonePath, pushClonesRoot()) {
				_ = os.RemoveAll(r.BareClonePath)
			}
		}
	})
	return m, srv.URL
}

// newWorkingCopy initialises a local repo on branch `main` with origin
// pointing at remoteURL.
func newWorkingCopy(t *testing.T, remoteURL string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "wc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir working copy: %v", err)
	}
	mustGit(t, dir, "init", "-b", "main")
	mustGit(t, dir, "remote", "add", "origin", remoteURL)
	return dir
}

// cloneInto clones remoteURL with the real client and returns the clone's path.
func cloneInto(t *testing.T, remoteURL, name string) string {
	t.Helper()
	parent := t.TempDir()
	mustGit(t, parent, "clone", remoteURL, name)
	return filepath.Join(parent, name)
}

// commitFile writes path=content, commits it, and returns the new SHA.
func commitFile(t *testing.T, dir, path, content, message string) string {
	t.Helper()
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	mustGit(t, dir, "add", path)
	mustGit(t, dir, "commit", "-m", message)
	return mustGit(t, dir, "rev-parse", "HEAD")
}

// branchRow returns the indexed Branch row for name, failing if absent.
func branchRow(t *testing.T, m *gitManager, repoName, name string) (repoID, branchID, sha string) {
	t.Helper()
	ctx := context.Background()
	repo, err := m.GetRepositoryByName(ctx, repoName)
	if err != nil {
		t.Fatalf("GetRepositoryByName(%q): %v", repoName, err)
	}
	branches, err := m.ListBranches(ctx, repo.ID)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	for _, b := range branches {
		if b.Name == name {
			return repo.ID, b.ID, b.SHA
		}
	}
	t.Fatalf("no indexed branch %q; have %+v", name, branches)
	return "", "", ""
}

// logMessages returns the branch's commit messages newest-first.
func logMessages(t *testing.T, m *gitManager, branchID string) []string {
	t.Helper()
	entries, err := m.Log(context.Background(), branchID, LogFilter{})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	msgs := make([]string, 0, len(entries))
	for _, e := range entries {
		msgs = append(msgs, strings.TrimSpace(e.Message))
	}
	return msgs
}

// ── Push ──────────────────────────────────────────────────────────────────────

// TestRealGit_FirstPushCreatesRepositoryAndIndexesIt covers the whole
// first-contact path: a client pushing to a name the server has never heard
// of gets the repository created for it, the way a real git host behaves.
func TestRealGit_FirstPushCreatesRepositoryAndIndexesIt(t *testing.T) {
	m, base := newGitServer(t)
	wc := newWorkingCopy(t, base+"/widgets")
	sha := commitFile(t, wc, "README.md", "# widgets\n", "initial commit")
	mustGit(t, wc, "push", "-u", "origin", "main")

	ctx := context.Background()
	repo, err := m.GetRepositoryByName(ctx, "widgets")
	if err != nil {
		t.Fatalf("repository should have been created by the push: %v", err)
	}
	_, branchID, gotSHA := branchRow(t, m, "widgets", "main")
	if gotSHA != sha {
		t.Errorf("indexed branch tip = %q, want the pushed SHA %q", gotSHA, sha)
	}

	blob, err := m.ReadFile(ctx, branchID, "README.md")
	if err != nil {
		t.Fatalf("ReadFile(README.md) after push: %v", err)
	}
	if blob.Path != "README.md" {
		t.Errorf("indexed blob path = %q, want %q", blob.Path, "README.md")
	}
	if repo.DefaultBranch != "main" {
		t.Errorf("repository default branch = %q, want %q", repo.DefaultBranch, "main")
	}
}

// TestRealGit_RepeatedPushOfSameCommitsIsIdempotent re-pushes an unchanged
// branch and confirms nothing is duplicated — the property the whole
// upsert-rather-than-create design in git_impl_push.go exists for, here
// through the real client rather than a direct call.
func TestRealGit_RepeatedPushOfSameCommitsIsIdempotent(t *testing.T) {
	m, base := newGitServer(t)
	wc := newWorkingCopy(t, base+"/widgets")
	commitFile(t, wc, "a.txt", "A\n", "add a")
	mustGit(t, wc, "push", "-u", "origin", "main")

	countRows := func() (commits, blobs, trees int64) {
		ctx := context.Background()
		m.db.WithContext(ctx).Table(m.tables.Commits).Count(&commits)
		m.db.WithContext(ctx).Table(m.tables.Blobs).Count(&blobs)
		m.db.WithContext(ctx).Table(m.tables.Trees).Count(&trees)
		return
	}
	c1, b1, t1 := countRows()

	// Pushing again with nothing new to send must not re-index anything.
	mustGit(t, wc, "push", "origin", "main")
	commitFile(t, wc, "b.txt", "B\n", "add b")
	mustGit(t, wc, "push", "origin", "main")
	c2, b2, t2 := countRows()

	if c2 != c1+1 {
		t.Errorf("Commit rows went %d → %d across a no-op push and one new commit, want exactly one more", c1, c2)
	}
	if b2 != b1+1 {
		t.Errorf("Blob rows went %d → %d, want exactly one more (a.txt must not be re-created)", b1, b2)
	}
	if t2 <= t1 {
		t.Errorf("Tree rows went %d → %d, want the new commit's tree added", t1, t2)
	}
}

// TestRealGit_NestedDirectoriesIndexWithFullPaths confirms a pushed directory
// tree is indexed at its real paths, not flattened to basenames.
func TestRealGit_NestedDirectoriesIndexWithFullPaths(t *testing.T) {
	m, base := newGitServer(t)
	wc := newWorkingCopy(t, base+"/widgets")
	paths := []string{"top.txt", "src/main.go", "src/inner/deep.txt", "docs/guide/intro.md"}
	for i, p := range paths {
		commitFile(t, wc, p, "content "+p+"\n", "add "+p)
		_ = i
	}
	mustGit(t, wc, "push", "-u", "origin", "main")

	_, branchID, _ := branchRow(t, m, "widgets", "main")
	for _, p := range paths {
		blob, err := m.ReadFile(context.Background(), branchID, p)
		if err != nil {
			t.Errorf("ReadFile(%q): %v", p, err)
			continue
		}
		if blob.Path != p {
			t.Errorf("blob for %q indexed at path %q", p, blob.Path)
		}
	}
}

// TestRealGit_IdenticalContentAtTwoPathsKeepsBoth is the real-client form of
// the (sha, path) keying fix: two byte-identical files share one blob SHA and
// must still be two readable files.
func TestRealGit_IdenticalContentAtTwoPathsKeepsBoth(t *testing.T) {
	m, base := newGitServer(t)
	wc := newWorkingCopy(t, base+"/widgets")
	same := "identical bytes\n"
	if err := os.MkdirAll(filepath.Join(wc, "vendor"), 0o755); err != nil {
		t.Fatalf("mkdir vendor: %v", err)
	}
	for _, p := range []string{"LICENSE", "vendor/LICENSE"} {
		if err := os.WriteFile(filepath.Join(wc, p), []byte(same), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
		mustGit(t, wc, "add", p)
	}
	mustGit(t, wc, "commit", "-m", "two identical licences")
	mustGit(t, wc, "push", "-u", "origin", "main")

	_, branchID, _ := branchRow(t, m, "widgets", "main")
	for _, p := range []string{"LICENSE", "vendor/LICENSE"} {
		if _, err := m.ReadFile(context.Background(), branchID, p); err != nil {
			t.Errorf("ReadFile(%q): %v — identical content must not collapse two paths into one row", p, err)
		}
	}
}

// ── Clone ─────────────────────────────────────────────────────────────────────

// TestRealGit_CloneChecksOutTheWorkingTree is the regression test for the
// dangling-HEAD defect real git found: the server's bare repo kept HEAD at
// refs/heads/master whatever branch was pushed, so `git clone` reported
// "remote HEAD refers to nonexistent ref", checked nothing out, and left the
// caller a directory containing only .git.
func TestRealGit_CloneChecksOutTheWorkingTree(t *testing.T) {
	_, base := newGitServer(t)
	wc := newWorkingCopy(t, base+"/widgets")
	commitFile(t, wc, "README.md", "# widgets\n", "initial commit")
	commitFile(t, wc, "src/app.go", "package main\n", "add source")
	mustGit(t, wc, "push", "-u", "origin", "main")

	clone := cloneInto(t, base+"/widgets", "fresh")

	for _, p := range []string{"README.md", "src/app.go"} {
		if _, err := os.Stat(filepath.Join(clone, p)); err != nil {
			t.Errorf("clone is missing %s — the working tree was not checked out: %v", p, err)
		}
	}
	if branch := mustGit(t, clone, "rev-parse", "--abbrev-ref", "HEAD"); branch != "main" {
		t.Errorf("clone checked out %q, want the pushed branch %q", branch, "main")
	}
	if remoteHead := mustGit(t, clone, "rev-parse", "HEAD"); remoteHead != mustGit(t, wc, "rev-parse", "HEAD") {
		t.Error("clone HEAD does not match the pushed tip")
	}
}

// TestRealGit_CloneOfNeverPushedRepositorySucceedsEmpty covers first contact
// from a reader rather than a writer: the loader auto-creates the repository,
// and the clone must succeed as an ordinary empty one rather than erroring.
func TestRealGit_CloneOfNeverPushedRepositorySucceedsEmpty(t *testing.T) {
	_, base := newGitServer(t)
	parent := t.TempDir()
	out, err := runGit(t, parent, "clone", base+"/never-pushed", "empty")
	if err != nil {
		t.Fatalf("clone of an empty repository should succeed: %v\n%s", err, out)
	}
	entries, err := os.ReadDir(filepath.Join(parent, "empty"))
	if err != nil {
		t.Fatalf("read clone dir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != ".git" {
			t.Errorf("empty clone unexpectedly contains %q", e.Name())
		}
	}
}

// ── Fetch and pull ────────────────────────────────────────────────────────────

// TestRealGit_PullPropagatesAnotherClonesPush is the two-developer case: work
// pushed from one clone must arrive in another through a plain `git pull`.
func TestRealGit_PullPropagatesAnotherClonesPush(t *testing.T) {
	m, base := newGitServer(t)
	remote := base + "/widgets"
	first := newWorkingCopy(t, remote)
	commitFile(t, first, "a.txt", "A\n", "add a")
	mustGit(t, first, "push", "-u", "origin", "main")

	second := cloneInto(t, remote, "second")
	commitFile(t, second, "b.txt", "B\n", "add b from the second clone")
	mustGit(t, second, "push", "origin", "main")

	mustGit(t, first, "pull", "origin", "main")
	if _, err := os.Stat(filepath.Join(first, "b.txt")); err != nil {
		t.Errorf("pull did not bring b.txt into the first working copy: %v", err)
	}

	_, branchID, _ := branchRow(t, m, "widgets", "main")
	msgs := logMessages(t, m, branchID)
	if len(msgs) != 2 || msgs[0] != "add b from the second clone" {
		t.Errorf("indexed history = %v, want both commits newest-first", msgs)
	}
}

// TestRealGit_FetchUpdatesRemoteRefsOnly confirms fetch is served correctly
// and, unlike pull, leaves the working tree alone.
func TestRealGit_FetchUpdatesRemoteRefsOnly(t *testing.T) {
	_, base := newGitServer(t)
	remote := base + "/widgets"
	first := newWorkingCopy(t, remote)
	commitFile(t, first, "a.txt", "A\n", "add a")
	mustGit(t, first, "push", "-u", "origin", "main")

	second := cloneInto(t, remote, "second")
	commitFile(t, second, "b.txt", "B\n", "add b")
	mustGit(t, second, "push", "origin", "main")

	mustGit(t, first, "fetch", "origin")
	if _, err := os.Stat(filepath.Join(first, "b.txt")); !os.IsNotExist(err) {
		t.Error("fetch must not touch the working tree, but b.txt appeared")
	}
	if fetched := mustGit(t, first, "rev-parse", "origin/main"); fetched != mustGit(t, second, "rev-parse", "HEAD") {
		t.Error("origin/main was not advanced by the fetch")
	}
}

// ── Rebase, force-push, and rejection ────────────────────────────────────────

// TestRealGit_NonFastForwardPushIsRejected confirms the server refuses a
// divergent push and, just as important, that the refusal leaves the indexed
// branch where it was.
func TestRealGit_NonFastForwardPushIsRejected(t *testing.T) {
	m, base := newGitServer(t)
	remote := base + "/widgets"
	first := newWorkingCopy(t, remote)
	commitFile(t, first, "a.txt", "A\n", "add a")
	mustGit(t, first, "push", "-u", "origin", "main")

	second := cloneInto(t, remote, "second")

	// Both clones commit on top of the same base, and the first one wins.
	commitFile(t, first, "from-first.txt", "1\n", "from the first clone")
	mustGit(t, first, "push", "origin", "main")
	acceptedSHA := mustGit(t, first, "rev-parse", "HEAD")

	commitFile(t, second, "from-second.txt", "2\n", "from the second clone")
	out, err := runGit(t, second, "push", "origin", "main")
	if err == nil {
		t.Fatalf("a non-fast-forward push must be rejected, but it succeeded:\n%s", out)
	}

	_, _, sha := branchRow(t, m, "widgets", "main")
	if sha != acceptedSHA {
		t.Errorf("indexed branch tip = %q after a rejected push, want the accepted push's %q", sha, acceptedSHA)
	}
}

// TestRealGit_RebaseOntoRemoteThenPush is the ordinary way out of the
// rejection above: fetch, rebase onto the remote tip, push. The result must
// be a linear history the server has indexed in full.
func TestRealGit_RebaseOntoRemoteThenPush(t *testing.T) {
	m, base := newGitServer(t)
	remote := base + "/widgets"
	first := newWorkingCopy(t, remote)
	commitFile(t, first, "a.txt", "A\n", "base commit")
	mustGit(t, first, "push", "-u", "origin", "main")

	second := cloneInto(t, remote, "second")

	commitFile(t, first, "from-first.txt", "1\n", "from the first clone")
	mustGit(t, first, "push", "origin", "main")

	commitFile(t, second, "from-second.txt", "2\n", "from the second clone")
	mustGit(t, second, "fetch", "origin")
	mustGit(t, second, "rebase", "origin/main")
	mustGit(t, second, "push", "origin", "main")

	_, branchID, sha := branchRow(t, m, "widgets", "main")
	if want := mustGit(t, second, "rev-parse", "HEAD"); sha != want {
		t.Errorf("indexed tip = %q after the rebase push, want %q", sha, want)
	}
	msgs := logMessages(t, m, branchID)
	want := []string{"from the second clone", "from the first clone", "base commit"}
	if len(msgs) != len(want) {
		t.Fatalf("indexed history = %v, want the rebased chain %v", msgs, want)
	}
	for i := range want {
		if msgs[i] != want[i] {
			t.Errorf("history[%d] = %q, want %q (full: %v)", i, msgs[i], want[i], msgs)
		}
	}
}

// TestRealGit_ForcePushRewritesTheBranch covers a deliberately rewritten
// history: after an interactive-style amend of an already-pushed commit, a
// plain push must be refused and --force must be honoured, with the indexed
// branch following the rewrite.
func TestRealGit_ForcePushRewritesTheBranch(t *testing.T) {
	m, base := newGitServer(t)
	wc := newWorkingCopy(t, base+"/widgets")
	commitFile(t, wc, "a.txt", "A\n", "base commit")
	commitFile(t, wc, "b.txt", "B\n", "message to be rewritten")
	mustGit(t, wc, "push", "-u", "origin", "main")
	original := mustGit(t, wc, "rev-parse", "HEAD")

	mustGit(t, wc, "commit", "--amend", "-m", "rewritten message")
	rewritten := mustGit(t, wc, "rev-parse", "HEAD")
	if rewritten == original {
		t.Fatal("amend did not rewrite the commit")
	}

	if out, err := runGit(t, wc, "push", "origin", "main"); err == nil {
		t.Fatalf("pushing a rewritten history without --force must be rejected:\n%s", out)
	}
	mustGit(t, wc, "push", "--force", "origin", "main")

	_, branchID, sha := branchRow(t, m, "widgets", "main")
	if sha != rewritten {
		t.Errorf("indexed tip = %q after the force push, want the rewritten %q", sha, rewritten)
	}
	if msgs := logMessages(t, m, branchID); len(msgs) == 0 || msgs[0] != "rewritten message" {
		t.Errorf("indexed history head = %v, want the rewritten message first", msgs)
	}
}

// ── Branches, merges, tags ───────────────────────────────────────────────────

// TestRealGit_FeatureBranchPushAndDelete walks a branch through its whole
// life over the wire: pushed into existence, indexed, then removed by
// `git push --delete` (which carries no packfile and takes its own path
// through the handler).
func TestRealGit_FeatureBranchPushAndDelete(t *testing.T) {
	m, base := newGitServer(t)
	wc := newWorkingCopy(t, base+"/widgets")
	commitFile(t, wc, "a.txt", "A\n", "base commit")
	mustGit(t, wc, "push", "-u", "origin", "main")

	mustGit(t, wc, "checkout", "-b", "feature/login")
	commitFile(t, wc, "login.go", "package login\n", "start login")
	mustGit(t, wc, "push", "origin", "feature/login")

	repoID, _, _ := branchRow(t, m, "widgets", "feature/login")

	mustGit(t, wc, "checkout", "main")
	mustGit(t, wc, "push", "origin", "--delete", "feature/login")

	branches, err := m.ListBranches(context.Background(), repoID)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	for _, b := range branches {
		if b.Name == "feature/login" {
			t.Fatalf("feature/login still indexed after a delete push: %+v", branches)
		}
	}
	if len(branches) != 1 || branches[0].Name != "main" {
		t.Errorf("expected only main to survive, got %+v", branches)
	}
}

// TestRealGit_MergePushLinksBothParents pushes a real `git merge --no-ff`
// result and confirms both parents are indexed, in git's own order.
func TestRealGit_MergePushLinksBothParents(t *testing.T) {
	m, base := newGitServer(t)
	wc := newWorkingCopy(t, base+"/widgets")
	commitFile(t, wc, "a.txt", "A\n", "base commit")
	mustGit(t, wc, "push", "-u", "origin", "main")
	baseSHA := mustGit(t, wc, "rev-parse", "HEAD")

	mustGit(t, wc, "checkout", "-b", "side")
	sideSHA := commitFile(t, wc, "side.txt", "side\n", "side work")

	mustGit(t, wc, "checkout", "main")
	mainSHA := commitFile(t, wc, "main.txt", "main\n", "main work")
	if mainSHA == baseSHA {
		t.Fatal("main did not advance")
	}
	mustGit(t, wc, "merge", "--no-ff", "-m", "merge side into main", "side")
	mergeSHA := mustGit(t, wc, "rev-parse", "HEAD")
	mustGit(t, wc, "push", "origin", "main")

	ctx := context.Background()
	var mergeRow gormstore.CommitRow
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).
		Where("sha = ?", mergeSHA).First(&mergeRow).Error; err != nil {
		t.Fatalf("merge commit was not indexed: %v", err)
	}

	type parentJoin struct {
		SHA         string
		ParentIndex int
	}
	var parents []parentJoin
	if err := m.db.WithContext(ctx).Table(m.tables.CommitParents+" AS cp").
		Select("c.sha AS sha, cp.parent_index AS parent_index").
		Joins("JOIN "+m.tables.Commits+" AS c ON c.id = cp.parent_id").
		Where("cp.commit_id = ?", mergeRow.ID).
		Order("cp.parent_index").
		Scan(&parents).Error; err != nil {
		t.Fatalf("read merge parents: %v", err)
	}
	if len(parents) != 2 {
		t.Fatalf("merge commit has %d indexed parents, want 2: %+v", len(parents), parents)
	}
	if parents[0].SHA != mainSHA {
		t.Errorf("first parent = %q, want the main line %q", parents[0].SHA, mainSHA)
	}
	if parents[1].SHA != sideSHA {
		t.Errorf("second parent = %q, want the merged-in side %q", parents[1].SHA, sideSHA)
	}
}

// TestRealGit_TagPushIsIndexedAsTagNotBranch: a pushed tag is filed as a Tag
// row (board row G13) and never as a Branch row named after the whole ref —
// the regression real git first surfaced.
//
// Both tag kinds are pushed on purpose. A lightweight tag's ref points
// straight at the commit; an annotated tag's points at a tag object carrying
// its own message and tagger, so the two reach the indexer differently.
func TestRealGit_TagPushIsIndexedAsTagNotBranch(t *testing.T) {
	m, base := newGitServer(t)
	remote := base + "/widgets"
	wc := newWorkingCopy(t, remote)
	commitFile(t, wc, "a.txt", "A\n", "base commit")
	mustGit(t, wc, "push", "-u", "origin", "main")

	mustGit(t, wc, "tag", "v1.0.0")                             // lightweight
	mustGit(t, wc, "tag", "-a", "v1.1.0", "-m", "next release") // annotated
	mustGit(t, wc, "push", "origin", "v1.0.0", "v1.1.0")

	repoID, _, _ := branchRow(t, m, "widgets", "main")
	branches, err := m.ListBranches(context.Background(), repoID)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	for _, b := range branches {
		if strings.HasPrefix(b.Name, "refs/") {
			t.Errorf("tag push created a Branch row named %q — refs outside refs/heads/ are not branches", b.Name)
		}
	}
	if len(branches) != 1 {
		t.Errorf("expected main to be the only branch after a tag push, got %+v", branches)
	}

	// The tags themselves are real on the server: a fresh clone gets them back.
	clone := cloneInto(t, remote, "tagged")
	cloned := mustGit(t, clone, "tag", "--list")
	for _, want := range []string{"v1.0.0", "v1.1.0"} {
		if !strings.Contains(cloned, want) {
			t.Errorf("clone did not receive pushed tag %s, got %q", want, cloned)
		}
	}

	tip := strings.TrimSpace(mustGit(t, wc, "rev-parse", "HEAD"))
	tags, err := m.ListTags(context.Background(), repoID)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	byName := map[string]Tag{}
	for _, tg := range tags {
		byName[tg.Name] = tg
	}
	if len(byName) != 2 {
		t.Fatalf("ListTags returned %+v, want exactly v1.0.0 and v1.1.0", tags)
	}
	light, ok := byName["v1.0.0"]
	if !ok || light.SHA != tip || light.Message != "" || light.TaggerName != "" {
		t.Errorf("lightweight tag = %+v, want SHA %s and no message or tagger", light, tip)
	}
	annotated, ok := byName["v1.1.0"]
	if !ok || annotated.SHA != tip || annotated.Message != "next release" || annotated.TaggerName == "" || annotated.TaggerAt == "" {
		t.Errorf("annotated tag = %+v, want SHA %s (the commit, not the tag object), message %q and a tagger", annotated, tip, "next release")
	}

	// A tag pushed for a commit no branch carries indexes that commit too.
	mustGit(t, wc, "checkout", "-b", "side")
	commitFile(t, wc, "b.txt", "B\n", "side commit")
	sideTip := strings.TrimSpace(mustGit(t, wc, "rev-parse", "HEAD"))
	mustGit(t, wc, "tag", "v2.0.0")
	mustGit(t, wc, "push", "origin", "v2.0.0")
	tags, _ = m.ListTags(context.Background(), repoID)
	var side *Tag
	for i := range tags {
		if tags[i].Name == "v2.0.0" {
			side = &tags[i]
		}
	}
	if side == nil || side.SHA != sideTip {
		t.Errorf("tag on a branch-less commit = %+v, want SHA %s", side, sideTip)
	}

	// Deleting a tag over push removes its row.
	mustGit(t, wc, "push", "origin", "--delete", "v1.0.0")
	tags, _ = m.ListTags(context.Background(), repoID)
	for _, tg := range tags {
		if tg.Name == "v1.0.0" {
			t.Errorf("v1.0.0 still listed after `git push --delete`: %+v", tags)
		}
	}
	if len(tags) != 2 {
		t.Errorf("after the delete want v1.1.0 and v2.0.0 left, got %+v", tags)
	}
}
