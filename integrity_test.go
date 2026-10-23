package mwanachamagit

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func writeOneFile(t *testing.T, m *gitManager, content, encoding string) models.Branch {
	t.Helper()
	ctx := context.Background()
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "results"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, _ := m.ListBranches(ctx, repo.ID)
	branch := branches[0]
	if _, err := m.WriteFile(ctx, WriteFileRequest{BranchID: branch.ID, Path: "result.md", Content: content, Encoding: encoding}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return branch
}

func TestReadFileRefusesContentAmendedByDirectSQL(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	branch := writeOneFile(t, m, "Candidate A: 412 votes", "")

	if _, err := m.ReadFile(ctx, branch.ID, "result.md"); err != nil {
		t.Fatalf("untouched read: %v", err)
	}

	res := m.db.Exec("UPDATE "+m.tables.Blobs+" SET content = ? WHERE path = ?", "Candidate A: 912 votes", "result.md")
	if res.Error != nil || res.RowsAffected != 1 {
		t.Fatalf("tamper: %v rows=%d", res.Error, res.RowsAffected)
	}

	_, err := m.ReadFile(ctx, branch.ID, "result.md")
	if !errors.Is(err, ErrContentTampered) {
		t.Fatalf("expected ErrContentTampered, got %v", err)
	}
}

func TestReadFileRefusesARecordedHashAmendedByDirectSQL(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	branch := writeOneFile(t, m, "Candidate A: 412 votes", "")

	res := m.db.Exec("UPDATE "+m.tables.Blobs+" SET sha = ? WHERE path = ?", "0123456789abcdef0123456789abcdef01234567", "result.md")
	if res.Error != nil || res.RowsAffected != 1 {
		t.Fatalf("tamper: %v rows=%d", res.Error, res.RowsAffected)
	}

	_, err := m.ReadFile(ctx, branch.ID, "result.md")
	if !errors.Is(err, ErrContentTampered) {
		t.Fatalf("expected ErrContentTampered, got %v", err)
	}
}

func TestReadFileAcceptsBase64ContentHashedOverItsBytes(t *testing.T) {
	raw := []byte{0x00, 0xff, 0x10, 0x80}
	blob := models.Blob{SHA: gitBlobSHA(raw), Content: base64.StdEncoding.EncodeToString(raw), Encoding: "base64"}
	if err := verifyBlobContent(blob); err != nil {
		t.Fatalf("base64 blob hashed over its bytes: %v", err)
	}
	blob.Content = base64.StdEncoding.EncodeToString([]byte{0x01})
	if !errors.Is(verifyBlobContent(blob), ErrContentTampered) {
		t.Fatal("a changed base64 blob must be refused")
	}
}

func TestReadFileAcceptsBase64ContentWrittenThroughWriteFile(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	branch := writeOneFile(t, m, base64.StdEncoding.EncodeToString([]byte("bytes")), "base64")
	if _, err := m.ReadFile(ctx, branch.ID, "result.md"); err != nil {
		t.Fatalf("base64 read: %v", err)
	}
}
