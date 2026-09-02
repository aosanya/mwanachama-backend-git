package mwanachamagit

import (
	"context"
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-go-shared/entitygraph"
)

func TestWriteReadDeleteFile(t *testing.T) {
	ctx := context.Background()
	m := newTestManager()

	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, _ := m.ListBranches(ctx, repo.ID)
	branch := branches[0]

	commit1, err := m.WriteFile(ctx, WriteFileRequest{
		BranchID: branch.ID,
		Path:     "README.md",
		Content:  "# widgets",
	})
	if err != nil {
		t.Fatalf("WriteFile 1: %v", err)
	}
	if commit1.ParentIDs != nil {
		t.Fatalf("expected first commit to have no parents, got %v", commit1.ParentIDs)
	}

	blob, err := m.ReadFile(ctx, branch.ID, "README.md")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if blob.Content != "# widgets" {
		t.Fatalf("expected cached content, got %q", blob.Content)
	}

	commit2, err := m.WriteFile(ctx, WriteFileRequest{
		BranchID: branch.ID,
		Path:     "src/main.go",
		Content:  "package main",
	})
	if err != nil {
		t.Fatalf("WriteFile 2: %v", err)
	}
	if len(commit2.ParentIDs) != 1 || commit2.ParentIDs[0] != commit1.ID {
		t.Fatalf("expected commit2 parent to be commit1, got %v", commit2.ParentIDs)
	}

	// README.md must still be reachable after a second, unrelated write —
	// this is the "each write includes the parent's full file set" guarantee.
	if _, err := m.ReadFile(ctx, branch.ID, "README.md"); err != nil {
		t.Fatalf("ReadFile README.md after second write: %v", err)
	}

	entries, err := m.ListDirectory(ctx, branch.ID, "")
	if err != nil {
		t.Fatalf("ListDirectory root: %v", err)
	}
	var sawReadme, sawSrcDir bool
	for _, e := range entries {
		if e.Path == "README.md" && !e.IsDir {
			sawReadme = true
		}
		if e.Path == "src" && e.IsDir {
			sawSrcDir = true
		}
	}
	if !sawReadme || !sawSrcDir {
		t.Fatalf("expected README.md file and src dir at root, got %+v", entries)
	}

	srcEntries, err := m.ListDirectory(ctx, branch.ID, "src")
	if err != nil {
		t.Fatalf("ListDirectory src: %v", err)
	}
	if len(srcEntries) != 1 || srcEntries[0].Path != "src/main.go" {
		t.Fatalf("expected src/main.go, got %+v", srcEntries)
	}

	history, err := m.Log(ctx, branch.ID, LogFilter{})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 commits in history, got %d", len(history))
	}

	// LogFilter.Path keeps commits whose tree still *contains* the path, not
	// only the commit that last changed it — each WriteFile carries the full
	// parent file set forward, so README.md is present in both commits' trees.
	readmeHistory, err := m.Log(ctx, branch.ID, LogFilter{Path: "README.md"})
	if err != nil {
		t.Fatalf("Log filtered: %v", err)
	}
	if len(readmeHistory) != 2 {
		t.Fatalf("expected 2 commits whose tree contains README.md, got %d", len(readmeHistory))
	}

	commit3, err := m.DeleteFile(ctx, DeleteFileRequest{BranchID: branch.ID, Path: "README.md"})
	if err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if commit3.ID == commit2.ID {
		t.Fatalf("DeleteFile should produce a new commit")
	}
	if _, err := m.ReadFile(ctx, branch.ID, "README.md"); err == nil {
		t.Fatalf("expected README.md to read back empty content or be gone after delete")
	}

	if _, err := m.ReadFile(ctx, branch.ID, "does/not/exist.txt"); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("expected ErrFileNotFound, got %v", err)
	}
}

func TestDiffBetweenCommits(t *testing.T) {
	ctx := context.Background()
	m := newTestManager()

	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, _ := m.ListBranches(ctx, repo.ID)
	branch := branches[0]

	c1, err := m.WriteFile(ctx, WriteFileRequest{BranchID: branch.ID, Path: "a.txt", Content: "one"})
	if err != nil {
		t.Fatalf("WriteFile a: %v", err)
	}
	c2, err := m.WriteFile(ctx, WriteFileRequest{BranchID: branch.ID, Path: "a.txt", Content: "two"})
	if err != nil {
		t.Fatalf("WriteFile a v2: %v", err)
	}
	if _, err := m.WriteFile(ctx, WriteFileRequest{BranchID: branch.ID, Path: "b.txt", Content: "new"}); err != nil {
		t.Fatalf("WriteFile b: %v", err)
	}

	diffs, err := m.Diff(ctx, c1.ID, c2.ID)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(diffs) != 1 || diffs[0].Path != "a.txt" || diffs[0].Operation != "modified" {
		t.Fatalf("expected a.txt modified, got %+v", diffs)
	}

	diffs, err = m.Diff(ctx, c1.ID, branch.ID)
	if err != nil {
		t.Fatalf("Diff to branch: %v", err)
	}
	if len(diffs) != 2 {
		t.Fatalf("expected 2 diffs (a.txt modified, b.txt added) from c1 to branch HEAD, got %+v", diffs)
	}

	if _, err := m.Diff(ctx, "nonexistent-ref", branch.ID); !errors.Is(err, ErrRefNotFound) {
		t.Fatalf("expected ErrRefNotFound, got %v", err)
	}
}

func TestImportRepoGuards(t *testing.T) {
	ctx := context.Background()
	m := newTestManager()

	if _, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"}); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	// A Repository entity already exists — ImportRepo must refuse before it
	// ever touches the network.
	if _, err := m.ImportRepo(ctx, ImportRepoRequest{Name: "other", SourceURL: "https://example.invalid/x.git"}); !errors.Is(err, ErrRepoAlreadyExists) {
		t.Fatalf("expected ErrRepoAlreadyExists, got %v", err)
	}
}

func TestImportRepoRejectsIfImportInProgress(t *testing.T) {
	ctx := context.Background()
	m := newTestManager()
	now := "2026-01-01T00:00:00Z"

	if _, err := m.dm.CreateEntity(ctx, entitygraph.CreateEntityRequest{
		AgencyID: m.agencyID,
		TypeID:   "ImportJob",
		Properties: map[string]any{
			"agency_id": m.agencyID, "source_url": "https://example.com/first.git",
			"status": "pending", "error_message": "", "created_at": now, "updated_at": now,
		},
	}); err != nil {
		t.Fatalf("seed ImportJob: %v", err)
	}

	if _, err := m.ImportRepo(ctx, ImportRepoRequest{Name: "second-import", SourceURL: "https://example.com/second.git"}); !errors.Is(err, ErrImportInProgress) {
		t.Fatalf("expected ErrImportInProgress, got %v", err)
	}
}

func TestGetImportStatusNotFound(t *testing.T) {
	m := newTestManager()
	if _, err := m.GetImportStatus(context.Background(), "does-not-exist"); !errors.Is(err, ErrImportJobNotFound) {
		t.Fatalf("expected ErrImportJobNotFound, got %v", err)
	}
}

func TestCancelImportNotFound(t *testing.T) {
	m := newTestManager()
	if err := m.CancelImport(context.Background(), "does-not-exist"); !errors.Is(err, ErrImportJobNotFound) {
		t.Fatalf("expected ErrImportJobNotFound, got %v", err)
	}
}

func TestCancelImportTerminalState(t *testing.T) {
	for _, status := range []string{"completed", "failed", "cancelled"} {
		t.Run("status="+status, func(t *testing.T) {
			ctx := context.Background()
			m := newTestManager()
			now := "2026-01-01T00:00:00Z"

			jobEntity, err := m.dm.CreateEntity(ctx, entitygraph.CreateEntityRequest{
				AgencyID: m.agencyID,
				TypeID:   "ImportJob",
				Properties: map[string]any{
					"agency_id": m.agencyID, "source_url": "https://example.com/repo.git",
					"status": status, "error_message": "", "created_at": now, "updated_at": now,
				},
			})
			if err != nil {
				t.Fatalf("seed ImportJob: %v", err)
			}
			if err := m.CancelImport(ctx, jobEntity.ID); !errors.Is(err, ErrImportJobNotCancellable) {
				t.Fatalf("status=%s: expected ErrImportJobNotCancellable, got %v", status, err)
			}
		})
	}
}

func TestFetchBranchAlreadyFetchedGuard(t *testing.T) {
	ctx := context.Background()
	m := newTestManager()

	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, _ := m.ListBranches(ctx, repo.ID)
	branch := branches[0]

	// WriteFile gives the branch a real HeadCommitID, which FetchBranch's
	// short-circuit path treats as "already fetched" without touching the
	// network — exercised here instead of the goroutine-driven clone path.
	if _, err := m.WriteFile(ctx, WriteFileRequest{BranchID: branch.ID, Path: "a.txt", Content: "one"}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	job, err := m.FetchBranch(ctx, FetchBranchRequest{RepoID: repo.ID, BranchID: branch.ID})
	if err != nil {
		t.Fatalf("FetchBranch: %v", err)
	}
	if job.Status != fetchJobStatusCompleted {
		t.Fatalf("expected short-circuited job to be completed, got %q", job.Status)
	}

	if _, err := m.FetchBranch(ctx, FetchBranchRequest{RepoID: repo.ID, BranchID: branch.ID}); !errors.Is(err, ErrBranchAlreadyFetched) {
		t.Fatalf("expected ErrBranchAlreadyFetched, got %v", err)
	}
}
