package mwanachamagit

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

type mcFixture struct {
	m    *gitManager
	repo models.Repository
	main models.Branch
}

func newMcFixture(t *testing.T) mcFixture {
	t.Helper()
	m := newTestManager(t)
	repo, main := mbRepo(t, m, "merge-commit")
	return mcFixture{m: m, repo: repo, main: main}
}

func (f mcFixture) branch(t *testing.T, name string) models.Branch {
	t.Helper()
	b, err := f.m.CreateBranch(context.Background(), CreateBranchRequest{RepositoryID: f.repo.ID, Name: name})
	if err != nil {
		t.Fatalf("CreateBranch %s: %v", name, err)
	}
	return b
}

func (f mcFixture) reload(t *testing.T, branchID string) models.Branch {
	t.Helper()
	b, err := f.m.GetBranch(context.Background(), branchID)
	if err != nil {
		t.Fatalf("GetBranch %s: %v", branchID, err)
	}
	return b
}

func (f mcFixture) commit(t *testing.T, branchID, path, content string) string {
	t.Helper()
	return mbCommit(t, f.m, branchID, path, content)
}

func (f mcFixture) content(t *testing.T, branchID, path string) string {
	t.Helper()
	blob, err := f.m.ReadFile(context.Background(), branchID, path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	return blob.Content
}

func (f mcFixture) sha(t *testing.T, commitID string) string {
	t.Helper()
	var row models.Commit
	if err := f.m.db.Table(f.m.tables.Commits).Where("id = ?", commitID).First(&row).Error; err != nil {
		t.Fatalf("commit %s: %v", commitID, err)
	}
	return row.SHA
}

func (f mcFixture) logSHAs(t *testing.T, branchID string) []string {
	t.Helper()
	entries, err := f.m.Log(context.Background(), branchID, LogFilter{})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.SHA
	}
	return ids
}

func (f mcFixture) diverged(t *testing.T) (source models.Branch, defaultCommitID, sourceCommitID string) {
	t.Helper()
	f.commit(t, f.main.ID, "base.txt", "base")
	source = f.branch(t, "source")
	defaultCommitID = f.commit(t, f.main.ID, "d.txt", "from default")
	sourceCommitID = f.commit(t, source.ID, "s.txt", "from source")
	return f.reload(t, source.ID), defaultCommitID, sourceCommitID
}

func TestMergeCommit_DivergedMergeHasBothParentsAndBothHistories(t *testing.T) {
	f := newMcFixture(t)
	source, defaultCommitID, sourceCommitID := f.diverged(t)

	merged, err := f.m.MergeBranch(context.Background(), source.ID)
	if err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	if merged.HeadCommitID == defaultCommitID || merged.HeadCommitID == sourceCommitID {
		t.Fatalf("default head %s is not a new merge commit", merged.HeadCommitID)
	}
	parents, err := CommitParentIDs(f.m.db, f.m.tables, merged.HeadCommitID)
	if err != nil {
		t.Fatalf("CommitParentIDs: %v", err)
	}
	if want := []string{defaultCommitID, sourceCommitID}; !reflect.DeepEqual(parents, want) {
		t.Fatalf("merge commit parents = %v, want %v", parents, want)
	}
	log := f.logSHAs(t, f.main.ID)
	for _, id := range []string{merged.HeadCommitID, defaultCommitID, sourceCommitID} {
		if !mergeContains(log, f.sha(t, id)) {
			t.Fatalf("default log %v lacks commit %s", log, id)
		}
	}
	for path, want := range map[string]string{"base.txt": "base", "d.txt": "from default", "s.txt": "from source"} {
		if got := f.content(t, f.main.ID, path); got != want {
			t.Fatalf("%s = %q, want %q", path, got, want)
		}
	}
	if got := f.reload(t, source.ID).HeadCommitID; got != sourceCommitID {
		t.Fatalf("source head moved to %s, want %s", got, sourceCommitID)
	}
}

func TestMergeCommit_FastForwardCreatesNoCommit(t *testing.T) {
	f := newMcFixture(t)
	f.commit(t, f.main.ID, "base.txt", "base")
	source := f.branch(t, "source")
	sourceCommitID := f.commit(t, source.ID, "s.txt", "from source")

	merged, err := f.m.MergeBranch(context.Background(), source.ID)
	if err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	if merged.HeadCommitID != sourceCommitID {
		t.Fatalf("default head = %s, want source head %s", merged.HeadCommitID, sourceCommitID)
	}
	parents, err := CommitParentIDs(f.m.db, f.m.tables, merged.HeadCommitID)
	if err != nil {
		t.Fatalf("CommitParentIDs: %v", err)
	}
	if len(parents) != 1 {
		t.Fatalf("fast-forward head has %d parents, want 1", len(parents))
	}
}

func TestMergeCommit_AlreadyContainedChangesNothing(t *testing.T) {
	f := newMcFixture(t)
	f.commit(t, f.main.ID, "base.txt", "base")
	source := f.branch(t, "source")
	f.commit(t, source.ID, "s.txt", "from source")
	if _, err := f.m.MergeBranch(context.Background(), source.ID); err != nil {
		t.Fatalf("first MergeBranch: %v", err)
	}
	afterFirst := f.reload(t, f.main.ID)
	logBefore := f.logSHAs(t, f.main.ID)
	f.commit(t, f.main.ID, "later.txt", "later")
	afterLater := f.reload(t, f.main.ID)

	merged, err := f.m.MergeBranch(context.Background(), source.ID)
	if err != nil {
		t.Fatalf("second MergeBranch: %v", err)
	}
	if merged.HeadCommitID != afterLater.HeadCommitID || merged.HeadCommitID == afterFirst.HeadCommitID {
		t.Fatalf("default head = %s, want unchanged %s", merged.HeadCommitID, afterLater.HeadCommitID)
	}
	if got := len(f.logSHAs(t, f.main.ID)); got != len(logBefore)+1 {
		t.Fatalf("log length = %d, want %d", got, len(logBefore)+1)
	}
}

func TestMergeCommit_StaleExpectedDefaultReturnsConcurrencyConflict(t *testing.T) {
	f := newMcFixture(t)
	source, _, _ := f.diverged(t)
	stale := f.reload(t, f.main.ID)
	moved := f.commit(t, f.main.ID, "other.txt", "written meanwhile")

	_, err := f.m.mergeIntoDefault(context.Background(), source, stale)
	if !errors.Is(err, ErrMergeConcurrencyConflict) {
		t.Fatalf("mergeIntoDefault error = %v, want ErrMergeConcurrencyConflict", err)
	}
	if got := f.reload(t, f.main.ID).HeadCommitID; got != moved {
		t.Fatalf("default head = %s, want writer's commit %s", got, moved)
	}
}

func TestMergeCommit_OverlappingEditsReturnMergeConflictAndLeaveDefaultUntouched(t *testing.T) {
	f := newMcFixture(t)
	f.commit(t, f.main.ID, "shared.txt", "base")
	source := f.branch(t, "source")
	f.commit(t, f.main.ID, "shared.txt", "default edit")
	f.commit(t, source.ID, "shared.txt", "source edit")
	headBefore := f.reload(t, f.main.ID).HeadCommitID

	_, err := f.m.MergeBranch(context.Background(), source.ID)

	var conflict *ErrMergeConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("MergeBranch error = %v, want *ErrMergeConflict", err)
	}
	if !reflect.DeepEqual(conflict.ConflictingFiles, []string{"shared.txt"}) {
		t.Fatalf("ConflictingFiles = %v, want [shared.txt]", conflict.ConflictingFiles)
	}
	if got := f.reload(t, f.main.ID).HeadCommitID; got != headBefore {
		t.Fatalf("default head = %s, want unchanged %s", got, headBefore)
	}
	if got := f.content(t, f.main.ID, "shared.txt"); got != "default edit" {
		t.Fatalf("shared.txt = %q, want default edit", got)
	}
}
