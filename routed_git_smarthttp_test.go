package mwanachamagit

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoutedGitManager_SmartHTTPServesEachRepositoryFileByNameAndRefusesTheDefaultBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git binary on PATH")
	}
	f := newRoutedGitFixture(t, 4)
	harvest, harvestMain := f.initRepo(t, "harvest")
	f.writeAndMerge(t, harvest, harvestMain, "plan.md", "plant in march\n")
	other, otherMain := f.initRepo(t, "orchard")
	f.writeAndMerge(t, other, otherMain, "trees.md", "apples\n")

	srv := httptest.NewServer(f.manager.SmartHTTPHandler())
	t.Cleanup(srv.Close)

	parent := t.TempDir()
	transportGit(t, parent, "clone", srv.URL+"/harvest.git", "first")
	first := filepath.Join(parent, "first")
	if data, err := os.ReadFile(filepath.Join(first, "plan.md")); err != nil || string(data) != "plant in march\n" {
		t.Fatalf("harvest clone plan.md = %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(first, "trees.md")); err == nil {
		t.Error("the harvest clone holds a file of the orchard repository")
	}

	transportGit(t, first, "checkout", "-b", "topic")
	if err := os.WriteFile(filepath.Join(first, "notes.md"), []byte("notes\n"), 0o644); err != nil {
		t.Fatalf("write notes.md: %v", err)
	}
	transportGit(t, first, "add", "notes.md")
	transportGit(t, first, "commit", "-m", "notes")
	tip := transportGit(t, first, "rev-parse", "HEAD")
	transportGit(t, first, "push", "origin", "topic")

	transportGit(t, parent, "clone", "--branch", "topic", srv.URL+"/harvest.git", "second")
	second := filepath.Join(parent, "second")
	if got := transportGit(t, second, "rev-parse", "HEAD"); got != tip {
		t.Errorf("second clone HEAD = %q, want %q", got, tip)
	}
	for path, want := range map[string]string{"plan.md": "plant in march\n", "notes.md": "notes\n"} {
		if data, err := os.ReadFile(filepath.Join(second, path)); err != nil || string(data) != want {
			t.Errorf("second clone %s = %q, %v, want %q", path, data, err, want)
		}
	}

	transportGit(t, first, "checkout", "main")
	transportGit(t, first, "commit", "--allow-empty", "-m", "straight to main")
	out, err := transportGitMayFail(first, "push", "origin", "main")
	if err == nil {
		t.Fatalf("push of a changed default branch succeeded:\n%s", out)
	}
	if !strings.Contains(out, "push to a merge request, not to the default branch") {
		t.Errorf("refusal did not carry the plain message:\n%s", out)
	}

	if out, err := transportGitMayFail(parent, "clone", srv.URL+"/missing.git", "third"); err == nil || !strings.Contains(out, "not found") {
		t.Errorf("clone of an unknown repository = %v, %q, want a not found refusal", err, out)
	}
}

func TestRoutedGitManager_SmartHTTPRefusesAPathWithNoRepositoryName(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	srv := httptest.NewServer(f.manager.SmartHTTPHandler())
	t.Cleanup(srv.Close)
	for _, path := range []string{"/", "/widgets.git", "/.git/info/refs"} {
		response, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want 400", path, response.StatusCode)
		}
	}
}
