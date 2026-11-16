package mwanachamagit

import (
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func (f *routedGitFixture) pointRepositoryAtSource(t *testing.T, repo models.Repository, bareDir string) {
	t.Helper()
	f.inFileOf(t, repo, func(_ *RepoFile, manager *gitManager) {
		err := manager.db.WithContext(f.ctx).Table(manager.tables.Repositories).
			Where("id = ?", repo.ID).Update("bare_clone_path", bareDir).Error
		if err != nil {
			t.Fatalf("set bare clone path of %s: %v", repo.Name, err)
		}
	})
}

func TestRoutedGitManager_APushedBranchIsIndexedInTheFileOfTheNamedRepositoryInEitherOrder(t *testing.T) {
	transportRequireGit(t)
	f := newRoutedGitFixture(t, 4)
	aaa, _ := f.initRepo(t, "aaa")
	zzz, _ := f.initRepo(t, "zzz")
	sources := map[string]transportSource{"zzz": newTransportSource(t), "aaa": newTransportSource(t)}
	repos := map[string]models.Repository{"aaa": aaa, "zzz": zzz}
	for name, repo := range repos {
		f.pointRepositoryAtSource(t, repo, sources[name].BareDir)
	}
	for _, name := range []string{"zzz", "aaa"} {
		repo, source := repos[name], sources[name]
		if _, err := f.manager.GetBranchByName(f.ctx, repo.ID, "feature"); !errors.Is(err, ErrBranchNotFound) {
			t.Fatalf("GetBranchByName feature in %s before indexing = %v, want ErrBranchNotFound", name, err)
		}
		if err := f.manager.IndexPushedBranch(f.ctx, name, "refs/heads/feature", transportZeroSHA, source.FeatureSHA); err != nil {
			t.Fatalf("IndexPushedBranch in %s: %v", name, err)
		}
		feature, err := f.manager.GetBranchByName(f.ctx, repo.ID, "feature")
		if err != nil || feature.SHA != source.FeatureSHA || feature.HeadCommitID == "" || feature.RepositoryID != repo.ID {
			t.Fatalf("feature in %s after indexing = %+v, %v; want sha %s and a head commit", name, feature, err, source.FeatureSHA)
		}
		blob, err := f.manager.ReadFile(f.ctx, feature.ID, "c.txt")
		if err != nil || blob.Content != "gamma\n" {
			t.Fatalf("c.txt on feature in %s = %+v, %v", name, blob, err)
		}
	}
}

func TestRoutedGitManager_IndexingAPushedBranchOfAnUnknownRepositoryNameIsRefused(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	f.initRepo(t, "aaa")
	f.initRepo(t, "zzz")
	if err := f.manager.IndexPushedBranch(f.ctx, "no-such-name", "refs/heads/topic", transportZeroSHA, transportZeroSHA); !errors.Is(err, ErrRepoNotInitialised) {
		t.Fatalf("IndexPushedBranch of an unknown name = %v, want ErrRepoNotInitialised", err)
	}
}
