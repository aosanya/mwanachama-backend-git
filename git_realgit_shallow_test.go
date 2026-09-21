//go:build integration

// git_realgit_shallow_test.go covers board row G14 (todo.md): a real `git
// clone --depth N` against SmartHTTPHandler. go-git's own upload-pack session
// refuses every shallow request and never advertises the "shallow"
// capability, so git used to die client-side with "Server does not support
// shallow clients". git_smarthttp_shallow.go now serves a fresh depth-limited
// clone itself and refuses the shapes it does not support with a stated
// reason.
package mwanachamagit

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestRealGit_ShallowClone(t *testing.T) {
	_, base := newGitServer(t)
	remote := base + "/widgets"
	wc := newWorkingCopy(t, remote)
	commitFile(t, wc, "a.txt", "A\n", "first commit")
	commitFile(t, wc, "b.txt", "B\n", "second commit")
	commitFile(t, wc, "c.txt", "C\n", "third commit")
	mustGit(t, wc, "push", "-u", "origin", "main")

	commits := func(dir string) []string {
		out := strings.TrimSpace(mustGit(t, dir, "log", "--format=%s"))
		return strings.Split(out, "\n")
	}

	parent := t.TempDir()
	if out, err := runGit(t, parent, "clone", "--depth", "1", remote, "d1"); err != nil {
		t.Fatalf("clone --depth 1 failed: %v\n%s", err, out)
	}
	d1 := parent + "/d1"
	if got := commits(d1); len(got) != 1 || got[0] != "third commit" {
		t.Fatalf("depth 1 history = %v, want only the tip", got)
	}
	if _, err := os.Stat(d1 + "/.git/shallow"); err != nil {
		t.Errorf("a depth-1 clone must record its boundary in .git/shallow: %v", err)
	}
	for _, f := range []string{"a.txt", "b.txt", "c.txt"} {
		if _, err := os.Stat(d1 + "/" + f); err != nil {
			t.Errorf("the tip's tree must be complete, missing %s: %v", f, err)
		}
	}
	mustGit(t, d1, "fsck")

	if out, err := runGit(t, parent, "clone", "--depth", "2", remote, "d2"); err != nil {
		t.Fatalf("clone --depth 2 failed: %v\n%s", err, out)
	}
	if got := commits(parent + "/d2"); len(got) != 2 || got[1] != "second commit" {
		t.Fatalf("depth 2 history = %v, want the two newest", got)
	}

	// Deeper than the history: everything, and no shallow boundary.
	if out, err := runGit(t, parent, "clone", "--depth", "10", remote, "d10"); err != nil {
		t.Fatalf("clone --depth 10 failed: %v\n%s", err, out)
	}
	if got := commits(parent + "/d10"); len(got) != 3 {
		t.Fatalf("depth 10 history = %v, want all 3", got)
	}
	if _, err := os.Stat(parent + "/d10/.git/shallow"); err == nil {
		t.Errorf("a clone that reached the root has no shallow boundary")
	}

	// A full clone still works now that the capability is advertised.
	if got := commits(cloneInto(t, remote, "full")); len(got) != 3 {
		t.Fatalf("full clone history = %v, want 3", got)
	}
}

// TestRealGit_ShallowFetchIntoExistingRepoIsRefusedWithAReason: deepening a
// shallow clone is not served. git itself can only report "the remote end
// hung up" for a refused POST, so the reason is checked on the wire: 501 with
// a body that says what is unsupported.
func TestRealGit_ShallowFetchIntoExistingRepoIsRefusedWithAReason(t *testing.T) {
	_, base := newGitServer(t)
	remote := base + "/widgets"
	wc := newWorkingCopy(t, remote)
	commitFile(t, wc, "a.txt", "A\n", "first commit")
	commitFile(t, wc, "b.txt", "B\n", "second commit")
	mustGit(t, wc, "push", "-u", "origin", "main")
	tip := strings.TrimSpace(mustGit(t, wc, "rev-parse", "HEAD"))

	parent := t.TempDir()
	if out, err := runGit(t, parent, "clone", "--depth", "1", remote, "s"); err != nil {
		t.Fatalf("clone --depth 1 failed: %v\n%s", err, out)
	}
	if out, err := runGit(t, parent+"/s", "fetch", "--depth", "2"); err == nil {
		t.Fatalf("deepening a shallow clone unexpectedly succeeded:\n%s", out)
	}

	pkt := func(line string) string { return fmt.Sprintf("%04x%s", len(line)+4, line) }
	body := pkt("want "+tip+" ofs-delta\n") + pkt("shallow "+tip+"\n") + pkt("deepen 2\n") + "0000" + pkt("done\n")
	resp, err := http.Post(remote+"/git-upload-pack", "application/x-git-upload-pack-request", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST git-upload-pack: %v", err)
	}
	defer resp.Body.Close()
	msg, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNotImplemented || !strings.Contains(string(msg), "not supported") {
		t.Fatalf("status=%d body=%q, want 501 saying the request is not supported", resp.StatusCode, msg)
	}
}
