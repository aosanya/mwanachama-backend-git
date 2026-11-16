package mwanachamagit

import (
	"testing"
	"time"
)

func TestRoutedGitManager_RepositorySummaryCarriesTheLastCommitTimeAndItMovesAfterAMerge(t *testing.T) {
	f := newRoutedGitFixture(t, 4)
	repo, main := f.initRepo(t, "widgets")

	before, err := f.manager.ListRepositorySummaries(f.ctx)
	if err != nil || len(before) != 1 {
		t.Fatalf("ListRepositorySummaries = %+v, %v; want one", before, err)
	}

	time.Sleep(1100 * time.Millisecond)
	f.writeAndMerge(t, repo, main, "hello.md", "hello")

	after, err := f.manager.ListRepositorySummaries(f.ctx)
	if err != nil || len(after) != 1 {
		t.Fatalf("ListRepositorySummaries = %+v, %v; want one", after, err)
	}
	if after[0].LastCommitAt == "" {
		t.Fatalf("LastCommitAt is empty after a merge: %+v", after[0])
	}
	if after[0].LastCommitAt == before[0].LastCommitAt {
		t.Fatalf("LastCommitAt did not move after a merge: before %q, after %q", before[0].LastCommitAt, after[0].LastCommitAt)
	}
	entries, err := f.catalog.List(f.ctx)
	if err != nil || len(entries) != 1 {
		t.Fatalf("catalog = %+v, %v; want one entry", entries, err)
	}
	if after[0].LastCommitAt != entries[0].LastCommitAt {
		t.Fatalf("LastCommitAt = %q, the catalog holds %q", after[0].LastCommitAt, entries[0].LastCommitAt)
	}
}
