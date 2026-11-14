package mwanachamagit

import (
	"context"
	"errors"
	"testing"
)

func TestRepoFileDeleteFile_RemovesPathFromTreeAndMergesAsDelete(t *testing.T) {
	ctx := context.Background()
	rf, repo, defaultBranch := newFeatureRepo(t)
	writeFeatureFiles(t, rf, defaultBranch.ID, []featureFile{
		{Path: "keep.txt", Content: "keep"},
		{Path: "docs/drop.md", Content: "drop"},
		{Path: "docs/stay.md", Content: "stay"},
	})
	feature := repoFeatureBranch(t, rf, repo, "feature", "run-delete")

	deletion, err := rf.DeleteFile(ctx, DeleteFileRequest{BranchID: feature.ID, Path: "docs/drop.md"})
	if err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}

	if _, err := rf.ReadFile(ctx, feature.ID, "docs/drop.md"); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("ReadFile after delete: want ErrFileNotFound, got %v", err)
	}

	entries, err := rf.ListDirectory(ctx, feature.ID, "docs")
	if err != nil {
		t.Fatalf("ListDirectory docs: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "stay.md" {
		t.Fatalf("docs listing after delete = %+v, want only stay.md", entries)
	}

	diffs, err := rf.Diff(ctx, defaultBranch.ID, feature.ID)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(diffs) != 1 || diffs[0].Path != "docs/drop.md" || diffs[0].Operation != "deleted" {
		t.Fatalf("diff default..feature = %+v, want one deleted docs/drop.md", diffs)
	}

	merged, err := rf.MergeBranch(ctx, feature.ID)
	if err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	if merged.ID != defaultBranch.ID || merged.HeadCommitID != deletion.ID {
		t.Fatalf("merged = %+v, want default branch at %s", merged, deletion.ID)
	}
	if _, err := rf.ReadFile(ctx, defaultBranch.ID, "docs/drop.md"); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("ReadFile on default after merge: want ErrFileNotFound, got %v", err)
	}
	rootEntries, err := rf.ListDirectory(ctx, defaultBranch.ID, "")
	if err != nil {
		t.Fatalf("ListDirectory root: %v", err)
	}
	names := map[string]bool{}
	for _, e := range rootEntries {
		names[e.Name] = true
	}
	if !names["keep.txt"] || !names["docs"] || len(names) != 2 {
		t.Fatalf("root listing after merge = %+v", rootEntries)
	}
	if blob := blobOnBranch(t, rf, defaultBranch.ID, "docs/stay.md"); blob.Content != "stay" {
		t.Fatalf("docs/stay.md = %q", blob.Content)
	}
}

func TestRepoFileDeleteFile_DivergedMergeDeletesOnDefault(t *testing.T) {
	ctx := context.Background()
	rf, repo, defaultBranch := newFeatureRepo(t)
	writeFeatureFiles(t, rf, defaultBranch.ID, []featureFile{
		{Path: "drop.txt", Content: "drop"},
		{Path: "other.txt", Content: "other"},
	})
	feature := repoFeatureBranch(t, rf, repo, "feature", "run-delete-diverged")

	if _, err := rf.DeleteFile(ctx, DeleteFileRequest{BranchID: feature.ID, Path: "drop.txt"}); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	writeFeatureFiles(t, rf, defaultBranch.ID, []featureFile{{Path: "late.txt", Content: "late"}})

	if _, err := rf.MergeBranch(ctx, feature.ID); err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	if _, err := rf.ReadFile(ctx, defaultBranch.ID, "drop.txt"); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("drop.txt after diverged merge: want ErrFileNotFound, got %v", err)
	}
	for path, want := range map[string]string{"other.txt": "other", "late.txt": "late"} {
		if blob := blobOnBranch(t, rf, defaultBranch.ID, path); blob.Content != want {
			t.Fatalf("%s = %q, want %q", path, blob.Content, want)
		}
	}
}
