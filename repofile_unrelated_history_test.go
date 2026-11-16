package mwanachamagit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

type unrelatedPush struct {
	server fileGitServer
	branch models.Branch
	root   string
	second string
}

func pushUnrelatedFolder(t *testing.T, s fileGitServer, files []featureFile) unrelatedPush {
	t.Helper()
	folder := filepath.Join(t.TempDir(), "folder")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatalf("mkdir folder: %v", err)
	}
	transportGit(t, folder, "init", "-b", "folder")
	transportGit(t, folder, "remote", "add", "origin", s.url)
	var tips []string
	for _, f := range files {
		tips = append(tips, transportCommit(t, folder, f.Path, f.Content, "folder "+f.Path))
	}
	transportGit(t, folder, "push", "origin", "folder")
	branch, err := s.rf.GetBranchByName(context.Background(), s.repo, "folder")
	if err != nil {
		t.Fatalf("GetBranchByName folder: %v", err)
	}
	return unrelatedPush{server: s, branch: branch, root: tips[0], second: tips[len(tips)-1]}
}

func openUnrelatedMergeRequest(t *testing.T, p unrelatedPush) models.MergeRequest {
	t.Helper()
	mr, err := p.server.rf.CreateMergeRequest(context.Background(), CreateMergeRequestRequest{
		RepositoryID:   p.server.repo,
		SourceBranchID: p.branch.ID,
		Title:          "bring in the folder",
	})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}
	return mr
}

func TestRepoFileUnrelatedHistory_DisjointPathsMergeAndKeepBothRoots(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()
	mainRoot := mergeLogSHAs(t, s.rf, s.main)
	p := pushUnrelatedFolder(t, s, []featureFile{
		{Path: "app/main.go", Content: "package main\n"},
		{Path: "app/util/util.go", Content: "package util\n"},
	})

	mr := openUnrelatedMergeRequest(t, p)
	done, err := s.rf.CompleteMergeRequest(ctx, mr.ID, "Grace")
	if err != nil {
		t.Fatalf("CompleteMergeRequest: %v", err)
	}
	if done.Status != models.MergeRequestStatusMerged {
		t.Fatalf("merge request status = %q, want merged", done.Status)
	}

	mergeAssertFiles(t, s.rf, s.main, map[string]string{
		"README.md":        "# widgets\n",
		"docs/guide.md":    "read me first\n",
		"app/main.go":      "package main\n",
		"app/util/util.go": "package util\n",
	})
	log := mergeLogSHAs(t, s.rf, s.main)
	for _, sha := range append(mainRoot, p.root, p.second) {
		if !mergeContains(log, sha) {
			t.Errorf("default log lacks %s: %v", sha, log)
		}
	}

	head := mergeReload(t, s.rf, s.main)
	m := transportManager(t, s.rf)
	parents, err := CommitParentIDs(m.db, m.tables, head.HeadCommitID)
	if err != nil {
		t.Fatalf("CommitParentIDs: %v", err)
	}
	if len(parents) != 2 {
		t.Fatalf("merge commit has %d parents, want 2", len(parents))
	}

	clone := s.clone(t)
	if got := readCloned(t, clone, "app/util/util.go"); got != "package util\n" {
		t.Errorf("cloned app/util/util.go = %q", got)
	}
	if got := readCloned(t, clone, "README.md"); got != "# widgets\n" {
		t.Errorf("cloned README.md = %q", got)
	}
	transportGit(t, clone, "fsck", "--no-dangling")
}

func TestRepoFileUnrelatedHistory_SamePathDifferentContentConflictsAndLeavesMainUntouched(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()
	before := mergeReload(t, s.rf, s.main)
	logBefore := mergeLogSHAs(t, s.rf, s.main)
	p := pushUnrelatedFolder(t, s, []featureFile{
		{Path: "README.md", Content: "# the folder\n"},
		{Path: "app/main.go", Content: "package main\n"},
	})

	if _, err := s.rf.MergeBranch(ctx, p.branch.ID); err == nil {
		t.Fatal("MergeBranch of unrelated histories with the same README.md succeeded")
	} else {
		var conflict *ErrMergeConflict
		if !errors.As(err, &conflict) {
			t.Fatalf("MergeBranch err = %v, want ErrMergeConflict", err)
		}
		if len(conflict.ConflictingFiles) != 1 || conflict.ConflictingFiles[0] != "README.md" {
			t.Fatalf("conflicting files = %v, want [README.md]", conflict.ConflictingFiles)
		}
	}

	mr := openUnrelatedMergeRequest(t, p)
	failed, err := s.rf.CompleteMergeRequest(ctx, mr.ID, "Grace")
	if !errors.Is(err, ErrMergeConflicted) {
		t.Fatalf("CompleteMergeRequest err = %v, want ErrMergeConflicted", err)
	}
	if failed.Status != models.MergeRequestStatusFailed {
		t.Errorf("merge request status = %q, want failed", failed.Status)
	}
	if !strings.Contains(failed.ErrorMessage, "README.md") {
		t.Errorf("error message %q does not name README.md", failed.ErrorMessage)
	}

	after := mergeReload(t, s.rf, s.main)
	if after.HeadCommitID != before.HeadCommitID || after.SHA != before.SHA {
		t.Errorf("default head moved from %s to %s", before.SHA, after.SHA)
	}
	if got := mergeLogSHAs(t, s.rf, s.main); len(got) != len(logBefore) {
		t.Errorf("default log changed: %v -> %v", logBefore, got)
	}
	mergeAssertFiles(t, s.rf, s.main, map[string]string{"README.md": "# widgets\n"})
	if _, err := s.rf.ReadFile(ctx, s.main, "app/main.go"); err == nil {
		t.Error("a half-merged file appeared on the default branch")
	}
}
