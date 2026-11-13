package mwanachamagit

import (
	"context"
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func repoFeatureBranch(t *testing.T, rf *RepoFile, repo models.Repository, name, runID string) models.Branch {
	t.Helper()
	branch, err := rf.CreateBranch(context.Background(), CreateBranchRequest{
		RepositoryID:  repo.ID,
		Name:          name,
		WorkflowRunID: runID,
	})
	if err != nil {
		t.Fatalf("CreateBranch %s: %v", name, err)
	}
	return branch
}

func repoFeatureBranchNames(branches []models.Branch) map[string]bool {
	names := make(map[string]bool, len(branches))
	for _, b := range branches {
		names[b.Name] = true
	}
	return names
}

func TestRepoFileFeatures_InitRepo(t *testing.T) {
	rf, repo, defaultBranch := newFeatureRepo(t)
	if repo.ID == "" {
		t.Fatal("repository has no ID")
	}
	if repo.Name != featureRepoName {
		t.Fatalf("Name = %q, want %q", repo.Name, featureRepoName)
	}
	if repo.DefaultBranch != "main" {
		t.Fatalf("DefaultBranch = %q, want main", repo.DefaultBranch)
	}
	if repo.CreatedAt == "" || repo.UpdatedAt == "" {
		t.Fatalf("timestamps not set: %+v", repo)
	}
	if defaultBranch.Name != "main" || !defaultBranch.IsDefault || defaultBranch.RepositoryID != repo.ID {
		t.Fatalf("default branch = %+v", defaultBranch)
	}
	if defaultBranch.HeadCommitID != "" {
		t.Fatalf("fresh default branch has head %q", defaultBranch.HeadCommitID)
	}
	_, err := rf.InitRepo(context.Background(), CreateRepoRequest{Name: "second"})
	if !errors.Is(err, ErrRepoAlreadyExists) {
		t.Fatalf("second InitRepo err = %v, want ErrRepoAlreadyExists", err)
	}
	_, err = rf.InitRepo(context.Background(), CreateRepoRequest{Name: featureRepoName})
	if !errors.Is(err, ErrRepoAlreadyExists) {
		t.Fatalf("duplicate-name InitRepo err = %v, want ErrRepoAlreadyExists", err)
	}
}

func TestRepoFileFeatures_InitRepoCustomDefaultBranchAndDescription(t *testing.T) {
	rf := openRepoFileForTest(t, t.TempDir()+"/custom.db")
	t.Cleanup(func() { _ = rf.Close() })
	ctx := context.Background()
	repo, err := rf.InitRepo(ctx, CreateRepoRequest{Name: "gadgets", Description: "all the gadgets", DefaultBranch: "trunk"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	if repo.Description != "all the gadgets" || repo.DefaultBranch != "trunk" {
		t.Fatalf("repo = %+v", repo)
	}
	branches, err := rf.ListBranches(ctx, repo.ID)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(branches) != 1 || branches[0].Name != "trunk" || !branches[0].IsDefault {
		t.Fatalf("branches = %+v", branches)
	}
	got, err := rf.GetRepository(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if got.Description != "all the gadgets" || got.DefaultBranch != "trunk" {
		t.Fatalf("persisted repo = %+v", got)
	}
}

func TestRepoFileFeatures_ListRepositories(t *testing.T) {
	rf := openRepoFileForTest(t, t.TempDir()+"/list.db")
	t.Cleanup(func() { _ = rf.Close() })
	ctx := context.Background()
	empty, err := rf.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("ListRepositories empty: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty file lists %d repositories", len(empty))
	}
	repo, err := rf.InitRepo(ctx, CreateRepoRequest{Name: featureRepoName})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	repos, err := rf.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repos) != 1 || repos[0].ID != repo.ID || repos[0].Name != featureRepoName {
		t.Fatalf("repos = %+v", repos)
	}
}

func TestRepoFileFeatures_ListRepositorySummaries(t *testing.T) {
	rf, repo, defaultBranch := newFeatureRepo(t)
	summaries, err := rf.ListRepositorySummaries(context.Background())
	if err != nil {
		t.Fatalf("ListRepositorySummaries: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summaries = %+v", summaries)
	}
	want := models.RepositorySummary{
		ID:              repo.ID,
		Name:            featureRepoName,
		DefaultBranch:   "main",
		DefaultBranchID: defaultBranch.ID,
	}
	if summaries[0] != want {
		t.Fatalf("summary = %+v, want %+v", summaries[0], want)
	}
}

func TestRepoFileFeatures_ListRepositorySummariesEmpty(t *testing.T) {
	rf := openRepoFileForTest(t, t.TempDir()+"/empty.db")
	t.Cleanup(func() { _ = rf.Close() })
	summaries, err := rf.ListRepositorySummaries(context.Background())
	if err != nil {
		t.Fatalf("ListRepositorySummaries: %v", err)
	}
	if summaries == nil || len(summaries) != 0 {
		t.Fatalf("summaries = %#v, want empty non-nil", summaries)
	}
}

func TestRepoFileFeatures_GetRepository(t *testing.T) {
	rf, repo, _ := newFeatureRepo(t)
	ctx := context.Background()
	got, err := rf.GetRepository(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if got.ID != repo.ID || got.Name != featureRepoName || got.DefaultBranch != "main" {
		t.Fatalf("got = %+v", got)
	}
	_, err = rf.GetRepository(ctx, "no-such-repo")
	if !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("missing repo err = %v, want ErrRepoNotInitialised", err)
	}
}

func TestRepoFileFeatures_GetRepositoryByName(t *testing.T) {
	rf, repo, _ := newFeatureRepo(t)
	ctx := context.Background()
	got, err := rf.GetRepositoryByName(ctx, featureRepoName)
	if err != nil {
		t.Fatalf("GetRepositoryByName: %v", err)
	}
	if got.ID != repo.ID {
		t.Fatalf("got ID %q, want %q", got.ID, repo.ID)
	}
	_, err = rf.GetRepositoryByName(ctx, "missing")
	if !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("missing name err = %v, want ErrRepoNotInitialised", err)
	}
}

func assertRepositoryGone(t *testing.T, rf *RepoFile, repo models.Repository, branch models.Branch) {
	t.Helper()
	ctx := context.Background()
	if _, err := rf.GetRepository(ctx, repo.ID); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("GetRepository after removal err = %v", err)
	}
	if _, err := rf.GetRepositoryByName(ctx, repo.Name); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("GetRepositoryByName after removal err = %v", err)
	}
	if _, err := rf.ListBranches(ctx, repo.ID); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("ListBranches after removal err = %v", err)
	}
	if _, err := rf.ListTags(ctx, repo.ID); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("ListTags after removal err = %v", err)
	}
	if _, err := rf.GetBranch(ctx, branch.ID); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("GetBranch after removal err = %v", err)
	}
	repos, err := rf.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repos) != 0 {
		t.Fatalf("repositories after removal = %+v", repos)
	}
	summaries, err := rf.ListRepositorySummaries(ctx)
	if err != nil {
		t.Fatalf("ListRepositorySummaries: %v", err)
	}
	if len(summaries) != 0 {
		t.Fatalf("summaries after removal = %+v", summaries)
	}
}

func TestRepoFileFeatures_DeleteRepo(t *testing.T) {
	rf, repo, branch := newFeatureRepo(t)
	ctx := context.Background()
	commits := writeFeatureFiles(t, rf, branch.ID, []featureFile{{Path: "a.txt", Content: "a"}})
	if _, err := rf.CreateTag(ctx, CreateTagRequest{RepositoryID: repo.ID, Name: "v1", CommitID: commits[0].ID}); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := rf.DeleteRepo(ctx, repo.ID); err != nil {
		t.Fatalf("DeleteRepo: %v", err)
	}
	assertRepositoryGone(t, rf, repo, branch)
	if err := rf.DeleteRepo(ctx, repo.ID); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("second DeleteRepo err = %v, want ErrRepoNotInitialised", err)
	}
	if _, err := rf.InitRepo(ctx, CreateRepoRequest{Name: "again"}); !errors.Is(err, ErrRepoAlreadyExists) {
		t.Fatalf("InitRepo after DeleteRepo err = %v, want ErrRepoAlreadyExists (single-repository file keeps its archived row)", err)
	}
}

func TestRepoFileFeatures_DeleteRepoUnknown(t *testing.T) {
	rf, _, _ := newFeatureRepo(t)
	if err := rf.DeleteRepo(context.Background(), "no-such-repo"); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("DeleteRepo unknown err = %v, want ErrRepoNotInitialised", err)
	}
}

func TestRepoFileFeatures_PurgeRepo(t *testing.T) {
	rf, repo, branch := newFeatureRepo(t)
	ctx := context.Background()
	if err := rf.PurgeRepo(ctx, repo.ID); err != nil {
		t.Fatalf("PurgeRepo: %v", err)
	}
	assertRepositoryGone(t, rf, repo, branch)
	if err := rf.PurgeRepo(ctx, repo.ID); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("second PurgeRepo err = %v, want ErrRepoNotInitialised", err)
	}
	if err := rf.PurgeRepo(ctx, "no-such-repo"); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("PurgeRepo unknown err = %v, want ErrRepoNotInitialised", err)
	}
}

func TestRepoFileFeatures_CreateBranch(t *testing.T) {
	rf, repo, defaultBranch := newFeatureRepo(t)
	ctx := context.Background()
	commits := writeFeatureFiles(t, rf, defaultBranch.ID, []featureFile{{Path: "a.txt", Content: "a"}})
	feature, err := rf.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "feature/one", WorkflowRunID: "run-1"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if feature.ID == "" || feature.Name != "feature/one" || feature.IsDefault || feature.RepositoryID != repo.ID {
		t.Fatalf("feature = %+v", feature)
	}
	if feature.WorkflowRunID != "run-1" {
		t.Fatalf("WorkflowRunID = %q", feature.WorkflowRunID)
	}
	if feature.HeadCommitID != commits[0].ID || feature.SHA != commits[0].SHA {
		t.Fatalf("feature head = %q/%q, want %q/%q", feature.HeadCommitID, feature.SHA, commits[0].ID, commits[0].SHA)
	}
	file, err := rf.ReadFile(ctx, feature.ID, "a.txt")
	if err != nil {
		t.Fatalf("ReadFile on new branch: %v", err)
	}
	if file.Content != "a" {
		t.Fatalf("content = %q", file.Content)
	}
}

func TestRepoFileFeatures_CreateBranchFromBranch(t *testing.T) {
	rf, repo, defaultBranch := newFeatureRepo(t)
	ctx := context.Background()
	base := repoFeatureBranch(t, rf, repo, "base", "")
	commits := writeFeatureFiles(t, rf, base.ID, []featureFile{{Path: "b.txt", Content: "b"}})
	child, err := rf.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "child", FromBranchID: base.ID})
	if err != nil {
		t.Fatalf("CreateBranch from branch: %v", err)
	}
	if child.HeadCommitID != commits[0].ID {
		t.Fatalf("child head = %q, want %q", child.HeadCommitID, commits[0].ID)
	}
	fromDefault, err := rf.GetBranch(ctx, defaultBranch.ID)
	if err != nil {
		t.Fatalf("GetBranch: %v", err)
	}
	if fromDefault.HeadCommitID != "" {
		t.Fatalf("default branch moved: %+v", fromDefault)
	}
}

func TestRepoFileFeatures_CreateBranchErrors(t *testing.T) {
	rf, repo, _ := newFeatureRepo(t)
	ctx := context.Background()
	repoFeatureBranch(t, rf, repo, "dup", "")
	if _, err := rf.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "dup"}); !errors.Is(err, ErrBranchExists) {
		t.Fatalf("duplicate branch err = %v, want ErrBranchExists", err)
	}
	if _, err := rf.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "main"}); !errors.Is(err, ErrBranchExists) {
		t.Fatalf("branch named like default err = %v, want ErrBranchExists", err)
	}
	if _, err := rf.CreateBranch(ctx, CreateBranchRequest{RepositoryID: "nope", Name: "x"}); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("unknown repo err = %v, want ErrRepoNotInitialised", err)
	}
	if _, err := rf.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "x", FromBranchID: "nope"}); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("unknown source err = %v, want ErrBranchNotFound", err)
	}
}

func TestRepoFileFeatures_GetBranch(t *testing.T) {
	rf, repo, defaultBranch := newFeatureRepo(t)
	ctx := context.Background()
	feature := repoFeatureBranch(t, rf, repo, "feature", "run-7")
	got, err := rf.GetBranch(ctx, feature.ID)
	if err != nil {
		t.Fatalf("GetBranch: %v", err)
	}
	if got.ID != feature.ID || got.Name != "feature" || got.WorkflowRunID != "run-7" || got.IsDefault {
		t.Fatalf("got = %+v", got)
	}
	def, err := rf.GetBranch(ctx, defaultBranch.ID)
	if err != nil {
		t.Fatalf("GetBranch default: %v", err)
	}
	if !def.IsDefault || def.Name != "main" {
		t.Fatalf("default = %+v", def)
	}
	if _, err := rf.GetBranch(ctx, "nope"); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("missing err = %v, want ErrBranchNotFound", err)
	}
}

func TestRepoFileFeatures_GetBranchByName(t *testing.T) {
	rf, repo, defaultBranch := newFeatureRepo(t)
	ctx := context.Background()
	feature := repoFeatureBranch(t, rf, repo, "task/abc-001", "")
	got, err := rf.GetBranchByName(ctx, repo.ID, "task/abc-001")
	if err != nil {
		t.Fatalf("GetBranchByName: %v", err)
	}
	if got.ID != feature.ID {
		t.Fatalf("ID = %q, want %q", got.ID, feature.ID)
	}
	def, err := rf.GetBranchByName(ctx, repo.ID, "main")
	if err != nil {
		t.Fatalf("GetBranchByName main: %v", err)
	}
	if def.ID != defaultBranch.ID {
		t.Fatalf("main ID = %q, want %q", def.ID, defaultBranch.ID)
	}
	if _, err := rf.GetBranchByName(ctx, repo.ID, "missing"); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("missing name err = %v, want ErrBranchNotFound", err)
	}
	if _, err := rf.GetBranchByName(ctx, "other-repo", "main"); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("wrong repo err = %v, want ErrBranchNotFound", err)
	}
}

func TestRepoFileFeatures_ListBranches(t *testing.T) {
	rf, repo, _ := newFeatureRepo(t)
	ctx := context.Background()
	repoFeatureBranch(t, rf, repo, "one", "")
	repoFeatureBranch(t, rf, repo, "two", "")
	branches, err := rf.ListBranches(ctx, repo.ID)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(branches) != 3 {
		t.Fatalf("branches = %+v", branches)
	}
	names := repoFeatureBranchNames(branches)
	for _, want := range []string{"main", "one", "two"} {
		if !names[want] {
			t.Fatalf("missing branch %q in %v", want, names)
		}
	}
	if _, err := rf.ListBranches(ctx, "nope"); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("unknown repo err = %v, want ErrRepoNotInitialised", err)
	}
}

func TestRepoFileFeatures_ListBranchesFiltered(t *testing.T) {
	rf, repo, _ := newFeatureRepo(t)
	ctx := context.Background()
	repoFeatureBranch(t, rf, repo, "a1", "run-a")
	repoFeatureBranch(t, rf, repo, "a2", "run-a")
	repoFeatureBranch(t, rf, repo, "b1", "run-b")
	repoFeatureBranch(t, rf, repo, "plain", "")

	all, err := rf.ListBranchesFiltered(ctx, repo.ID, BranchFilter{})
	if err != nil {
		t.Fatalf("ListBranchesFiltered empty filter: %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("empty filter returned %d branches, want 5", len(all))
	}

	runA, err := rf.ListBranchesFiltered(ctx, repo.ID, BranchFilter{WorkflowRunID: "run-a"})
	if err != nil {
		t.Fatalf("ListBranchesFiltered run-a: %v", err)
	}
	names := repoFeatureBranchNames(runA)
	if len(runA) != 2 || !names["a1"] || !names["a2"] {
		t.Fatalf("run-a branches = %+v", runA)
	}

	crossRepo, err := rf.ListBranchesFiltered(ctx, "", BranchFilter{WorkflowRunID: "run-b"})
	if err != nil {
		t.Fatalf("ListBranchesFiltered cross-repository: %v", err)
	}
	if len(crossRepo) != 1 || crossRepo[0].Name != "b1" {
		t.Fatalf("cross-repository run-b = %+v", crossRepo)
	}

	none, err := rf.ListBranchesFiltered(ctx, repo.ID, BranchFilter{WorkflowRunID: "run-missing"})
	if err != nil {
		t.Fatalf("ListBranchesFiltered unknown run: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("unknown run returned %+v", none)
	}

	if _, err := rf.ListBranchesFiltered(ctx, "nope", BranchFilter{WorkflowRunID: "run-a"}); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("unknown repo err = %v, want ErrRepoNotInitialised", err)
	}
}

func TestRepoFileFeatures_ListBranchesFilteredSkipsDeleted(t *testing.T) {
	rf, repo, _ := newFeatureRepo(t)
	ctx := context.Background()
	keep := repoFeatureBranch(t, rf, repo, "keep", "run-x")
	drop := repoFeatureBranch(t, rf, repo, "drop", "run-x")
	if err := rf.DeleteBranch(ctx, drop.ID); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	got, err := rf.ListBranchesFiltered(ctx, "", BranchFilter{WorkflowRunID: "run-x"})
	if err != nil {
		t.Fatalf("ListBranchesFiltered: %v", err)
	}
	if len(got) != 1 || got[0].ID != keep.ID {
		t.Fatalf("got = %+v", got)
	}
}

func TestRepoFileFeatures_DeleteBranch(t *testing.T) {
	rf, repo, defaultBranch := newFeatureRepo(t)
	ctx := context.Background()
	feature := repoFeatureBranch(t, rf, repo, "feature", "")
	if err := rf.DeleteBranch(ctx, feature.ID); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	if _, err := rf.GetBranch(ctx, feature.ID); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("GetBranch after delete err = %v", err)
	}
	if _, err := rf.GetBranchByName(ctx, repo.ID, "feature"); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("GetBranchByName after delete err = %v", err)
	}
	branches, err := rf.ListBranches(ctx, repo.ID)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(branches) != 1 || branches[0].ID != defaultBranch.ID {
		t.Fatalf("branches after delete = %+v", branches)
	}
	if err := rf.DeleteBranch(ctx, feature.ID); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("second DeleteBranch err = %v, want ErrBranchNotFound", err)
	}
	if err := rf.DeleteBranch(ctx, defaultBranch.ID); !errors.Is(err, ErrDefaultBranchDeleteForbidden) {
		t.Fatalf("DeleteBranch default err = %v, want ErrDefaultBranchDeleteForbidden", err)
	}
	reused := repoFeatureBranch(t, rf, repo, "feature", "")
	if reused.ID == feature.ID {
		t.Fatal("recreated branch reused the deleted branch ID")
	}
}

func TestRepoFileFeatures_MergeBranch(t *testing.T) {
	rf, repo, defaultBranch := newFeatureRepo(t)
	ctx := context.Background()
	feature := repoFeatureBranch(t, rf, repo, "feature", "run-m")
	commits := writeFeatureFiles(t, rf, feature.ID, []featureFile{
		{Path: "docs/readme.md", Content: "hello"},
		{Path: "src/main.go", Content: "package main"},
	})
	last := commits[len(commits)-1]

	merged, err := rf.MergeBranch(ctx, feature.ID)
	if err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	if merged.ID != defaultBranch.ID || !merged.IsDefault {
		t.Fatalf("merged = %+v, want the default branch", merged)
	}
	if merged.HeadCommitID != last.ID || merged.SHA != last.SHA {
		t.Fatalf("default head = %q/%q, want %q/%q", merged.HeadCommitID, merged.SHA, last.ID, last.SHA)
	}
	for path, want := range map[string]string{"docs/readme.md": "hello", "src/main.go": "package main"} {
		blob, err := rf.ReadFile(ctx, defaultBranch.ID, path)
		if err != nil {
			t.Fatalf("ReadFile %s on default: %v", path, err)
		}
		if blob.Content != want {
			t.Fatalf("%s = %q, want %q", path, blob.Content, want)
		}
	}
	reread, err := rf.GetBranch(ctx, defaultBranch.ID)
	if err != nil {
		t.Fatalf("GetBranch: %v", err)
	}
	if reread.HeadCommitID != last.ID {
		t.Fatalf("persisted default head = %q, want %q", reread.HeadCommitID, last.ID)
	}
}

func TestRepoFileFeatures_MergeBranchWithoutCommitsLeavesDefaultUntouched(t *testing.T) {
	rf, repo, defaultBranch := newFeatureRepo(t)
	empty := repoFeatureBranch(t, rf, repo, "empty", "")
	merged, err := rf.MergeBranch(context.Background(), empty.ID)
	if err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	if merged.ID != defaultBranch.ID || merged.HeadCommitID != "" {
		t.Fatalf("merged = %+v", merged)
	}
}

func TestRepoFileFeatures_MergeBranchUnknown(t *testing.T) {
	rf, _, _ := newFeatureRepo(t)
	if _, err := rf.MergeBranch(context.Background(), "nope"); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("MergeBranch unknown err = %v, want ErrBranchNotFound", err)
	}
}

func TestRepoFileFeatures_CreateTag(t *testing.T) {
	rf, repo, branch := newFeatureRepo(t)
	ctx := context.Background()
	commits := writeFeatureFiles(t, rf, branch.ID, []featureFile{{Path: "a.txt", Content: "a"}})
	tag, err := rf.CreateTag(ctx, CreateTagRequest{
		RepositoryID: repo.ID,
		Name:         "v1.0.0",
		CommitID:     commits[0].ID,
		Message:      "first release",
		TaggerName:   "Tony",
	})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if tag.ID == "" || tag.Name != "v1.0.0" || tag.RepositoryID != repo.ID {
		t.Fatalf("tag = %+v", tag)
	}
	if tag.CommitID != commits[0].ID || tag.SHA != commits[0].SHA {
		t.Fatalf("tag points at %q/%q, want %q/%q", tag.CommitID, tag.SHA, commits[0].ID, commits[0].SHA)
	}
	if tag.Message != "first release" || tag.TaggerName != "Tony" || tag.TaggerAt == "" {
		t.Fatalf("tag annotation = %+v", tag)
	}
}

func TestRepoFileFeatures_CreateTagErrors(t *testing.T) {
	rf, repo, branch := newFeatureRepo(t)
	ctx := context.Background()
	commits := writeFeatureFiles(t, rf, branch.ID, []featureFile{{Path: "a.txt", Content: "a"}})
	if _, err := rf.CreateTag(ctx, CreateTagRequest{RepositoryID: repo.ID, Name: "v1", CommitID: commits[0].ID}); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if _, err := rf.CreateTag(ctx, CreateTagRequest{RepositoryID: repo.ID, Name: "v1", CommitID: commits[0].ID}); !errors.Is(err, ErrTagAlreadyExists) {
		t.Fatalf("duplicate tag err = %v, want ErrTagAlreadyExists", err)
	}
	if _, err := rf.CreateTag(ctx, CreateTagRequest{RepositoryID: repo.ID, Name: "v2", CommitID: "no-such-commit"}); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("unknown commit err = %v, want ErrBranchNotFound", err)
	}
	if _, err := rf.CreateTag(ctx, CreateTagRequest{RepositoryID: "nope", Name: "v3", CommitID: commits[0].ID}); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("unknown repo err = %v, want ErrRepoNotInitialised", err)
	}
}

func TestRepoFileFeatures_GetTag(t *testing.T) {
	rf, repo, branch := newFeatureRepo(t)
	ctx := context.Background()
	commits := writeFeatureFiles(t, rf, branch.ID, []featureFile{{Path: "a.txt", Content: "a"}})
	tag, err := rf.CreateTag(ctx, CreateTagRequest{RepositoryID: repo.ID, Name: "v1", CommitID: commits[0].ID, Message: "m"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	got, err := rf.GetTag(ctx, tag.ID)
	if err != nil {
		t.Fatalf("GetTag: %v", err)
	}
	if got.ID != tag.ID || got.Name != "v1" || got.SHA != commits[0].SHA || got.CommitID != commits[0].ID || got.Message != "m" {
		t.Fatalf("got = %+v", got)
	}
	if _, err := rf.GetTag(ctx, "nope"); !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("missing tag err = %v, want ErrTagNotFound", err)
	}
}

func TestRepoFileFeatures_ListTags(t *testing.T) {
	rf, repo, branch := newFeatureRepo(t)
	ctx := context.Background()
	empty, err := rf.ListTags(ctx, repo.ID)
	if err != nil {
		t.Fatalf("ListTags empty: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("tags on fresh repo = %+v", empty)
	}
	commits := writeFeatureFiles(t, rf, branch.ID, []featureFile{{Path: "a.txt", Content: "a"}, {Path: "b.txt", Content: "b"}})
	for i, name := range []string{"v1", "v2"} {
		if _, err := rf.CreateTag(ctx, CreateTagRequest{RepositoryID: repo.ID, Name: name, CommitID: commits[i].ID}); err != nil {
			t.Fatalf("CreateTag %s: %v", name, err)
		}
	}
	tags, err := rf.ListTags(ctx, repo.ID)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	shaOf := map[string]string{}
	for _, tg := range tags {
		shaOf[tg.Name] = tg.SHA
	}
	if len(tags) != 2 || shaOf["v1"] != commits[0].SHA || shaOf["v2"] != commits[1].SHA {
		t.Fatalf("tags = %+v", tags)
	}
	if _, err := rf.ListTags(ctx, "nope"); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("unknown repo err = %v, want ErrRepoNotInitialised", err)
	}
}

func TestRepoFileFeatures_DeleteTag(t *testing.T) {
	rf, repo, branch := newFeatureRepo(t)
	ctx := context.Background()
	commits := writeFeatureFiles(t, rf, branch.ID, []featureFile{{Path: "a.txt", Content: "a"}})
	tag, err := rf.CreateTag(ctx, CreateTagRequest{RepositoryID: repo.ID, Name: "v1", CommitID: commits[0].ID})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := rf.DeleteTag(ctx, tag.ID); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	if _, err := rf.GetTag(ctx, tag.ID); !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("GetTag after delete err = %v", err)
	}
	tags, err := rf.ListTags(ctx, repo.ID)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("tags after delete = %+v", tags)
	}
	if err := rf.DeleteTag(ctx, tag.ID); !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("second DeleteTag err = %v, want ErrTagNotFound", err)
	}
	again, err := rf.CreateTag(ctx, CreateTagRequest{RepositoryID: repo.ID, Name: "v1", CommitID: commits[0].ID})
	if err != nil {
		t.Fatalf("recreate deleted tag name: %v", err)
	}
	if again.ID == tag.ID {
		t.Fatal("recreated tag reused the deleted tag ID")
	}
}

func TestRepoFileFeatures_CreateMergeRequest(t *testing.T) {
	rf, repo, defaultBranch := newFeatureRepo(t)
	ctx := context.Background()
	feature := repoFeatureBranch(t, rf, repo, "feature", "run-1")
	mr, err := rf.CreateMergeRequest(ctx, CreateMergeRequestRequest{
		RepositoryID:   repo.ID,
		Title:          "Add widgets",
		Description:    "adds the widgets",
		SourceBranchID: feature.ID,
		AuthorName:     "Tony",
		WorkflowRunID:  "run-1",
	})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}
	if mr.ID == "" || mr.Title != "Add widgets" || mr.Description != "adds the widgets" || mr.AuthorName != "Tony" {
		t.Fatalf("mr = %+v", mr)
	}
	if mr.Status != models.MergeRequestStatusOpen || mr.RepositoryID != repo.ID || mr.WorkflowRunID != "run-1" {
		t.Fatalf("mr = %+v", mr)
	}
	if mr.SourceBranchID != feature.ID || mr.SourceBranchName != "feature" {
		t.Fatalf("source = %q/%q", mr.SourceBranchID, mr.SourceBranchName)
	}
	if mr.TargetBranchID != defaultBranch.ID || mr.TargetBranchName != "main" {
		t.Fatalf("target = %q/%q, want default branch", mr.TargetBranchID, mr.TargetBranchName)
	}
}

func TestRepoFileFeatures_CreateMergeRequestExplicitTarget(t *testing.T) {
	rf, repo, _ := newFeatureRepo(t)
	source := repoFeatureBranch(t, rf, repo, "source", "")
	target := repoFeatureBranch(t, rf, repo, "release", "")
	mr, err := rf.CreateMergeRequest(context.Background(), CreateMergeRequestRequest{
		RepositoryID:   repo.ID,
		Title:          "Into release",
		SourceBranchID: source.ID,
		TargetBranchID: target.ID,
	})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}
	if mr.TargetBranchID != target.ID || mr.TargetBranchName != "release" {
		t.Fatalf("target = %q/%q", mr.TargetBranchID, mr.TargetBranchName)
	}
}

func TestRepoFileFeatures_CreateMergeRequestErrors(t *testing.T) {
	rf, repo, _ := newFeatureRepo(t)
	ctx := context.Background()
	feature := repoFeatureBranch(t, rf, repo, "feature", "")
	if _, err := rf.CreateMergeRequest(ctx, CreateMergeRequestRequest{RepositoryID: repo.ID, SourceBranchID: feature.ID}); err == nil {
		t.Fatal("missing title accepted")
	}
	if _, err := rf.CreateMergeRequest(ctx, CreateMergeRequestRequest{RepositoryID: repo.ID, Title: "t"}); err == nil {
		t.Fatal("missing source branch accepted")
	}
	if _, err := rf.CreateMergeRequest(ctx, CreateMergeRequestRequest{RepositoryID: "nope", Title: "t", SourceBranchID: feature.ID}); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("unknown repo err = %v, want ErrRepoNotInitialised", err)
	}
	if _, err := rf.CreateMergeRequest(ctx, CreateMergeRequestRequest{RepositoryID: repo.ID, Title: "t", SourceBranchID: "nope"}); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("unknown source err = %v, want ErrBranchNotFound", err)
	}
	if _, err := rf.CreateMergeRequest(ctx, CreateMergeRequestRequest{RepositoryID: repo.ID, Title: "t", SourceBranchID: feature.ID, TargetBranchID: "nope"}); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("unknown target err = %v, want ErrBranchNotFound", err)
	}
}

func TestRepoFileFeatures_GetMergeRequest(t *testing.T) {
	rf, repo, _ := newFeatureRepo(t)
	ctx := context.Background()
	feature := repoFeatureBranch(t, rf, repo, "feature", "")
	mr, err := rf.CreateMergeRequest(ctx, CreateMergeRequestRequest{RepositoryID: repo.ID, Title: "t", SourceBranchID: feature.ID})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}
	got, err := rf.GetMergeRequest(ctx, mr.ID)
	if err != nil {
		t.Fatalf("GetMergeRequest: %v", err)
	}
	if got.ID != mr.ID || got.Title != "t" || got.Status != models.MergeRequestStatusOpen || got.SourceBranchID != feature.ID {
		t.Fatalf("got = %+v", got)
	}
	if _, err := rf.GetMergeRequest(ctx, "nope"); !errors.Is(err, ErrMergeRequestNotFound) {
		t.Fatalf("missing err = %v, want ErrMergeRequestNotFound", err)
	}
}

func TestRepoFileFeatures_ListMergeRequests(t *testing.T) {
	rf, repo, _ := newFeatureRepo(t)
	ctx := context.Background()
	open := func(title, runID string) models.MergeRequest {
		branch := repoFeatureBranch(t, rf, repo, "br-"+title, runID)
		mr, err := rf.CreateMergeRequest(ctx, CreateMergeRequestRequest{RepositoryID: repo.ID, Title: title, SourceBranchID: branch.ID, WorkflowRunID: runID})
		if err != nil {
			t.Fatalf("CreateMergeRequest %s: %v", title, err)
		}
		return mr
	}
	first := open("first", "run-a")
	second := open("second", "run-a")
	third := open("third", "run-b")
	if _, err := rf.CloseMergeRequest(ctx, second.ID); err != nil {
		t.Fatalf("CloseMergeRequest: %v", err)
	}

	titles := func(mrs []models.MergeRequest) map[string]bool {
		out := map[string]bool{}
		for _, mr := range mrs {
			out[mr.Title] = true
		}
		return out
	}

	all, err := rf.ListMergeRequests(ctx, MergeRequestFilter{})
	if err != nil {
		t.Fatalf("ListMergeRequests: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("all = %+v", all)
	}

	byRepo, err := rf.ListMergeRequests(ctx, MergeRequestFilter{RepositoryID: repo.ID})
	if err != nil {
		t.Fatalf("by repository: %v", err)
	}
	if len(byRepo) != 3 {
		t.Fatalf("by repository = %+v", byRepo)
	}
	otherRepo, err := rf.ListMergeRequests(ctx, MergeRequestFilter{RepositoryID: "other"})
	if err != nil {
		t.Fatalf("by other repository: %v", err)
	}
	if len(otherRepo) != 0 {
		t.Fatalf("other repository = %+v", otherRepo)
	}

	openOnly, err := rf.ListMergeRequests(ctx, MergeRequestFilter{Status: models.MergeRequestStatusOpen})
	if err != nil {
		t.Fatalf("by status open: %v", err)
	}
	got := titles(openOnly)
	if len(openOnly) != 2 || !got[first.Title] || !got[third.Title] {
		t.Fatalf("open = %+v", openOnly)
	}
	closed, err := rf.ListMergeRequests(ctx, MergeRequestFilter{Status: models.MergeRequestStatusClosed})
	if err != nil {
		t.Fatalf("by status closed: %v", err)
	}
	if len(closed) != 1 || closed[0].ID != second.ID {
		t.Fatalf("closed = %+v", closed)
	}

	runA, err := rf.ListMergeRequests(ctx, MergeRequestFilter{WorkflowRunID: "run-a"})
	if err != nil {
		t.Fatalf("by run: %v", err)
	}
	got = titles(runA)
	if len(runA) != 2 || !got["first"] || !got["second"] {
		t.Fatalf("run-a = %+v", runA)
	}

	runAOpen, err := rf.ListMergeRequests(ctx, MergeRequestFilter{WorkflowRunID: "run-a", Status: models.MergeRequestStatusOpen})
	if err != nil {
		t.Fatalf("by run and status: %v", err)
	}
	if len(runAOpen) != 1 || runAOpen[0].ID != first.ID {
		t.Fatalf("run-a open = %+v", runAOpen)
	}

	limited, err := rf.ListMergeRequests(ctx, MergeRequestFilter{Limit: 2})
	if err != nil {
		t.Fatalf("limited: %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("limited = %+v", limited)
	}
}

func TestRepoFileFeatures_CompleteMergeRequest(t *testing.T) {
	rf, repo, defaultBranch := newFeatureRepo(t)
	ctx := context.Background()
	feature := repoFeatureBranch(t, rf, repo, "feature", "run-c")
	commits := writeFeatureFiles(t, rf, feature.ID, []featureFile{{Path: "w.txt", Content: "widget"}})
	mr, err := rf.CreateMergeRequest(ctx, CreateMergeRequestRequest{RepositoryID: repo.ID, Title: "ship", SourceBranchID: feature.ID, WorkflowRunID: "run-c"})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}
	done, err := rf.CompleteMergeRequest(ctx, mr.ID, "reviewer")
	if err != nil {
		t.Fatalf("CompleteMergeRequest: %v", err)
	}
	if done.Status != models.MergeRequestStatusMerged || done.MergedBy != "reviewer" {
		t.Fatalf("done = %+v", done)
	}
	if done.MergedCommitSHA != commits[0].SHA {
		t.Fatalf("MergedCommitSHA = %q, want %q", done.MergedCommitSHA, commits[0].SHA)
	}
	persisted, err := rf.GetMergeRequest(ctx, mr.ID)
	if err != nil {
		t.Fatalf("GetMergeRequest: %v", err)
	}
	if persisted.Status != models.MergeRequestStatusMerged || persisted.MergedCommitSHA != commits[0].SHA || persisted.MergedBy != "reviewer" {
		t.Fatalf("persisted = %+v", persisted)
	}
	blob, err := rf.ReadFile(ctx, defaultBranch.ID, "w.txt")
	if err != nil {
		t.Fatalf("ReadFile on default after merge: %v", err)
	}
	if blob.Content != "widget" {
		t.Fatalf("content = %q", blob.Content)
	}
	if _, err := rf.CompleteMergeRequest(ctx, mr.ID, "reviewer"); !errors.Is(err, ErrMergeRequestNotOpen) {
		t.Fatalf("second complete err = %v, want ErrMergeRequestNotOpen", err)
	}
	if _, err := rf.CloseMergeRequest(ctx, mr.ID); !errors.Is(err, ErrMergeRequestNotOpen) {
		t.Fatalf("close after merge err = %v, want ErrMergeRequestNotOpen", err)
	}
	if _, err := rf.CompleteMergeRequest(ctx, "nope", "x"); !errors.Is(err, ErrMergeRequestNotFound) {
		t.Fatalf("unknown complete err = %v, want ErrMergeRequestNotFound", err)
	}
}

func TestRepoFileFeatures_CloseMergeRequest(t *testing.T) {
	rf, repo, defaultBranch := newFeatureRepo(t)
	ctx := context.Background()
	feature := repoFeatureBranch(t, rf, repo, "feature", "")
	writeFeatureFiles(t, rf, feature.ID, []featureFile{{Path: "w.txt", Content: "widget"}})
	mr, err := rf.CreateMergeRequest(ctx, CreateMergeRequestRequest{RepositoryID: repo.ID, Title: "abandon", SourceBranchID: feature.ID})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}
	closed, err := rf.CloseMergeRequest(ctx, mr.ID)
	if err != nil {
		t.Fatalf("CloseMergeRequest: %v", err)
	}
	if closed.Status != models.MergeRequestStatusClosed || closed.MergedCommitSHA != "" {
		t.Fatalf("closed = %+v", closed)
	}
	persisted, err := rf.GetMergeRequest(ctx, mr.ID)
	if err != nil {
		t.Fatalf("GetMergeRequest: %v", err)
	}
	if persisted.Status != models.MergeRequestStatusClosed {
		t.Fatalf("persisted status = %q", persisted.Status)
	}
	def, err := rf.GetBranch(ctx, defaultBranch.ID)
	if err != nil {
		t.Fatalf("GetBranch: %v", err)
	}
	if def.HeadCommitID != "" {
		t.Fatalf("closing moved the default branch: %+v", def)
	}
	if _, err := rf.CloseMergeRequest(ctx, mr.ID); !errors.Is(err, ErrMergeRequestNotOpen) {
		t.Fatalf("second close err = %v, want ErrMergeRequestNotOpen", err)
	}
	if _, err := rf.CompleteMergeRequest(ctx, mr.ID, "x"); !errors.Is(err, ErrMergeRequestNotOpen) {
		t.Fatalf("complete after close err = %v, want ErrMergeRequestNotOpen", err)
	}
	if _, err := rf.CloseMergeRequest(ctx, "nope"); !errors.Is(err, ErrMergeRequestNotFound) {
		t.Fatalf("unknown close err = %v, want ErrMergeRequestNotFound", err)
	}
}
