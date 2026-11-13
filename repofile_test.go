package mwanachamagit

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func openRepoFileForTest(t *testing.T, path string) *RepoFile {
	t.Helper()
	s, err := LoadSpec(filepath.Join("spec", "examples", "engineering.git.json"))
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	rf, err := OpenRepoFile(path, s, nil, nil, nil)
	if err != nil {
		t.Fatalf("OpenRepoFile %s: %v", path, err)
	}
	return rf
}

func closeRepoFileForTest(t *testing.T, rf *RepoFile) {
	t.Helper()
	if err := rf.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestRepoFile_CreatedRepoLivesInItsOwnFile(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "a.db")

	rf := openRepoFileForTest(t, path)
	if rf.Path() != path {
		t.Fatalf("Path() = %q, want %q", rf.Path(), path)
	}
	if _, err := rf.InitRepo(ctx, CreateRepoRequest{Name: "widgets"}); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	closeRepoFileForTest(t, rf)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Size() == 0 {
		t.Fatalf("expected %s to be non-empty", path)
	}

	reopened := openRepoFileForTest(t, path)
	defer closeRepoFileForTest(t, reopened)
	repos, err := reopened.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repos) != 1 || repos[0].Name != "widgets" {
		t.Fatalf("expected exactly the repository widgets, got %+v", repos)
	}
}

func TestRepoFile_OperationsWorkAgainstTheFile(t *testing.T) {
	ctx := context.Background()
	rf := openRepoFileForTest(t, filepath.Join(t.TempDir(), "a.db"))
	defer closeRepoFileForTest(t, rf)

	repo, err := rf.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, err := rf.ListBranches(ctx, repo.ID)
	if err != nil || len(branches) != 1 || !branches[0].IsDefault {
		t.Fatalf("expected one default branch, got %+v (err=%v)", branches, err)
	}
	defaultBranch := branches[0]

	feature, err := rf.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "feature/x"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	commit, err := rf.WriteFile(ctx, WriteFileRequest{
		BranchID: feature.ID,
		Path:     "docs/readme.md",
		Content:  "# widgets",
		Message:  "add readme",
	})
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	blob, err := rf.ReadFile(ctx, feature.ID, "docs/readme.md")
	if err != nil {
		t.Fatalf("ReadFile on feature: %v", err)
	}
	if blob.Content != "# widgets" {
		t.Fatalf("ReadFile content = %q, want %q", blob.Content, "# widgets")
	}

	featureLog, err := rf.Log(ctx, feature.ID, LogFilter{})
	if err != nil {
		t.Fatalf("Log on feature: %v", err)
	}
	if len(featureLog) != 1 || featureLog[0].Message != "add readme" {
		t.Fatalf("expected the single commit 'add readme' on feature, got %+v", featureLog)
	}

	merged, err := rf.MergeBranch(ctx, feature.ID)
	if err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	if merged.ID != defaultBranch.ID || merged.HeadCommitID != commit.ID {
		t.Fatalf("expected default branch head %s after merge, got %+v", commit.ID, merged)
	}

	onDefault, err := rf.ReadFile(ctx, defaultBranch.ID, "docs/readme.md")
	if err != nil {
		t.Fatalf("ReadFile on default after merge: %v", err)
	}
	if onDefault.Content != "# widgets" {
		t.Fatalf("default content = %q, want %q", onDefault.Content, "# widgets")
	}
	defaultLog, err := rf.Log(ctx, defaultBranch.ID, LogFilter{})
	if err != nil || len(defaultLog) != 1 || defaultLog[0].SHA != featureLog[0].SHA {
		t.Fatalf("expected default log to equal feature log, got %+v (err=%v)", defaultLog, err)
	}

	tag, err := rf.CreateTag(ctx, CreateTagRequest{RepositoryID: repo.ID, Name: "v1.0.0", CommitID: commit.ID})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	tags, err := rf.ListTags(ctx, repo.ID)
	if err != nil || len(tags) != 1 || tags[0].ID != tag.ID {
		t.Fatalf("expected the one tag v1.0.0, got %+v (err=%v)", tags, err)
	}

	if _, err := rf.CreateKeyword(ctx, CreateKeywordRequest{Name: "storage"}); err != nil {
		t.Fatalf("CreateKeyword: %v", err)
	}
	keywords, err := rf.ListKeywords(ctx, KeywordFilter{})
	if err != nil || len(keywords) != 1 || keywords[0].Name != "storage" {
		t.Fatalf("expected the one keyword storage, got %+v (err=%v)", keywords, err)
	}
}

func TestRepoFile_RefusesASecondRepository(t *testing.T) {
	ctx := context.Background()
	rf := openRepoFileForTest(t, filepath.Join(t.TempDir(), "a.db"))
	defer closeRepoFileForTest(t, rf)

	if _, err := rf.InitRepo(ctx, CreateRepoRequest{Name: "widgets"}); err != nil {
		t.Fatalf("InitRepo first: %v", err)
	}
	if _, err := rf.InitRepo(ctx, CreateRepoRequest{Name: "gadgets"}); !errors.Is(err, ErrRepoAlreadyExists) {
		t.Fatalf("expected ErrRepoAlreadyExists for a second repository, got %v", err)
	}
	repos, err := rf.ListRepositories(ctx)
	if err != nil || len(repos) != 1 {
		t.Fatalf("expected one repository, got %+v (err=%v)", repos, err)
	}
}

func TestRepoFile_CopiedFileReadsTheSameHistory(t *testing.T) {
	ctx := context.Background()
	original := filepath.Join(t.TempDir(), "a.db")

	rf := openRepoFileForTest(t, original)
	repo, err := rf.InitRepo(ctx, CreateRepoRequest{Name: "widgets"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, err := rf.ListBranches(ctx, repo.ID)
	if err != nil || len(branches) != 1 {
		t.Fatalf("ListBranches: %+v (err=%v)", branches, err)
	}
	branchID := branches[0].ID

	writes := []WriteFileRequest{
		{BranchID: branchID, Path: "one.txt", Content: "first", Message: "add one"},
		{BranchID: branchID, Path: "two.txt", Content: "second", Message: "add two"},
		{BranchID: branchID, Path: "one.txt", Content: "first, revised", Message: "revise one"},
	}
	for _, w := range writes {
		if _, err := rf.WriteFile(ctx, w); err != nil {
			t.Fatalf("WriteFile %s: %v", w.Message, err)
		}
	}
	wantLog, err := rf.Log(ctx, branchID, LogFilter{})
	if err != nil {
		t.Fatalf("Log before copy: %v", err)
	}
	if len(wantLog) < 3 {
		t.Fatalf("expected at least 3 commits, got %d", len(wantLog))
	}
	closeRepoFileForTest(t, rf)

	bytesOfOriginal, err := os.ReadFile(original)
	if err != nil {
		t.Fatalf("read original: %v", err)
	}
	copied := filepath.Join(t.TempDir(), "copy-of-widgets.db")
	if err := os.WriteFile(copied, bytesOfOriginal, 0o600); err != nil {
		t.Fatalf("write copy: %v", err)
	}

	cp := openRepoFileForTest(t, copied)
	defer closeRepoFileForTest(t, cp)

	gotLog, err := cp.Log(ctx, branchID, LogFilter{})
	if err != nil {
		t.Fatalf("Log on copy: %v", err)
	}
	if len(gotLog) != len(wantLog) {
		t.Fatalf("copy has %d commits, want %d", len(gotLog), len(wantLog))
	}
	for i := range wantLog {
		if gotLog[i].SHA != wantLog[i].SHA || gotLog[i].Message != wantLog[i].Message {
			t.Fatalf("commit %d differs: copy %+v, original %+v", i, gotLog[i], wantLog[i])
		}
	}

	one, err := cp.ReadFile(ctx, branchID, "one.txt")
	if err != nil || one.Content != "first, revised" {
		t.Fatalf("one.txt on copy = %q (err=%v), want %q", one.Content, err, "first, revised")
	}
	two, err := cp.ReadFile(ctx, branchID, "two.txt")
	if err != nil || two.Content != "second" {
		t.Fatalf("two.txt on copy = %q (err=%v), want %q", two.Content, err, "second")
	}
}

func TestGitPathHasNoPostgresDriver(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", ".")
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, errOut.String())
	}
	forbidden := []string{"gorm.io/driver/postgres", "jackc/pgx", "lib/pq"}
	for _, importPath := range strings.Fields(out.String()) {
		for _, name := range forbidden {
			if strings.Contains(importPath, name) {
				t.Errorf("package depends on %s", importPath)
			}
		}
	}
}
