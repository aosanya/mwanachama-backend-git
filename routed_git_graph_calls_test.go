package mwanachamagit

import (
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

type graphScene struct {
	branch  models.Branch
	blobA   models.Blob
	blobB   models.Blob
	keyword models.Keyword
}

func (f *routedGitFixture) graphSceneIn(t *testing.T, repo models.Repository, main models.Branch) graphScene {
	t.Helper()
	branch, err := f.manager.CreateBranch(f.ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: "graph-" + repo.Name, FromBranchID: main.ID})
	if err != nil {
		t.Fatalf("CreateBranch in %s: %v", repo.Name, err)
	}
	scene := graphScene{branch: branch}
	for path, target := range map[string]*models.Blob{"a.md": &scene.blobA, "b.md": &scene.blobB} {
		if _, err := f.manager.WriteFile(f.ctx, WriteFileRequest{BranchID: branch.ID, Path: path, Content: "content of " + path}); err != nil {
			t.Fatalf("WriteFile %s in %s: %v", path, repo.Name, err)
		}
		blob, err := f.manager.ReadFile(f.ctx, branch.ID, path)
		if err != nil {
			t.Fatalf("ReadFile %s in %s: %v", path, repo.Name, err)
		}
		*target = blob
	}
	f.inFileOf(t, repo, func(file *RepoFile, _ *gitManager) {
		keyword, err := file.CreateKeyword(f.ctx, CreateKeywordRequest{Name: "topic-" + repo.Name})
		if err != nil {
			t.Fatalf("CreateKeyword in %s: %v", repo.Name, err)
		}
		scene.keyword = keyword
	})
	return scene
}

func (s graphScene) tagBlobA() CreateEdgeRequest {
	return CreateEdgeRequest{BranchID: s.branch.ID, FromEntityID: s.blobA.ID, ToEntityID: s.keyword.ID, RelationshipName: "tagged_with"}
}

func (s graphScene) referenceBlobAFromB() CreateEdgeRequest {
	return CreateEdgeRequest{
		BranchID: s.branch.ID, FromEntityID: s.blobB.ID, ToEntityID: s.blobA.ID,
		RelationshipName: "references", Properties: map[string]any{"descriptor": "depends_on"},
	}
}

func hasNode(result GraphResult, id string) bool {
	for _, node := range result.Nodes {
		if node.ID == id {
			return true
		}
	}
	return false
}

func hasEdge(result GraphResult, name, fromID, toID string) bool {
	for _, edge := range result.Edges {
		if edge.Name == name && edge.FromID == fromID && edge.ToID == toID {
			return true
		}
	}
	return false
}

func TestRoutedGitManager_EdgesAndGraphCallsReachTheFileOfTheirBranchInEitherOrder(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	aaa, aMain := f.initRepo(t, "aaa")
	zzz, zMain := f.initRepo(t, "zzz")
	scenes := map[string]graphScene{"zzz": f.graphSceneIn(t, zzz, zMain), "aaa": f.graphSceneIn(t, aaa, aMain)}
	for _, name := range []string{"zzz", "aaa", "zzz", "aaa"} {
		scene := scenes[name]
		if err := f.manager.CreateEdge(f.ctx, scene.tagBlobA()); err != nil {
			t.Fatalf("CreateEdge tagged_with in %s: %v", name, err)
		}
		if err := f.manager.CreateEdge(f.ctx, scene.referenceBlobAFromB()); err != nil {
			t.Fatalf("CreateEdge references in %s: %v", name, err)
		}

		around, err := f.manager.GetNeighborhood(f.ctx, scene.branch.ID, scene.blobA.ID, 1)
		if err != nil || !hasNode(around, scene.keyword.ID) || !hasNode(around, scene.blobB.ID) ||
			!hasEdge(around, "tagged_with", scene.blobA.ID, scene.keyword.ID) {
			t.Fatalf("GetNeighborhood in %s = %+v, %v; want the keyword and blob b around blob a", name, around, err)
		}
		found, err := f.manager.SearchByKeywords(f.ctx, SearchByKeywordsRequest{BranchID: scene.branch.ID, Keywords: []string{scene.keyword.ID}})
		if err != nil || !hasNode(found, scene.blobA.ID) {
			t.Fatalf("SearchByKeywords in %s = %+v, %v; want blob a", name, found, err)
		}
		queried, err := f.manager.QueryGraph(f.ctx, QueryGraphRequest{BranchID: scene.branch.ID, KeywordIDs: []string{scene.keyword.ID}})
		if err != nil || !hasNode(queried, scene.blobA.ID) {
			t.Fatalf("QueryGraph in %s = %+v, %v; want blob a", name, queried, err)
		}

		if err := f.manager.DeleteEdge(f.ctx, DeleteEdgeRequest{
			BranchID: scene.branch.ID, FromEntityID: scene.blobA.ID, ToEntityID: scene.keyword.ID, RelationshipName: "tagged_with",
		}); err != nil {
			t.Fatalf("DeleteEdge tagged_with in %s: %v", name, err)
		}
		afterDelete, err := f.manager.SearchByKeywords(f.ctx, SearchByKeywordsRequest{BranchID: scene.branch.ID, Keywords: []string{scene.keyword.ID}})
		if err != nil || hasNode(afterDelete, scene.blobA.ID) {
			t.Fatalf("SearchByKeywords in %s after DeleteEdge = %+v, %v; want no blob a", name, afterDelete, err)
		}
	}
}

func TestRoutedGitManager_AnEdgeOrGraphCallOnAnUnknownBranchIsBranchNotFound(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	f.initRepo(t, "aaa")
	f.initRepo(t, "zzz")
	if err := f.manager.CreateEdge(f.ctx, CreateEdgeRequest{BranchID: "no-such-branch", FromEntityID: "x", ToEntityID: "y", RelationshipName: "references"}); !errors.Is(err, ErrBranchNotFound) {
		t.Errorf("CreateEdge = %v, want ErrBranchNotFound", err)
	}
	if err := f.manager.DeleteEdge(f.ctx, DeleteEdgeRequest{BranchID: "no-such-branch", FromEntityID: "x", ToEntityID: "y", RelationshipName: "references"}); !errors.Is(err, ErrBranchNotFound) {
		t.Errorf("DeleteEdge = %v, want ErrBranchNotFound", err)
	}
	if _, err := f.manager.GetNeighborhood(f.ctx, "no-such-branch", "x", 1); !errors.Is(err, ErrBranchNotFound) {
		t.Errorf("GetNeighborhood = %v, want ErrBranchNotFound", err)
	}
	if _, err := f.manager.SearchByKeywords(f.ctx, SearchByKeywordsRequest{BranchID: "no-such-branch", Keywords: []string{"k"}}); !errors.Is(err, ErrBranchNotFound) {
		t.Errorf("SearchByKeywords = %v, want ErrBranchNotFound", err)
	}
	if _, err := f.manager.QueryGraph(f.ctx, QueryGraphRequest{BranchID: "no-such-branch"}); !errors.Is(err, ErrBranchNotFound) {
		t.Errorf("QueryGraph = %v, want ErrBranchNotFound", err)
	}
}
