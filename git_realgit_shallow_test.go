//go:build integration

// git_realgit_shallow_test.go pins board row G14 (todo.md): a real `git
// clone --depth N` (or `git fetch --depth N`) against SmartHTTPHandler
// fails outright, rather than serving a real shallow clone or refusing with
// a caller-legible message.
//
// Root cause lives one layer down: go-git v5.16.5's own server-side upload-
// pack session (plumbing/transport/server/server.go) hard-refuses any
// request whose UploadPackRequest.Shallows is non-empty ("shallow not
// supported"), and git_smarthttp.go's uploadPack handler passes every
// client request through to that session unmodified — there is no
// capability advertisement or pre-check that would let a real git client
// negotiate a full clone instead, or get a clean error before it tries.
//
// This is a real, caller-visible gap even though its root cause is a
// third-party library limitation: any operator running a plain `git clone
// --depth 1 <server>/<repo>` against this service — a common move on a
// large or slow-networked repository — gets a bare exit-128 failure and no
// local repository at all.
package mwanachamagit

import (
	"strings"
	"testing"
)

// TestRealGit_ShallowCloneFailsOutright pins the CURRENT broken behaviour.
// Once G14 is fixed (either by implementing real shallow support in
// git_smarthttp.go's uploadPack, or by having it reject a shallow request
// with a clear, caller-legible message instead of letting go-git's server
// package's raw "shallow not supported" propagate as a mid-protocol hang-
// up), update this test to assert the fixed behaviour and close G14.
func TestRealGit_ShallowCloneFailsOutright(t *testing.T) {
	_, base := newGitServer(t)
	remote := base + "/widgets"
	wc := newWorkingCopy(t, remote)
	commitFile(t, wc, "a.txt", "A\n", "first commit")
	commitFile(t, wc, "b.txt", "B\n", "second commit")
	commitFile(t, wc, "c.txt", "C\n", "third commit")
	mustGit(t, wc, "push", "-u", "origin", "main")

	// Sanity: a full (non-shallow) clone of the same repo succeeds — only
	// the shallow negotiation path is broken.
	fullClone := cloneInto(t, remote, "full")
	fullLog := mustGit(t, fullClone, "log", "--oneline")
	if got := len(strings.Split(strings.TrimSpace(fullLog), "\n")); got != 3 {
		t.Fatalf("full clone: expected 3 commits, got %d:\n%s", got, fullLog)
	}

	parent := t.TempDir()
	out, err := runGit(t, parent, "clone", "--depth", "1", remote, "shallow")
	if err == nil {
		t.Fatalf("G14 REGRESSION-OR-FIX DETECTED: `git clone --depth 1` succeeded (output below), "+
			"which means G14 has been fixed — update this test to assert the shallow clone actually "+
			"has 1 commit (a real .git/shallow file) and close G14 on the board.\n%s", out)
	}
	if !strings.Contains(out, "shallow") {
		t.Fatalf("G14 pin: expected the failure to mention \"shallow\" (go-git's server-side refusal), got:\n%s\nerr=%v", out, err)
	}
	t.Logf("G14 pinned: `git clone --depth 1` against SmartHTTPHandler fails as expected:\n%s\nerr=%v", out, err)
}
