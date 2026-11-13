package mwanachamagit

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

const featureRepoName = "widgets"

type featureFile struct {
	Path    string
	Content string
}

func newFeatureRepo(t *testing.T) (*RepoFile, models.Repository, models.Branch) {
	t.Helper()
	rf := openRepoFileForTest(t, filepath.Join(t.TempDir(), "feature.db"))
	t.Cleanup(func() { _ = rf.Close() })
	ctx := context.Background()
	repo, err := rf.InitRepo(ctx, CreateRepoRequest{Name: featureRepoName})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, err := rf.ListBranches(ctx, repo.ID)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(branches) == 0 {
		t.Fatalf("repository %s has no default branch", repo.Name)
	}
	return rf, repo, branches[0]
}

func writeFeatureFiles(t *testing.T, rf *RepoFile, branchID string, files []featureFile) []models.Commit {
	t.Helper()
	ctx := context.Background()
	commits := make([]models.Commit, 0, len(files))
	for _, f := range files {
		commit, err := rf.WriteFile(ctx, WriteFileRequest{
			BranchID: branchID,
			Path:     f.Path,
			Content:  f.Content,
		})
		if err != nil {
			t.Fatalf("WriteFile %s: %v", f.Path, err)
		}
		commits = append(commits, commit)
	}
	return commits
}

func copyClosedRepoFile(t *testing.T, srcPath string) string {
	t.Helper()
	dstPath := filepath.Join(t.TempDir(), filepath.Base(srcPath))
	for _, suffix := range []string{"", "-wal", "-shm"} {
		copyFeatureFile(t, srcPath+suffix, dstPath+suffix, suffix != "")
	}
	return dstPath
}

func copyFeatureFile(t *testing.T, src, dst string, optional bool) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		if optional && os.IsNotExist(err) {
			return
		}
		t.Fatalf("open %s: %v", src, err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create %s: %v", dst, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatalf("copy %s: %v", src, err)
	}
}
