//go:build g62defect

package mwanachamagit

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aosanya/mwanachama-backend-git/models"
)

const (
	concurrencyCommitsPerAgent = 2
	concurrencyMaxAttempts     = 200
	concurrencyDeadline        = 2 * time.Minute
)

type concurrencyAgent struct {
	index    int
	path     string
	finalTxt string
	shas     []string
}

type concurrencyRun struct {
	rf        *RepoFile
	repoID    string
	defaultID string
	written   atomic.Int64
	merged    atomic.Int64
	refused   atomic.Int64
	stale     atomic.Int64
	recheck   bool
}

func (r *concurrencyRun) writeAndMerge(ctx context.Context, a *concurrencyAgent, attempt int) (merged bool, err error) {
	branch, err := r.rf.CreateBranch(ctx, CreateBranchRequest{
		RepositoryID: r.repoID,
		Name:         fmt.Sprintf("agent-%d-attempt-%d", a.index, attempt),
	})
	if err != nil {
		return false, fmt.Errorf("CreateBranch: %w", err)
	}
	shas := make([]string, 0, concurrencyCommitsPerAgent)
	for rev := 1; rev <= concurrencyCommitsPerAgent; rev++ {
		commit, err := r.rf.WriteFile(ctx, WriteFileRequest{
			BranchID: branch.ID,
			Path:     a.path,
			Content:  fmt.Sprintf("agent %d revision %d", a.index, rev),
			Message:  fmt.Sprintf("agent %d revision %d", a.index, rev),
		})
		if err != nil {
			return false, fmt.Errorf("WriteFile: %w", err)
		}
		r.written.Add(1)
		shas = append(shas, commit.SHA)
	}
	if r.recheck && r.defaultHeadMoved(ctx, branch.HeadCommitID) {
		r.stale.Add(1)
		return false, nil
	}
	_, err = r.rf.MergeBranch(ctx, branch.ID)
	if errors.Is(err, ErrMergeConcurrencyConflict) {
		r.refused.Add(1)
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("MergeBranch: %w", err)
	}
	r.merged.Add(1)
	a.shas = shas
	return true, nil
}

func (r *concurrencyRun) defaultHeadMoved(ctx context.Context, cutFrom string) bool {
	current, err := r.rf.GetBranch(ctx, r.defaultID)
	return err != nil || current.HeadCommitID != cutFrom
}

func (r *concurrencyRun) runAgent(ctx context.Context, a *concurrencyAgent) error {
	for attempt := 1; attempt <= concurrencyMaxAttempts; attempt++ {
		merged, err := r.writeAndMerge(ctx, a, attempt)
		if err != nil {
			return err
		}
		if merged {
			return nil
		}
	}
	return fmt.Errorf("agent %d: still refused after %d attempts", a.index, concurrencyMaxAttempts)
}

func (r *concurrencyRun) runAgents(ctx context.Context, agents []*concurrencyAgent) []error {
	start := make(chan struct{})
	errs := make([]error, len(agents))
	var wg sync.WaitGroup
	for i, a := range agents {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = r.runAgent(ctx, a)
		}()
	}
	close(start)
	wg.Wait()
	return errs
}

func newConcurrencyAgents(n int) []*concurrencyAgent {
	agents := make([]*concurrencyAgent, n)
	for i := range agents {
		agents[i] = &concurrencyAgent{
			index:    i,
			path:     fmt.Sprintf("agents/agent-%03d.txt", i),
			finalTxt: fmt.Sprintf("agent %d revision %d", i, concurrencyCommitsPerAgent),
		}
	}
	return agents
}

func defaultBranchOf(t *testing.T, ctx context.Context, rf *RepoFile, repoID string) models.Branch {
	t.Helper()
	branches, err := rf.ListBranches(ctx, repoID)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	for _, b := range branches {
		if b.IsDefault {
			return b
		}
	}
	t.Fatalf("no default branch among %+v", branches)
	return models.Branch{}
}

func TestRepoFileConcurrency_ManyAgentsMergeIntoOneRepositoryFile(t *testing.T) {
	for _, protocol := range []struct {
		name    string
		recheck bool
	}{{"mergeWithoutRecheck", false}, {"recheckDefaultHeadBeforeMerge", true}} {
		for _, n := range []int{10, 30} {
			t.Run(fmt.Sprintf("%s/agents=%d", protocol.name, n), func(t *testing.T) {
				runManyAgents(t, n, protocol.recheck)
			})
		}
	}
}

func runManyAgents(t *testing.T, n int, recheck bool) {
	ctx, cancel := context.WithTimeout(context.Background(), concurrencyDeadline)
	defer cancel()

	rf := openRepoFileForTest(t, filepath.Join(t.TempDir(), "agents.db"))
	defer closeRepoFileForTest(t, rf)

	repo, err := rf.InitRepo(ctx, CreateRepoRequest{Name: "agents"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	run := &concurrencyRun{rf: rf, repoID: repo.ID, defaultID: defaultBranchOf(t, ctx, rf, repo.ID).ID, recheck: recheck}
	agents := newConcurrencyAgents(n)

	began := time.Now()
	errs := run.runAgents(ctx, agents)
	elapsed := time.Since(began)

	for i, err := range errs {
		if err != nil {
			t.Errorf("agent %d: %v", i, err)
		}
	}
	if ctx.Err() != nil {
		t.Fatalf("deadline exceeded after %s: %v", elapsed, ctx.Err())
	}
	t.Logf("agents=%d commits=%d merges=%d refused=%d stale=%d elapsed=%s commits/sec=%.1f",
		n, run.written.Load(), run.merged.Load(), run.refused.Load(), run.stale.Load(), elapsed.Round(time.Millisecond),
		float64(run.written.Load()+run.merged.Load())/elapsed.Seconds())

	assertNoCommitLost(t, ctx, run, agents)
}

func assertNoCommitLost(t *testing.T, ctx context.Context, run *concurrencyRun, agents []*concurrencyAgent) {
	t.Helper()
	history, err := run.rf.Log(ctx, run.defaultID, LogFilter{})
	if err != nil {
		t.Fatalf("Log default: %v", err)
	}
	inHistory := make(map[string]bool, len(history))
	for _, entry := range history {
		inHistory[entry.SHA] = true
	}
	var lostFiles, lostCommits []string
	for _, a := range agents {
		blob, err := run.rf.ReadFile(ctx, run.defaultID, a.path)
		if err != nil || blob.Content != a.finalTxt {
			lostFiles = append(lostFiles, fmt.Sprintf("agent %d (err=%v content=%q)", a.index, err, blob.Content))
		}
		for _, sha := range a.shas {
			if !inHistory[sha] {
				lostCommits = append(lostCommits, fmt.Sprintf("agent %d %s", a.index, sha[:8]))
			}
		}
	}
	if len(lostFiles) > 0 {
		t.Errorf("%d of %d agent files missing or wrong on default branch: %s", len(lostFiles), len(agents), strings.Join(lostFiles, "; "))
	}
	if len(lostCommits) > 0 {
		t.Errorf("%d commits missing from default branch history (%d entries): %s", len(lostCommits), len(history), strings.Join(lostCommits, "; "))
	}
}

func TestRepoFileConcurrency_SecondMergeOfSiblingBranchKeepsFirstMergedCommit(t *testing.T) {
	ctx := context.Background()
	rf := openRepoFileForTest(t, filepath.Join(t.TempDir(), "siblings.db"))
	defer closeRepoFileForTest(t, rf)

	repo, err := rf.InitRepo(ctx, CreateRepoRequest{Name: "siblings"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	defaultBranch := defaultBranchOf(t, ctx, rf, repo.ID)

	var branches [2]models.Branch
	for i := range branches {
		branches[i], err = rf.CreateBranch(ctx, CreateBranchRequest{RepositoryID: repo.ID, Name: fmt.Sprintf("sibling-%d", i)})
		if err != nil {
			t.Fatalf("CreateBranch %d: %v", i, err)
		}
		if _, err := rf.WriteFile(ctx, WriteFileRequest{BranchID: branches[i].ID, Path: fmt.Sprintf("sibling-%d.txt", i), Content: fmt.Sprintf("sibling %d", i)}); err != nil {
			t.Fatalf("WriteFile %d: %v", i, err)
		}
	}
	for i := range branches {
		if _, err := rf.MergeBranch(ctx, branches[i].ID); err != nil && !errors.Is(err, ErrMergeConcurrencyConflict) {
			t.Fatalf("MergeBranch %d: %v", i, err)
		}
	}
	for i := range branches {
		blob, err := rf.ReadFile(ctx, defaultBranch.ID, fmt.Sprintf("sibling-%d.txt", i))
		if err != nil || blob.Content != fmt.Sprintf("sibling %d", i) {
			t.Errorf("sibling-%d.txt on default after both merges: content=%q err=%v", i, blob.Content, err)
		}
	}
}
