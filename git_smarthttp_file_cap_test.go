package mwanachamagit

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	pushCapMessage = "this push is over the 32 MiB limit; push fewer files at a time"
	pushCapBytes   = 32 << 20
)

func randomBinaryFile(t *testing.T, dir, name string, size int) {
	t.Helper()
	data := make([]byte, size)
	rand.New(rand.NewSource(int64(size))).Read(data)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func commitRandomFile(t *testing.T, dir, name string, size int) string {
	t.Helper()
	randomBinaryFile(t, dir, name, size)
	transportGit(t, dir, "add", name)
	transportGit(t, dir, "commit", "-m", "binary "+name)
	return transportGit(t, dir, "rev-parse", "HEAD")
}

type countingBody struct {
	reader io.Reader
	read   atomic.Int64
}

func (c *countingBody) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	c.read.Add(int64(n))
	return n, err
}

func (c *countingBody) Close() error { return nil }

func serveReceivePack(h http.Handler, body io.Reader, contentLength int64) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/git-receive-pack", body)
	req.ContentLength = contentLength
	req.Header.Set("Content-Type", "application/x-git-receive-pack-request")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func statusesOf(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	return reportStatuses(t, rec.Code, rec.Result().Status, bytes.NewReader(rec.Body.Bytes()))
}

func TestGitPushCap_APushOverThirtyTwoMiBIsRefusedWithAPlainMessageAndNothingIsStored(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()
	work := s.clone(t)
	transportGit(t, work, "checkout", "-b", "big")
	commitRandomFile(t, work, "huge.bin", pushCapBytes+(1<<20))

	out, err := transportGitMayFail(work, "push", "origin", "big")

	if err == nil {
		t.Fatalf("a push over the cap was accepted:\n%s", out)
	}
	if !strings.Contains(out, pushCapMessage) {
		t.Errorf("the refusal did not carry the plain message:\n%s", out)
	}
	if _, err := s.rf.GetBranchByName(ctx, s.repo, "big"); err == nil {
		t.Error("a branch row exists for the refused push")
	}
	fresh := s.clone(t)
	if _, err := os.Stat(filepath.Join(fresh, "huge.bin")); err == nil {
		t.Error("the refused file is visible in a new clone")
	}
}

func TestGitPushCap_APushJustUnderTheCapIsAcceptedAndCloneable(t *testing.T) {
	s := newFileGitServer(t)
	work := s.clone(t)
	transportGit(t, work, "checkout", "-b", "large")
	tip := commitRandomFile(t, work, "large.bin", 20<<20)

	transportGit(t, work, "push", "origin", "large")

	fresh := s.clone(t, "--branch", "large")
	if got := transportGit(t, fresh, "rev-parse", "HEAD"); got != tip {
		t.Errorf("clone HEAD = %s, want %s", got, tip)
	}
}

func TestGitPushCap_APushWhoseBodyIsExactlyTheCapIsAcceptedAndOneByteMoreIsRefused(t *testing.T) {
	s := newFileGitServer(t)
	work := s.clone(t)
	mainTip := transportGit(t, work, "rev-parse", "HEAD")
	transportGit(t, work, "checkout", "-b", "edge")
	tip := transportCommit(t, work, "edge.txt", "edge\n", "edge")
	body := receivePackBody(t, work, mainTip, pushCommand{"refs/heads/edge", transportZeroSHA, tip}).Bytes()
	manager := transportManager(t, s.rf)
	handler := s.rf.GitHTTPHandler()

	manager.maxPush = int64(len(body)) - 1
	over := statusesOf(t, serveReceivePack(handler, bytes.NewReader(body), -1))
	if over["refs/heads/edge"] == "ok" {
		t.Fatalf("a body one byte over the cap was accepted: %v", over)
	}
	if !strings.Contains(over["refs/heads/edge"], "over the") {
		t.Errorf("status = %q, want the limit message", over["refs/heads/edge"])
	}

	manager.maxPush = int64(len(body))
	at := statusesOf(t, serveReceivePack(handler, bytes.NewReader(body), -1))
	if at["refs/heads/edge"] != "ok" {
		t.Errorf("a body exactly at the cap: status = %q, want ok", at["refs/heads/edge"])
	}
}

func TestGitPushCap_ABodyThatDeclaresMoreThanTheCapIsRefusedWithoutReadingIt(t *testing.T) {
	s := newFileGitServer(t)
	manager := transportManager(t, s.rf)
	manager.maxPush = 1 << 10
	body := &countingBody{reader: bytes.NewReader(make([]byte, 64<<10))}

	rec := serveReceivePack(s.rf.GitHTTPHandler(), body, 64<<10)

	if rec.Code != http.StatusOK && rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "over the") {
		t.Errorf("body = %q, want the limit message", rec.Body.String())
	}
	if got := body.read.Load(); got != 0 {
		t.Errorf("the server read %d bytes of a body it could refuse by its declared length", got)
	}
}

func TestGitPushCap_AChunkedBodyIsCutOffAtTheCapAndNotReadToTheEnd(t *testing.T) {
	s := newFileGitServer(t)
	manager := transportManager(t, s.rf)
	work := s.clone(t)
	mainTip := transportGit(t, work, "rev-parse", "HEAD")
	transportGit(t, work, "checkout", "-b", "chunked")
	tip := commitRandomFile(t, work, "chunk.bin", 4<<20)
	whole := receivePackBody(t, work, mainTip, pushCommand{"refs/heads/chunked", transportZeroSHA, tip}).Bytes()
	manager.maxPush = int64(len(whole)) / 4
	body := &countingBody{reader: bytes.NewReader(whole)}

	rec := serveReceivePack(s.rf.GitHTTPHandler(), body, -1)

	if got := statusesOf(t, rec)["refs/heads/chunked"]; got == "ok" || got == "" {
		t.Fatalf("status = %q, want a refusal", got)
	}
	if got, limit := body.read.Load(), manager.maxPush+(1<<20); got > limit {
		t.Errorf("the server read %d bytes, want at most %d (the cap plus its buffers)", got, limit)
	}
}
