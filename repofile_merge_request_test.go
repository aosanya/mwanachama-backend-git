package mwanachamagit

import (
	"context"
	"reflect"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

type mrFixture struct {
	rf   *RepoFile
	pub  *fakePublisher
	repo models.Repository
	main models.Branch
}

func newMrFixture(t *testing.T) mrFixture {
	t.Helper()
	pub := &fakePublisher{}
	rf, repo, main := conflictOpen(t, pub)
	writeFeatureFiles(t, rf, main.ID, []featureFile{{Path: "base.txt", Content: "base"}})
	return mrFixture{rf: rf, pub: pub, repo: repo, main: main}
}

func (f mrFixture) branch(t *testing.T, name string) models.Branch {
	t.Helper()
	return repoFeatureBranch(t, f.rf, f.repo, name, "")
}

func (f mrFixture) write(t *testing.T, branchID, path, content string) models.Commit {
	t.Helper()
	return writeFeatureFiles(t, f.rf, branchID, []featureFile{{Path: path, Content: content}})[0]
}

func (f mrFixture) open(t *testing.T, source models.Branch) models.MergeRequest {
	t.Helper()
	mr, err := f.rf.CreateMergeRequest(context.Background(), CreateMergeRequestRequest{
		RepositoryID:   f.repo.ID,
		SourceBranchID: source.ID,
		Title:          "merge " + source.Name,
	})
	if err != nil {
		t.Fatalf("CreateMergeRequest %s: %v", source.Name, err)
	}
	return mr
}

func (f mrFixture) complete(t *testing.T, mr models.MergeRequest) models.MergeRequest {
	t.Helper()
	done, err := f.rf.CompleteMergeRequest(context.Background(), mr.ID, "Grace")
	if err != nil {
		t.Fatalf("CompleteMergeRequest %s: %v", mr.Title, err)
	}
	return done
}

func (f mrFixture) parents(t *testing.T, commitID string) []string {
	t.Helper()
	m := transportManager(t, f.rf)
	ids, err := CommitParentIDs(m.db, m.tables, commitID)
	if err != nil {
		t.Fatalf("CommitParentIDs: %v", err)
	}
	return ids
}

func (f mrFixture) commitCount(t *testing.T) int64 {
	t.Helper()
	m := transportManager(t, f.rf)
	var n int64
	if err := m.db.Table(m.tables.Commits).Count(&n).Error; err != nil {
		t.Fatalf("count commits: %v", err)
	}
	return n
}

func (f mrFixture) completedPayload(t *testing.T) MergeRequestCompletedPayload {
	t.Helper()
	var found []MergeRequestCompletedPayload
	for _, e := range f.pub.published() {
		if e.topic != TopicMergeCompleted {
			continue
		}
		payload, ok := e.payload.(MergeRequestCompletedPayload)
		if !ok {
			t.Fatalf("%s payload is %T", TopicMergeCompleted, e.payload)
		}
		found = append(found, payload)
	}
	if len(found) != 1 {
		t.Fatalf("%d %s events, want 1", len(found), TopicMergeCompleted)
	}
	return found[0]
}

func TestMergeRequest_DivergedCompletesWithATwoParentMergeCommit(t *testing.T) {
	f := newMrFixture(t)
	source := f.branch(t, "source")
	defaultCommit := f.write(t, f.main.ID, "d.txt", "from default")
	sourceCommit := f.write(t, source.ID, "s.txt", "from source")
	mr := f.open(t, source)

	done := f.complete(t, mr)

	head := mergeReload(t, f.rf, f.main.ID)
	if done.Status != models.MergeRequestStatusMerged {
		t.Fatalf("status = %q, want merged", done.Status)
	}
	if head.HeadCommitID == defaultCommit.ID || head.HeadCommitID == sourceCommit.ID {
		t.Fatalf("default head %s is not a new merge commit", head.HeadCommitID)
	}
	if done.MergedCommitSHA == "" || done.MergedCommitSHA != head.SHA {
		t.Fatalf("merged_commit_sha = %q, default head sha = %q", done.MergedCommitSHA, head.SHA)
	}
	if done.MergedCommitSHA == sourceCommit.SHA {
		t.Fatalf("merged_commit_sha is the source head %s", sourceCommit.SHA)
	}
	if want := []string{defaultCommit.ID, sourceCommit.ID}; !reflect.DeepEqual(f.parents(t, head.HeadCommitID), want) {
		t.Fatalf("merge commit parents = %v, want %v", f.parents(t, head.HeadCommitID), want)
	}
	mergeAssertFiles(t, f.rf, f.main.ID, map[string]string{"base.txt": "base", "d.txt": "from default", "s.txt": "from source"})
}

func TestMergeRequest_DivergedCompletedEventCarriesTheMergeCommitSHA(t *testing.T) {
	f := newMrFixture(t)
	source := f.branch(t, "source")
	f.write(t, f.main.ID, "d.txt", "from default")
	f.write(t, source.ID, "s.txt", "from source")
	mr := f.open(t, source)

	f.complete(t, mr)

	head := mergeReload(t, f.rf, f.main.ID)
	payload := f.completedPayload(t)
	if payload.MergedCommitSHA == "" || payload.MergedCommitSHA != head.SHA {
		t.Fatalf("event MergedCommitSHA = %q, default head sha = %q", payload.MergedCommitSHA, head.SHA)
	}
	if payload.MergeRequestID != mr.ID {
		t.Fatalf("event MergeRequestID = %q, want %q", payload.MergeRequestID, mr.ID)
	}
}

func TestMergeRequest_DivergedKeepsTheSourceBranchHead(t *testing.T) {
	f := newMrFixture(t)
	source := f.branch(t, "source")
	f.write(t, f.main.ID, "d.txt", "from default")
	sourceCommit := f.write(t, source.ID, "s.txt", "from source")
	mr := f.open(t, source)

	f.complete(t, mr)

	if got := mergeReload(t, f.rf, source.ID).HeadCommitID; got != sourceCommit.ID {
		t.Fatalf("source head moved to %s, want %s", got, sourceCommit.ID)
	}
}

func TestMergeRequest_FastForwardRecordsTheSourceHeadAndAddsNoCommit(t *testing.T) {
	f := newMrFixture(t)
	source := f.branch(t, "source")
	sourceCommit := f.write(t, source.ID, "s.txt", "from source")
	mr := f.open(t, source)
	before := f.commitCount(t)

	done := f.complete(t, mr)

	head := mergeReload(t, f.rf, f.main.ID)
	if head.HeadCommitID != sourceCommit.ID {
		t.Fatalf("default head = %s, want source head %s", head.HeadCommitID, sourceCommit.ID)
	}
	if sourceCommit.SHA == "" || done.MergedCommitSHA != sourceCommit.SHA {
		t.Fatalf("merged_commit_sha = %q, want source head sha %q", done.MergedCommitSHA, sourceCommit.SHA)
	}
	if after := f.commitCount(t); after != before {
		t.Fatalf("commit count = %d, want %d", after, before)
	}
	if got := f.completedPayload(t).MergedCommitSHA; got != sourceCommit.SHA {
		t.Fatalf("event MergedCommitSHA = %q, want %q", got, sourceCommit.SHA)
	}
}

func TestMergeRequest_SiblingBranchesCompletedInTurnKeepEveryFile(t *testing.T) {
	f := newMrFixture(t)
	first := f.branch(t, "first")
	second := f.branch(t, "second")
	firstCommit := f.write(t, first.ID, "first.txt", "from first")
	secondCommit := f.write(t, second.ID, "second.txt", "from second")
	firstMr := f.open(t, first)
	secondMr := f.open(t, second)

	firstDone := f.complete(t, firstMr)
	afterFirst := mergeReload(t, f.rf, f.main.ID)
	secondDone := f.complete(t, secondMr)
	afterSecond := mergeReload(t, f.rf, f.main.ID)

	if firstCommit.SHA == "" || firstDone.MergedCommitSHA != firstCommit.SHA {
		t.Fatalf("first merged_commit_sha = %q, want fast-forward to %q", firstDone.MergedCommitSHA, firstCommit.SHA)
	}
	if secondDone.Status != models.MergeRequestStatusMerged {
		t.Fatalf("second status = %q, want merged", secondDone.Status)
	}
	if want := []string{afterFirst.HeadCommitID, secondCommit.ID}; !reflect.DeepEqual(f.parents(t, afterSecond.HeadCommitID), want) {
		t.Fatalf("second merge parents = %v, want %v", f.parents(t, afterSecond.HeadCommitID), want)
	}
	if secondDone.MergedCommitSHA != afterSecond.SHA {
		t.Fatalf("second merged_commit_sha = %q, default head sha = %q", secondDone.MergedCommitSHA, afterSecond.SHA)
	}
	mergeAssertFiles(t, f.rf, f.main.ID, map[string]string{"base.txt": "base", "first.txt": "from first", "second.txt": "from second"})
	log := mergeLogSHAs(t, f.rf, f.main.ID)
	for _, sha := range []string{firstCommit.SHA, secondCommit.SHA} {
		if !mergeContains(log, sha) {
			t.Fatalf("default log %v lacks %s", log, sha)
		}
	}
}
