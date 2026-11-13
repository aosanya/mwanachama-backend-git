package mwanachamagit

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

type copyFixture struct {
	repo          models.Repository
	defaultBranch models.Branch
	openBranch    models.Branch
	mergedCommits []models.Commit
	tag           models.Tag
	openRequest   models.MergeRequest
	parent        models.Keyword
	child         models.Keyword
	readmeBlobID  string
	apiBlobID     string
}

func buildCopyFixture(t *testing.T, rf *RepoFile, defaultBranch models.Branch, repo models.Repository) copyFixture {
	t.Helper()
	ctx := context.Background()
	fx := copyFixture{repo: repo, defaultBranch: defaultBranch}

	fx.mergedCommits = writeFeatureFiles(t, rf, defaultBranch.ID, []featureFile{
		{Path: "README.md", Content: "# widgets\n"},
		{Path: "docs/notes.txt", Content: "scratch\n"},
		{Path: "src/api.go", Content: "package src\n"},
		{Path: "README.md", Content: "# widgets\nedited\n"},
	})
	if _, err := rf.DeleteFile(ctx, DeleteFileRequest{BranchID: defaultBranch.ID, Path: "docs/notes.txt"}); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}

	merged, err := rf.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "feature-merged"})
	if err != nil {
		t.Fatalf("CreateBranch feature-merged: %v", err)
	}
	writeFeatureFiles(t, rf, merged.ID, []featureFile{
		{Path: "src/feature.go", Content: "package src\n\nfunc Feature() {}\n"},
		{Path: "src/api.go", Content: "package src\n\nfunc API() {}\n"},
	})
	if _, err := rf.MergeBranch(ctx, merged.ID); err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}

	commits, err := rf.Log(ctx, defaultBranch.ID, LogFilter{})
	if err != nil || len(commits) == 0 {
		t.Fatalf("Log before tag: %v (%d)", err, len(commits))
	}
	fx.tag, err = rf.CreateTag(ctx, CreateTagRequest{RepositoryID: repo.ID, Name: "v1", CommitID: fx.mergedCommits[0].ID, Message: "first"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	fx.openBranch, err = rf.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "feature-open"})
	if err != nil {
		t.Fatalf("CreateBranch feature-open: %v", err)
	}
	writeFeatureFiles(t, rf, fx.openBranch.ID, []featureFile{{Path: "src/open.go", Content: "package src\n"}})
	fx.openRequest, err = rf.CreateMergeRequest(ctx, CreateMergeRequestRequest{
		RepositoryID:   repo.ID,
		Title:          "open change",
		SourceBranchID: fx.openBranch.ID,
		TargetBranchID: defaultBranch.ID,
	})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}

	fx.parent, err = rf.CreateKeyword(ctx, CreateKeywordRequest{Name: "platform", Scope: "agency"})
	if err != nil {
		t.Fatalf("CreateKeyword parent: %v", err)
	}
	fx.child, err = rf.CreateKeyword(ctx, CreateKeywordRequest{Name: "storage", Scope: "agency", ParentID: fx.parent.ID})
	if err != nil {
		t.Fatalf("CreateKeyword child: %v", err)
	}

	readme, err := rf.ReadFile(ctx, defaultBranch.ID, "README.md")
	if err != nil {
		t.Fatalf("ReadFile README: %v", err)
	}
	api, err := rf.ReadFile(ctx, defaultBranch.ID, "src/api.go")
	if err != nil {
		t.Fatalf("ReadFile api: %v", err)
	}
	fx.readmeBlobID, fx.apiBlobID = readme.ID, api.ID

	if err := rf.CreateEdge(ctx, CreateEdgeRequest{
		BranchID: defaultBranch.ID, FromEntityID: fx.readmeBlobID, ToEntityID: fx.child.ID, RelationshipName: "tagged_with",
	}); err != nil {
		t.Fatalf("CreateEdge tagged_with: %v", err)
	}
	if err := rf.CreateEdge(ctx, CreateEdgeRequest{
		BranchID: defaultBranch.ID, FromEntityID: fx.readmeBlobID, ToEntityID: fx.apiBlobID,
		RelationshipName: "references", Properties: map[string]any{"descriptor": "depends_on"},
	}); err != nil {
		t.Fatalf("CreateEdge references: %v", err)
	}
	return fx
}

type copyObservation struct {
	repo          models.Repository
	branches      []models.Branch
	tags          []models.Tag
	requests      []models.MergeRequest
	keywords      []models.Keyword
	childKeywords []models.Keyword
	tree          []KeywordTreeNode
	neighborhood  GraphResult
	log           []CommitEntry
	diff          []FileDiff
	files         map[string]string
	rootEntries   []FileEntry
	srcEntries    []FileEntry
}

func observeCopyFeatures(t *testing.T, rf *RepoFile, fx copyFixture) copyObservation {
	t.Helper()
	ctx := context.Background()
	var (
		obs copyObservation
		err error
	)
	step := func(name string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	obs.repo, err = rf.GetRepositoryByName(ctx, featureRepoName)
	step("GetRepositoryByName", err)
	obs.branches, err = rf.ListBranches(ctx, fx.repo.ID)
	step("ListBranches", err)
	obs.tags, err = rf.ListTags(ctx, fx.repo.ID)
	step("ListTags", err)
	obs.requests, err = rf.ListMergeRequests(ctx, MergeRequestFilter{RepositoryID: fx.repo.ID})
	step("ListMergeRequests", err)
	obs.keywords, err = rf.ListKeywords(ctx, KeywordFilter{})
	step("ListKeywords roots", err)
	obs.childKeywords, err = rf.ListKeywords(ctx, KeywordFilter{ParentID: fx.parent.ID})
	step("ListKeywords children", err)
	obs.tree, err = rf.GetKeywordTree(ctx, fx.parent.ID)
	step("GetKeywordTree", err)
	obs.neighborhood, err = rf.GetNeighborhood(ctx, fx.defaultBranch.ID, fx.readmeBlobID, 2)
	step("GetNeighborhood", err)
	obs.log, err = rf.Log(ctx, fx.defaultBranch.ID, LogFilter{})
	step("Log", err)
	obs.diff, err = rf.Diff(ctx, fx.mergedCommits[0].ID, fx.defaultBranch.ID)
	step("Diff", err)
	sort.Slice(obs.diff, func(i, j int) bool { return obs.diff[i].Path < obs.diff[j].Path })
	obs.files = map[string]string{}
	for _, path := range []string{"README.md", "src/api.go", "src/feature.go"} {
		blob, err := rf.ReadFile(ctx, fx.defaultBranch.ID, path)
		step("ReadFile "+path, err)
		obs.files[path] = blob.Content
	}
	obs.rootEntries, err = rf.ListDirectory(ctx, fx.defaultBranch.ID, "")
	step("ListDirectory root", err)
	obs.srcEntries, err = rf.ListDirectory(ctx, fx.defaultBranch.ID, "src")
	step("ListDirectory src", err)
	return obs
}

func TestRepoFileFeatures_CopiedFileKeepsEveryFeature(t *testing.T) {
	ctx := context.Background()
	rf, repo, defaultBranch := newFeatureRepo(t)
	fx := buildCopyFixture(t, rf, defaultBranch, repo)
	original := observeCopyFeatures(t, rf, fx)
	originalPath := rf.Path()
	if err := rf.Close(); err != nil {
		t.Fatalf("Close original: %v", err)
	}

	copyPath := copyClosedRepoFile(t, originalPath)
	copied := openRepoFileForTest(t, copyPath)
	t.Cleanup(func() { _ = copied.Close() })
	got := observeCopyFeatures(t, copied, fx)

	if len(original.log) < 6 {
		t.Fatalf("fixture too thin: log has %d commits", len(original.log))
	}
	if len(original.tags) != 1 || len(original.requests) != 1 || len(original.keywords) != 1 || len(original.childKeywords) != 1 {
		t.Fatalf("fixture too thin: tags=%d requests=%d keywords=%d children=%d", len(original.tags), len(original.requests), len(original.keywords), len(original.childKeywords))
	}
	if len(original.neighborhood.Edges) < 2 {
		t.Fatalf("fixture too thin: neighborhood has %d edges", len(original.neighborhood.Edges))
	}

	cases := []struct {
		name string
		want any
		got  any
	}{
		{"GetRepositoryByName", original.repo, got.repo},
		{"ListBranches", original.branches, got.branches},
		{"ListTags", original.tags, got.tags},
		{"ListMergeRequests", original.requests, got.requests},
		{"ListKeywords", original.keywords, got.keywords},
		{"ListKeywordsChildren", original.childKeywords, got.childKeywords},
		{"GetKeywordTree", original.tree, got.tree},
		{"GetNeighborhood", original.neighborhood, got.neighborhood},
		{"Log", original.log, got.log},
		{"Diff", original.diff, got.diff},
		{"ReadFile", original.files, got.files},
		{"ListDirectoryRoot", original.rootEntries, got.rootEntries},
		{"ListDirectorySrc", original.srcEntries, got.srcEntries},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !reflect.DeepEqual(c.want, c.got) {
				t.Fatalf("copy differs\n original: %+v\n copy:     %+v", c.want, c.got)
			}
		})
	}

	t.Run("OpenMergeRequestStillOpen", func(t *testing.T) {
		if len(got.requests) != 1 || got.requests[0].ID != fx.openRequest.ID || got.requests[0].Status != "open" {
			t.Fatalf("open merge request not preserved: %+v", got.requests)
		}
	})

	t.Run("KeywordTreeReturnsChildUnderParent", func(t *testing.T) {
		if len(got.tree) != 1 || got.tree[0].Keyword.ID != fx.child.ID || got.tree[0].Keyword.ParentID != fx.parent.ID {
			t.Fatalf("keyword tree lost its child: %+v", got.tree)
		}
	})

	t.Run("WriteOnCopyLeavesOriginalUnchanged", func(t *testing.T) {
		before := len(got.log)
		commit, err := copied.WriteFile(ctx, WriteFileRequest{BranchID: fx.defaultBranch.ID, Path: "after-copy.txt", Content: "only on the copy\n"})
		if err != nil {
			t.Fatalf("WriteFile on copy: %v", err)
		}
		if err := copied.Close(); err != nil {
			t.Fatalf("Close copy: %v", err)
		}

		reopenedCopy := openRepoFileForTest(t, copyPath)
		defer reopenedCopy.Close()
		copyLog, err := reopenedCopy.Log(ctx, fx.defaultBranch.ID, LogFilter{})
		if err != nil {
			t.Fatalf("Log copy: %v", err)
		}
		if len(copyLog) != before+1 || copyLog[0].SHA != commit.SHA {
			t.Fatalf("copy did not advance by one commit: before=%d after=%d head=%v want=%s", before, len(copyLog), copyLog[0].SHA, commit.SHA)
		}

		reopenedOriginal := openRepoFileForTest(t, originalPath)
		defer reopenedOriginal.Close()
		originalLog, err := reopenedOriginal.Log(ctx, fx.defaultBranch.ID, LogFilter{})
		if err != nil {
			t.Fatalf("Log original: %v", err)
		}
		if !reflect.DeepEqual(original.log, originalLog) {
			t.Fatalf("original changed after writing to the copy\n before: %+v\n after:  %+v", original.log, originalLog)
		}
		if _, err := reopenedOriginal.ReadFile(ctx, fx.defaultBranch.ID, "after-copy.txt"); err == nil {
			t.Fatalf("original unexpectedly contains after-copy.txt")
		}
	})
}
