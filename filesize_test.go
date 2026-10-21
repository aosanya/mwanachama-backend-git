package mwanachamagit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func agencyManager(t *testing.T, db *gorm.DB, instance string, opts ...Option) GitManager {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("spec", "examples", "engineering.git.json"))
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal spec: %v", err)
	}
	doc["instance"] = instance
	raw, _ = json.Marshal(doc)
	s, err := ParseSpec(raw)
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	if err := Provision(db, s); err != nil {
		t.Fatalf("Provision: %v", err)
	}
	m, err := NewGitManager(db, s, nil, nil, nil, opts...)
	if err != nil {
		t.Fatalf("NewGitManager: %v", err)
	}
	return m
}

func sharedDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	return db
}

func reportBranch(t *testing.T, m GitManager) string {
	t.Helper()
	ctx := context.Background()
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "reports"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, err := m.ListBranches(ctx, repo.ID)
	if err != nil || len(branches) == 0 {
		t.Fatalf("ListBranches: %v", err)
	}
	return branches[0].ID
}

func pdf(body string) string {
	return base64.StdEncoding.EncodeToString([]byte("%PDF-1.7\n" + body))
}

func TestTwoAgenciesKeepTheirReportsApart(t *testing.T) {
	ctx := context.Background()
	db := sharedDB(t)
	a := agencyManager(t, db, "agya")
	b := agencyManager(t, db, "agyb")
	branchA := reportBranch(t, a)
	branchB := reportBranch(t, b)

	if _, err := a.WriteFile(ctx, WriteFileRequest{BranchID: branchA, Path: "forms/f1/report.pdf", Content: pdf("agency a"), Encoding: "base64", Message: "Report v1"}); err != nil {
		t.Fatalf("WriteFile a: %v", err)
	}

	if _, err := b.ReadFile(ctx, branchB, "forms/f1/report.pdf"); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("agency b read a's path on its own branch: err = %v, want ErrFileNotFound", err)
	}
	if blob, err := b.ReadFile(ctx, branchA, "forms/f1/report.pdf"); err == nil {
		t.Fatalf("agency b read agency a's report through a's branch id: %q", blob.Content)
	}
	if log, err := b.Log(ctx, branchA, LogFilter{}); err == nil && len(log) > 0 {
		t.Fatalf("agency b read agency a's history: %d commits", len(log))
	}
	reposB, err := b.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("ListRepositories b: %v", err)
	}
	if len(reposB) != 1 {
		t.Fatalf("agency b sees %d repositories, want its own one", len(reposB))
	}
}

func TestAReplacedReportKeepsItsHistory(t *testing.T) {
	ctx := context.Background()
	m := agencyManager(t, sharedDB(t), "agya")
	branch := reportBranch(t, m)
	path := "forms/f1/report.pdf"

	if _, err := m.WriteFile(ctx, WriteFileRequest{BranchID: branch, Path: path, Content: pdf("first"), Encoding: "base64", Message: "Report v1"}); err != nil {
		t.Fatalf("WriteFile v1: %v", err)
	}
	if _, err := m.WriteFile(ctx, WriteFileRequest{BranchID: branch, Path: path, Content: pdf("second"), Encoding: "base64", Message: "Report v2"}); err != nil {
		t.Fatalf("WriteFile v2: %v", err)
	}

	latest, err := m.ReadFile(ctx, branch, path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if latest.Content != pdf("second") {
		t.Fatalf("latest content = %q, want the second version", latest.Content)
	}

	history, err := m.Log(ctx, branch, LogFilter{Path: path})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	var messages []string
	for _, c := range history {
		messages = append(messages, c.Message)
	}
	joined := strings.Join(messages, "|")
	if !strings.Contains(joined, "Report v1") || !strings.Contains(joined, "Report v2") {
		t.Fatalf("history messages = %v, want both versions", messages)
	}
}

func TestTheHostSetsTheFileSizeLimit(t *testing.T) {
	ctx := context.Background()
	m := agencyManager(t, sharedDB(t), "agya", WithMaxFileSize(16))
	branch := reportBranch(t, m)

	_, err := m.WriteFile(ctx, WriteFileRequest{BranchID: branch, Path: "big.pdf", Content: pdf("more than sixteen bytes"), Encoding: "base64"})
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("oversized write err = %v, want ErrFileTooLarge", err)
	}
	if _, err := m.ReadFile(ctx, branch, "big.pdf"); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("refused write left a file: err = %v", err)
	}
	if _, err := m.WriteFile(ctx, WriteFileRequest{BranchID: branch, Path: "small.pdf", Content: pdf("ok"), Encoding: "base64"}); err != nil {
		t.Fatalf("write within the limit: %v", err)
	}
}

func TestNoLimitUnlessTheHostSetsOne(t *testing.T) {
	ctx := context.Background()
	m := agencyManager(t, sharedDB(t), "agya")
	branch := reportBranch(t, m)
	if _, err := m.WriteFile(ctx, WriteFileRequest{BranchID: branch, Path: "big.pdf", Content: pdf(strings.Repeat("x", 4096)), Encoding: "base64"}); err != nil {
		t.Fatalf("write with no limit set: %v", err)
	}
}
