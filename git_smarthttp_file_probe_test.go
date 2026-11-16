package mwanachamagit

import (
	"bytes"
	"crypto/rand"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func postReceivePack(t *testing.T, url string, body []byte) (int, string, []byte) {
	t.Helper()
	resp, err := http.Post(url+"/git-receive-pack", "application/x-git-receive-pack-request", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post receive-pack: %v", err)
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return resp.StatusCode, resp.Header.Get("Content-Type"), got
}

func pushRandomFile(t *testing.T, s fileGitServer, size int) {
	t.Helper()
	work := s.clone(t)
	transportGit(t, work, "checkout", "-b", "big")
	payload := make([]byte, size)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("random bytes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(work, "blob.bin"), payload, 0o644); err != nil {
		t.Fatalf("write blob: %v", err)
	}
	transportGit(t, work, "add", "blob.bin")
	transportGit(t, work, "commit", "-m", "add blob")
	tip := transportGit(t, work, "rev-parse", "HEAD")
	transportGit(t, work, "push", "origin", "big")

	again := s.clone(t, "--branch", "big")
	if got := transportGit(t, again, "rev-parse", "HEAD"); got != tip {
		t.Errorf("cloned HEAD = %q, want %q", got, tip)
	}
	cloned, err := os.ReadFile(filepath.Join(again, "blob.bin"))
	if err != nil {
		t.Fatalf("read cloned blob: %v", err)
	}
	if !bytes.Equal(cloned, payload) {
		t.Errorf("cloned blob differs from the pushed one (%d bytes vs %d)", len(cloned), len(payload))
	}
}

func TestReceivePackProbe_AThreeMiBRandomFilePushesAndClonesBackIdentical(t *testing.T) {
	pushRandomFile(t, newFileGitServer(t), 3<<20)
}

func TestReceivePackProbe_ATwentyMiBRandomFilePushesAndClonesBackIdentical(t *testing.T) {
	pushRandomFile(t, newFileGitServer(t), 20<<20)
}

func TestReceivePackProbe_ABodyOfExactlyFourZerosGetsAnEmptyOK(t *testing.T) {
	s := newFileGitServer(t)
	code, contentType, body := postReceivePack(t, s.url, []byte("0000"))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", code, body)
	}
	if contentType != "application/x-git-receive-pack-result" {
		t.Errorf("content type = %q", contentType)
	}
	if len(body) != 0 {
		t.Errorf("body = %q, want empty", body)
	}
}

func TestReceivePackProbe_AGarbageBodyKeepsTheMalformedRefusal(t *testing.T) {
	s := newFileGitServer(t)
	code, _, body := postReceivePack(t, s.url, []byte("garbage that is no push"))
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
	if !strings.Contains(string(body), "malformed receive-pack request") {
		t.Errorf("body = %q", body)
	}
}

func TestReceivePackProbe_AShortPushStillWorks(t *testing.T) {
	s := newFileGitServer(t)
	work := s.clone(t)
	transportGit(t, work, "checkout", "-b", "short")
	tip := transportCommit(t, work, "note.txt", "short\n", "add note")
	transportGit(t, work, "push", "origin", "short")
	again := s.clone(t, "--branch", "short")
	if got := transportGit(t, again, "rev-parse", "HEAD"); got != tip {
		t.Errorf("cloned HEAD = %q, want %q", got, tip)
	}
}
