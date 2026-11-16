package mwanachamagit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	mwanachamagit "github.com/aosanya/mwanachama-backend-git"
	"github.com/aosanya/mwanachama-backend-git/routes"
)

func wireUnrelatedServer(t *testing.T) (*httptest.Server, *mwanachamagit.RepoFile, string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git binary on PATH")
	}
	s, err := mwanachamagit.LoadSpec(filepath.Join("spec", "examples", "engineering.git.json"))
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	rf, err := mwanachamagit.OpenRepoFile(filepath.Join(t.TempDir(), "unrelated.db"), s, nil, nil, nil)
	if err != nil {
		t.Fatalf("OpenRepoFile: %v", err)
	}
	t.Cleanup(func() { _ = rf.Close() })
	ctx := context.Background()
	repo, err := rf.InitRepo(ctx, mwanachamagit.CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, err := rf.ListBranches(ctx, repo.ID)
	if err != nil || len(branches) == 0 {
		t.Fatalf("ListBranches: %v", err)
	}
	if _, err := rf.WriteFile(ctx, mwanachamagit.WriteFileRequest{BranchID: branches[0].ID, Path: "README.md", Content: "# widgets\n"}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	mux := http.NewServeMux()
	for _, rt := range routes.Routes(rf) {
		mux.HandleFunc(rt.Pattern(""), rt.Handler)
	}
	api := httptest.NewServer(mux)
	t.Cleanup(api.Close)
	gitServer := httptest.NewServer(rf.GitHTTPHandler())
	t.Cleanup(gitServer.Close)
	return api, rf, repo.ID, gitServer.URL
}

func wireGitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=Folder", "GIT_AUTHOR_EMAIL=folder@example.com",
		"GIT_COMMITTER_NAME=Folder", "GIT_COMMITTER_EMAIL=folder@example.com",
		"GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func wirePushFolder(t *testing.T, url string, files map[string]string) {
	t.Helper()
	dir := t.TempDir()
	wireGitIn(t, dir, "init", "-b", "folder")
	wireGitIn(t, dir, "remote", "add", "origin", url)
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		wireGitIn(t, dir, "add", path)
	}
	wireGitIn(t, dir, "commit", "-m", "the folder")
	wireGitIn(t, dir, "push", "origin", "folder")
}

func wireBranchID(t *testing.T, api *httptest.Server, repoID, name string) string {
	t.Helper()
	code, body := wireCall(t, api, "GET", "/repos/"+repoID+"/branches", "")
	var branches []map[string]any
	if err := json.Unmarshal(body, &branches); err != nil || code != http.StatusOK {
		t.Fatalf("list branches: %d %s", code, body)
	}
	for _, b := range branches {
		if b["name"] == name {
			id, _ := b["id"].(string)
			return id
		}
	}
	t.Fatalf("no branch %q in %s", name, body)
	return ""
}

func wireOpenAndComplete(t *testing.T, api *httptest.Server, repoID, sourceID string) (int, []byte, string) {
	t.Helper()
	code, body := wireCall(t, api, "POST", "/merge-requests",
		`{"repository_id":"`+repoID+`","title":"bring in the folder","source_branch_id":"`+sourceID+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("open merge request: %d %s", code, body)
	}
	mrID := wireDecodeID(t, body)
	code, body = wireCall(t, api, "POST", "/merge-requests/"+mrID+"/complete", `{}`)
	return code, body, mrID
}

func TestCompletingAMergeRequestOfAnUnrelatedPushedHistoryMergesDisjointPaths(t *testing.T) {
	api, rf, repoID, gitURL := wireUnrelatedServer(t)
	wirePushFolder(t, gitURL, map[string]string{"app.go": "package main\n"})
	folderID := wireBranchID(t, api, repoID, "folder")

	code, body, mrID := wireOpenAndComplete(t, api, repoID, folderID)
	if code != http.StatusOK {
		t.Fatalf("completing an unrelated-history merge request: %d %s", code, body)
	}
	code, body = wireCall(t, api, "GET", "/merge-requests/"+mrID, "")
	var mr map[string]any
	if err := json.Unmarshal(body, &mr); err != nil || code != http.StatusOK || mr["status"] != "merged" {
		t.Fatalf("merge request after completion: %d %s", code, body)
	}

	mainID := wireBranchID(t, api, repoID, "main")
	for path, want := range map[string]string{"README.md": "# widgets\n", "app.go": "package main\n"} {
		blob, err := rf.ReadFile(context.Background(), mainID, path)
		if err != nil || blob.Content != want {
			t.Errorf("main %s = %q, err %v, want %q", path, blob.Content, err, want)
		}
	}
	entries, err := rf.Log(context.Background(), mainID, mwanachamagit.LogFilter{})
	if err != nil || len(entries) != 3 {
		t.Errorf("main log has %d entries (err %v), want main root, folder root and the merge commit", len(entries), err)
	}
}

func TestCompletingAMergeRequestOfAnUnrelatedPushedHistoryWithTheSameReadmeAnswersConflict(t *testing.T) {
	api, rf, repoID, gitURL := wireUnrelatedServer(t)
	wirePushFolder(t, gitURL, map[string]string{"README.md": "# the folder\n", "app.go": "package main\n"})
	folderID := wireBranchID(t, api, repoID, "folder")
	mainID := wireBranchID(t, api, repoID, "main")
	before, err := rf.GetBranch(context.Background(), mainID)
	if err != nil {
		t.Fatalf("GetBranch: %v", err)
	}

	code, body, mrID := wireOpenAndComplete(t, api, repoID, folderID)
	if code != http.StatusConflict {
		t.Fatalf("completing a conflicting unrelated-history merge request must be 409, got %d: %s", code, body)
	}
	if !strings.Contains(string(body), "README.md") {
		t.Errorf("the 409 must name README.md, got %s", body)
	}

	code, body = wireCall(t, api, "GET", "/merge-requests/"+mrID, "")
	var mr map[string]any
	if err := json.Unmarshal(body, &mr); err != nil || code != http.StatusOK {
		t.Fatalf("read merge request: %d %s", code, body)
	}
	if mr["status"] != "failed" {
		t.Errorf("status = %v, want failed", mr["status"])
	}
	if msg, _ := mr["error_message"].(string); !strings.Contains(msg, "README.md") {
		t.Errorf("error_message must name README.md, got %q", msg)
	}

	after, err := rf.GetBranch(context.Background(), mainID)
	if err != nil {
		t.Fatalf("GetBranch: %v", err)
	}
	if after.HeadCommitID != before.HeadCommitID {
		t.Errorf("main moved from %s to %s", before.SHA, after.SHA)
	}
	blob, err := rf.ReadFile(context.Background(), mainID, "README.md")
	if err != nil || blob.Content != "# widgets\n" {
		t.Errorf("main README.md = %q, err %v", blob.Content, err)
	}
}

func wireDecodeID(t *testing.T, body []byte) string {
	t.Helper()
	var row map[string]any
	if err := json.Unmarshal(body, &row); err != nil {
		t.Fatalf("decode: %v: %s", err, body)
	}
	id, _ := row["id"].(string)
	if id == "" {
		t.Fatalf("no id in %s", body)
	}
	return id
}

func wireCall(t *testing.T, srv *httptest.Server, method, path, body string) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequest(method, srv.URL+path, r)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}
