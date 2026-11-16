package mwanachamagit

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type pushOutcome struct {
	branch   string
	output   string
	err      error
	duration time.Duration
}

type prepared struct {
	branch string
	dir    string
	tip    string
	file   string
	body   string
}

func prepareBranches(t *testing.T, s fileGitServer, count int) []prepared {
	t.Helper()
	out := make([]prepared, 0, count)
	for i := 0; i < count; i++ {
		branch := fmt.Sprintf("topic-%02d", i)
		dir := s.clone(t)
		transportGit(t, dir, "checkout", "-b", branch)
		file := fmt.Sprintf("work/%s.txt", branch)
		body := fmt.Sprintf("content of %s\n", branch)
		tip := transportCommit(t, dir, file, body, "add "+branch)
		out = append(out, prepared{branch: branch, dir: dir, tip: tip, file: file, body: body})
	}
	return out
}

func pushTogether(items []prepared) []pushOutcome {
	outcomes := make([]pushOutcome, len(items))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, item := range items {
		wg.Add(1)
		go func(i int, item prepared) {
			defer wg.Done()
			<-start
			began := time.Now()
			output, err := transportGitMayFail(item.dir, "push", "origin", item.branch)
			outcomes[i] = pushOutcome{branch: item.branch, output: output, err: err, duration: time.Since(began)}
		}(i, item)
	}
	close(start)
	wg.Wait()
	return outcomes
}

func (s fileGitServer) requirePushedBranch(t *testing.T, item prepared) {
	t.Helper()
	ctx := context.Background()
	branch, err := s.rf.GetBranchByName(ctx, s.repo, item.branch)
	if err != nil {
		t.Errorf("%s: not found through the tools: %v", item.branch, err)
		return
	}
	if branch.SHA != item.tip {
		t.Errorf("%s: tip through the tools = %s, want %s", item.branch, branch.SHA, item.tip)
	}
	blob, err := s.rf.ReadFile(ctx, branch.ID, item.file)
	if err != nil {
		t.Errorf("%s: ReadFile %s: %v", item.branch, item.file, err)
	} else if blob.Content != item.body {
		t.Errorf("%s: %s through the tools = %q, want %q", item.branch, item.file, blob.Content, item.body)
	}
	log, err := s.rf.Log(ctx, branch.ID, LogFilter{})
	if err != nil {
		t.Errorf("%s: Log: %v", item.branch, err)
	} else if len(log) == 0 || log[0].SHA != item.tip {
		t.Errorf("%s: log through the tools does not start at the pushed commit", item.branch)
	}

	fresh := s.clone(t, "--branch", item.branch)
	if got := transportGit(t, fresh, "rev-parse", "HEAD"); got != item.tip {
		t.Errorf("%s: fresh clone HEAD = %s, want %s", item.branch, got, item.tip)
	}
	if got := readCloned(t, fresh, item.file); got != item.body {
		t.Errorf("%s: fresh clone %s = %q, want %q", item.branch, item.file, got, item.body)
	}
	if got := readCloned(t, fresh, "README.md"); got != "# widgets\n" {
		t.Errorf("%s: fresh clone README.md = %q", item.branch, got)
	}
	transportGit(t, fresh, "fsck", "--strict")
}

func TestRepoFileGitHTTPConcurrency_TenPushesOfTenBranchesAtTheSameTime(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()
	items := prepareBranches(t, s, 10)
	mainBefore, err := s.rf.GetBranch(ctx, s.main)
	if err != nil {
		t.Fatalf("GetBranch main: %v", err)
	}

	began := time.Now()
	outcomes := pushTogether(items)
	elapsed := time.Since(began)

	refused, slowest := 0, time.Duration(0)
	for _, o := range outcomes {
		if o.err != nil {
			refused++
			t.Errorf("push of %s refused: %v\n%s", o.branch, o.err, o.output)
		}
		if o.duration > slowest {
			slowest = o.duration
		}
	}
	t.Logf("ten simultaneous pushes of ten branches: %s in total, slowest single push %s, refused %d", elapsed, slowest, refused)

	for _, item := range items {
		s.requirePushedBranch(t, item)
	}

	branches, err := s.rf.ListBranches(ctx, s.repo)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(branches) != 11 {
		names := make([]string, 0, len(branches))
		for _, b := range branches {
			names = append(names, b.Name)
		}
		t.Errorf("branch rows = %d (%s), want 11", len(branches), strings.Join(names, ", "))
	}
	mainAfter, err := s.rf.GetBranch(ctx, s.main)
	if err != nil {
		t.Fatalf("GetBranch main: %v", err)
	}
	if mainAfter.SHA != mainBefore.SHA {
		t.Errorf("default branch moved from %s to %s", mainBefore.SHA, mainAfter.SHA)
	}

	fresh := s.clone(t)
	for _, item := range items {
		transportGit(t, fresh, "fetch", "origin", item.branch)
		if got := transportGit(t, fresh, "rev-parse", "FETCH_HEAD"); got != item.tip {
			t.Errorf("%s: fetched tip = %s, want %s", item.branch, got, item.tip)
		}
	}
}

func TestRepoFileGitHTTPConcurrency_APushDuringToolWritesAndAMergeOnAnotherBranch(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()

	toolBranch, err := s.rf.CreateBranch(ctx, CreateBranchRequest{RepositoryID: s.repo, Name: "by-tools"})
	if err != nil {
		t.Fatalf("CreateBranch by-tools: %v", err)
	}

	work := s.clone(t)
	transportGit(t, work, "checkout", "-b", "by-git")
	const pushes = 6
	tips := make([]string, 0, pushes)
	for i := 0; i < pushes; i++ {
		tips = append(tips, transportCommit(t, work, fmt.Sprintf("git/file-%d.txt", i), fmt.Sprintf("git %d\n", i), fmt.Sprintf("git commit %d", i)))
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	pushErrs := make([]string, 0)
	var toolErr error
	merged := false

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		for i, tip := range tips {
			out, err := transportGitMayFail(work, "push", "origin", tip+":refs/heads/by-git")
			if err != nil {
				pushErrs = append(pushErrs, fmt.Sprintf("push %d: %v\n%s", i, err, out))
			}
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < pushes; i++ {
			_, err := s.rf.WriteFile(ctx, WriteFileRequest{BranchID: toolBranch.ID, Path: fmt.Sprintf("tool/file-%d.txt", i), Content: fmt.Sprintf("tool %d\n", i)})
			if err != nil {
				toolErr = fmt.Errorf("WriteFile %d: %w", i, err)
				return
			}
		}
		if _, err := s.rf.MergeBranch(ctx, toolBranch.ID); err != nil {
			toolErr = fmt.Errorf("MergeBranch: %w", err)
			return
		}
		merged = true
	}()
	began := time.Now()
	close(start)
	wg.Wait()
	t.Logf("%d pushes on one branch while another branch took %d tool writes and a merge: %s", pushes, pushes, time.Since(began))

	for _, e := range pushErrs {
		t.Errorf("%s", e)
	}
	if toolErr != nil {
		t.Fatalf("tool side failed: %v", toolErr)
	}
	if !merged {
		t.Fatal("the merge did not complete")
	}

	byGit, err := s.rf.GetBranchByName(ctx, s.repo, "by-git")
	if err != nil {
		t.Fatalf("by-git not found through the tools: %v", err)
	}
	if byGit.SHA != tips[pushes-1] {
		t.Errorf("by-git tip = %s, want %s", byGit.SHA, tips[pushes-1])
	}
	log, err := s.rf.Log(ctx, byGit.ID, LogFilter{})
	if err != nil {
		t.Fatalf("Log by-git: %v", err)
	}
	inLog := map[string]bool{}
	for _, e := range log {
		inLog[e.SHA] = true
	}
	for i, tip := range tips {
		if !inLog[tip] {
			t.Errorf("pushed commit %d (%s) is missing from the log read through the tools", i, tip)
		}
		blob, err := s.rf.ReadFile(ctx, byGit.ID, fmt.Sprintf("git/file-%d.txt", i))
		if err != nil || blob.Content != fmt.Sprintf("git %d\n", i) {
			t.Errorf("git/file-%d.txt through the tools = %q, %v", i, blob.Content, err)
		}
	}

	mainNow, err := s.rf.GetBranch(ctx, s.main)
	if err != nil {
		t.Fatalf("GetBranch main: %v", err)
	}
	for i := 0; i < pushes; i++ {
		blob, err := s.rf.ReadFile(ctx, mainNow.ID, fmt.Sprintf("tool/file-%d.txt", i))
		if err != nil || blob.Content != fmt.Sprintf("tool %d\n", i) {
			t.Errorf("merged tool/file-%d.txt on main = %q, %v", i, blob.Content, err)
		}
	}

	pushedClone := s.clone(t, "--branch", "by-git")
	if got := transportGit(t, pushedClone, "rev-parse", "HEAD"); got != tips[pushes-1] {
		t.Errorf("clone of by-git HEAD = %s, want %s", got, tips[pushes-1])
	}
	transportGit(t, pushedClone, "fsck", "--strict")
	mainClone := s.clone(t)
	if got := transportGit(t, mainClone, "rev-parse", "HEAD"); got != mainNow.SHA {
		t.Errorf("clone of main HEAD = %s, want %s", got, mainNow.SHA)
	}
	for i := 0; i < pushes; i++ {
		want := fmt.Sprintf("tool %d\n", i)
		if got := readCloned(t, mainClone, fmt.Sprintf("tool/file-%d.txt", i)); got != want {
			t.Errorf("clone of main tool/file-%d.txt = %q, want %q", i, got, want)
		}
	}
	byToolsClone := s.clone(t, "--branch", "by-tools")
	for i := 0; i < pushes; i++ {
		want := fmt.Sprintf("tool %d\n", i)
		if got := readCloned(t, byToolsClone, fmt.Sprintf("tool/file-%d.txt", i)); got != want {
			t.Errorf("clone of by-tools tool/file-%d.txt = %q, want %q", i, got, want)
		}
	}
}

type failureRecorder struct {
	http.ResponseWriter
	code int
	body bytes.Buffer
}

func (f *failureRecorder) WriteHeader(code int) {
	f.code = code
	f.ResponseWriter.WriteHeader(code)
}

func (f *failureRecorder) Write(p []byte) (int, error) {
	if f.code >= http.StatusBadRequest && f.body.Len() < 400 {
		f.body.Write(p)
	}
	return f.ResponseWriter.Write(p)
}

type serverFailures struct {
	mu    sync.Mutex
	lines []string
}

func (s *serverFailures) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &failureRecorder{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rec, r)
		if rec.code >= http.StatusBadRequest {
			s.mu.Lock()
			s.lines = append(s.lines, fmt.Sprintf("%s %s -> %d %s", r.Method, r.URL.Path, rec.code, strings.TrimSpace(rec.body.String())))
			s.mu.Unlock()
		}
	})
}

type observedClone struct {
	output string
	err    error
	head   string
	state  string
	fsck   string
}

func observeClone(s fileGitServer, branch string) observedClone {
	parent, err := os.MkdirTemp("", "concurrent-clone-")
	if err != nil {
		return observedClone{err: err}
	}
	defer os.RemoveAll(parent)
	out, err := transportGitMayFail(parent, "clone", "--branch", branch, s.url, "copy")
	if err != nil {
		return observedClone{output: out, err: err}
	}
	dir := filepath.Join(parent, "copy")
	head, _ := transportGitMayFail(dir, "rev-parse", "HEAD")
	state, _ := os.ReadFile(filepath.Join(dir, "state.txt"))
	fsck, fsckErr := transportGitMayFail(dir, "fsck", "--strict")
	if fsckErr != nil {
		return observedClone{head: head, state: string(state), fsck: fsck, err: fsckErr}
	}
	return observedClone{head: head, state: string(state)}
}

func TestRepoFileGitHTTPConcurrency_ACloneDuringPushesSeesTheOldOrTheNewStateNeverAHalfIndexedOne(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()
	failures := &serverFailures{}
	recording := httptest.NewServer(failures.wrap(s.rf.GitHTTPHandler()))
	t.Cleanup(recording.Close)
	s.url = recording.URL

	work := s.clone(t)
	transportGit(t, work, "checkout", "-b", "live")
	const versions = 8
	padding := strings.Repeat("padding line for a bigger push\n", 4000)
	tips := make([]string, 0, versions)
	stateOf := map[string]string{}
	for i := 0; i < versions; i++ {
		transportCommit(t, work, "state.txt", fmt.Sprintf("version %d\n", i), fmt.Sprintf("version %d", i))
		tip := transportCommit(t, work, fmt.Sprintf("bulk/part-%d.txt", i), padding+fmt.Sprintf("%d\n", i), fmt.Sprintf("bulk %d", i))
		tips = append(tips, tip)
		stateOf[tip] = fmt.Sprintf("version %d\n", i)
	}
	transportGit(t, work, "push", "origin", tips[0]+":refs/heads/live")

	stop := make(chan struct{})
	var mu sync.Mutex
	var observed []observedClone
	var cloners sync.WaitGroup
	for c := 0; c < 3; c++ {
		cloners.Add(1)
		go func() {
			defer cloners.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				o := observeClone(s, "live")
				mu.Lock()
				observed = append(observed, o)
				mu.Unlock()
			}
		}()
	}

	began := time.Now()
	var pushFailures []string
	for i := 1; i < versions; i++ {
		out, err := transportGitMayFail(work, "push", "origin", tips[i]+":refs/heads/live")
		if err != nil {
			pushFailures = append(pushFailures, fmt.Sprintf("push %d: %v\n%s", i, err, out))
		}
	}
	pushing := time.Since(began)
	close(stop)
	cloners.Wait()

	for _, f := range pushFailures {
		t.Errorf("%s", f)
	}

	clean, missingBranch, torn, unknown := 0, 0, 0, 0
	for _, o := range observed {
		switch {
		case o.err != nil && strings.Contains(o.output, "not found"):
			missingBranch++
		case o.err != nil:
			torn++
			t.Errorf("a clone during the pushes failed: %v\n%s\n%s", o.err, o.output, o.fsck)
		case stateOf[o.head] == "":
			unknown++
			t.Errorf("a clone during the pushes landed on %s, which no push ever made the tip", o.head)
		case o.state != stateOf[o.head]:
			torn++
			t.Errorf("a clone at %s has state.txt %q, want %q", o.head, o.state, stateOf[o.head])
		default:
			clean++
		}
	}
	t.Logf("%d pushes of one branch took %s while %d clones ran: clean %d, branch missing %d, broken %d, unknown tip %d", versions-1, pushing, len(observed), clean, missingBranch, torn, unknown)
	for _, line := range failures.lines {
		t.Logf("server answered: %s", line)
	}
	if len(observed) == 0 {
		t.Fatal("no clone ran during the pushes")
	}

	live, err := s.rf.GetBranchByName(ctx, s.repo, "live")
	if err != nil {
		t.Fatalf("live not found: %v", err)
	}
	if live.SHA != tips[versions-1] {
		t.Errorf("live tip = %s, want %s", live.SHA, tips[versions-1])
	}
	final := s.clone(t, "--branch", "live")
	if got := transportGit(t, final, "rev-parse", "HEAD"); got != tips[versions-1] {
		t.Errorf("final clone HEAD = %s, want %s", got, tips[versions-1])
	}
}

func TestRepoFileGitHTTPConcurrency_ABranchMovedToACommitWhoseObjectsAreNotStoredYetIsStillCloneable(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()
	m := s.rf.GitManager.(*gitManager)

	work := s.clone(t)
	transportGit(t, work, "checkout", "-b", "live")
	oldTip := transportCommit(t, work, "state.txt", "old\n", "old state")
	transportGit(t, work, "push", "origin", "live")
	newTip := transportCommit(t, work, "state.txt", "new\n", "new state")
	transportGit(t, work, "push", "origin", newTip+":refs/heads/staging")

	live, err := s.rf.GetBranchByName(ctx, s.repo, "live")
	if err != nil {
		t.Fatalf("live not found: %v", err)
	}
	var newRow struct{ ID string }
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).Select("id").Where("sha = ?", newTip).Scan(&newRow).Error; err != nil || newRow.ID == "" {
		t.Fatalf("commit row of the new tip: %q, %v", newRow.ID, err)
	}
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).Where("id = ?", newRow.ID).Update("data", "").Error; err != nil {
		t.Fatalf("empty the new tip's stored object: %v", err)
	}
	if _, err := m.advanceBranchHead(ctx, live.ID, newRow.ID, ""); err != nil {
		t.Fatalf("advance live to the new tip: %v", err)
	}

	out, err := transportGitMayFail(t.TempDir(), "clone", "--branch", "live", s.url, "copy")
	if err != nil {
		t.Fatalf("a clone while live points at a commit whose object is not stored yet failed, so it saw neither the old tip %s nor the new tip %s:\n%s", oldTip, newTip, out)
	}
}

func TestRepoFileGitHTTPConcurrency_AToolWriteFromAnotherHandleToTheSameFileDoesNotLoseOrHidePushes(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()

	other := openRepoFileForTest(t, s.rf.Path())
	t.Cleanup(func() { _ = other.Close() })
	toolBranch, err := other.CreateBranch(ctx, CreateBranchRequest{RepositoryID: s.repo, Name: "second-handle"})
	if err != nil {
		t.Fatalf("CreateBranch through the second handle: %v", err)
	}

	items := prepareBranches(t, s, 10)

	stop := make(chan struct{})
	var writes, writeFailures int
	var firstFailure error
	var writer sync.WaitGroup
	writer.Add(1)
	go func() {
		defer writer.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			_, err := other.WriteFile(ctx, WriteFileRequest{BranchID: toolBranch.ID, Path: fmt.Sprintf("second/file-%d.txt", i), Content: fmt.Sprintf("second %d\n", i)})
			writes++
			if err != nil {
				writeFailures++
				if firstFailure == nil {
					firstFailure = err
				}
			}
		}
	}()

	began := time.Now()
	outcomes := pushTogether(items)
	elapsed := time.Since(began)
	close(stop)
	writer.Wait()

	refused := 0
	for _, o := range outcomes {
		if o.err != nil {
			refused++
			t.Errorf("push of %s refused: %v\n%s", o.branch, o.err, o.output)
		}
	}
	t.Logf("ten simultaneous pushes while a second handle wrote to the same file: %s, refused %d, tool writes %d, failed tool writes %d (first: %v)", elapsed, refused, writes, writeFailures, firstFailure)
	if writeFailures > 0 && !strings.Contains(firstFailure.Error(), "locked") && !strings.Contains(firstFailure.Error(), "busy") {
		t.Errorf("a tool write failed with a message that does not name the lock: %v", firstFailure)
	}
	for _, item := range items {
		s.requirePushedBranch(t, item)
	}
}
