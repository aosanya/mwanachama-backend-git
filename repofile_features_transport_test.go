package mwanachamagit

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gogitplumbing "github.com/go-git/go-git/v5/plumbing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

const transportTimeout = 20 * time.Second

var transportZeroSHA = strings.Repeat("0", 40)

func transportRequireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git binary on PATH")
	}
}

func transportGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=Transport Test",
		"GIT_AUTHOR_EMAIL=transport@example.com",
		"GIT_COMMITTER_NAME=Transport Test",
		"GIT_COMMITTER_EMAIL=transport@example.com",
		"GIT_TERMINAL_PROMPT=0",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func transportGitMayFail(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=Transport Test",
		"GIT_AUTHOR_EMAIL=transport@example.com",
		"GIT_COMMITTER_NAME=Transport Test",
		"GIT_COMMITTER_EMAIL=transport@example.com",
		"GIT_TERMINAL_PROMPT=0",
	)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func transportCommit(t *testing.T, dir, path, content, message string) string {
	t.Helper()
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	transportGit(t, dir, "add", path)
	transportGit(t, dir, "commit", "-m", message)
	return transportGit(t, dir, "rev-parse", "HEAD")
}

type transportSource struct {
	BareDir     string
	FirstSHA    string
	SecondSHA   string
	FeatureSHA  string
	WorkingCopy string
}

func newTransportSource(t *testing.T) transportSource {
	t.Helper()
	transportRequireGit(t)
	bare := filepath.Join(t.TempDir(), "source.git")
	transportGit(t, t.TempDir(), "init", "--bare", "-b", "main", bare)
	work := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	transportGit(t, work, "init", "-b", "main")
	transportGit(t, work, "remote", "add", "origin", bare)
	src := transportSource{BareDir: bare, WorkingCopy: work}
	src.FirstSHA = transportCommit(t, work, "a.txt", "alpha\n", "first")
	src.SecondSHA = transportCommit(t, work, "docs/b.txt", "beta\n", "second")
	transportGit(t, work, "push", "origin", "main")
	transportGit(t, work, "checkout", "-b", "feature")
	src.FeatureSHA = transportCommit(t, work, "c.txt", "gamma\n", "feature work")
	transportGit(t, work, "push", "origin", "feature")
	return src
}

func transportManager(t *testing.T, rf *RepoFile) *gitManager {
	t.Helper()
	m, ok := rf.GitManager.(*gitManager)
	if !ok {
		t.Fatalf("RepoFile.GitManager is %T, want *gitManager", rf.GitManager)
	}
	return m
}

func newEmptyTransportFile(t *testing.T) (*RepoFile, *gitManager) {
	t.Helper()
	rf := openRepoFileForTest(t, filepath.Join(t.TempDir(), "transport.db"))
	t.Cleanup(func() { _ = rf.Close() })
	m := transportManager(t, rf)
	t.Cleanup(func() { removeTransportClones(m) })
	return rf, m
}

func removeTransportClones(m *gitManager) {
	var rows []models.Repository
	if err := m.db.Table(m.tables.Repositories).Find(&rows).Error; err != nil {
		return
	}
	for _, r := range rows {
		if r.BareClonePath != "" {
			_ = os.RemoveAll(r.BareClonePath)
		}
	}
}

func waitTransport(t *testing.T, what string, done func() (bool, string)) {
	t.Helper()
	deadline := time.Now().Add(transportTimeout)
	last := ""
	for time.Now().Before(deadline) {
		ok, state := done()
		if ok {
			return
		}
		last = state
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("%s did not finish within %s (last state: %s)", what, transportTimeout, last)
}

func waitImportDone(t *testing.T, rf *RepoFile, jobID string) models.ImportJob {
	t.Helper()
	var final models.ImportJob
	waitTransport(t, "import "+jobID, func() (bool, string) {
		job, err := rf.GetImportStatus(context.Background(), jobID)
		if err != nil {
			t.Fatalf("GetImportStatus: %v", err)
		}
		final = job
		switch job.Status {
		case importStatusCompleted, importStatusFailed, importStatusCancelled:
			return true, job.Status
		}
		return false, job.Status
	})
	return final
}

func waitBranchFetched(t *testing.T, m *gitManager, branchID string) {
	t.Helper()
	waitTransport(t, "branch "+branchID+" fetch", func() (bool, string) {
		var row models.Branch
		if err := m.db.Table(m.tables.Branches).Where("id = ?", branchID).First(&row).Error; err != nil {
			t.Fatalf("read branch row: %v", err)
		}
		if row.Status == branchStatusFetchFailed {
			t.Fatalf("branch fetch failed: %s", row.ErrorMessage)
		}
		return row.Status == branchStatusFetched, row.Status
	})
}

func waitFetchJobDone(t *testing.T, rf *RepoFile, jobID string) models.FetchBranchJob {
	t.Helper()
	var final models.FetchBranchJob
	waitTransport(t, "fetch job "+jobID, func() (bool, string) {
		job, err := rf.GetFetchBranchStatus(context.Background(), jobID)
		if err != nil {
			t.Fatalf("GetFetchBranchStatus: %v", err)
		}
		final = job
		return job.Status == fetchJobStatusCompleted || job.Status == fetchJobStatusFailed, job.Status
	})
	return final
}

type importedTransport struct {
	rf      *RepoFile
	m       *gitManager
	src     transportSource
	repo    models.Repository
	main    models.Branch
	feature models.Branch
}

func importTransportSource(t *testing.T) importedTransport {
	t.Helper()
	src := newTransportSource(t)
	rf, m := newEmptyTransportFile(t)
	ctx := context.Background()
	job, err := rf.ImportRepo(ctx, ImportRepoRequest{Name: "imported", SourceURL: src.BareDir, DefaultBranch: "main"})
	if err != nil {
		t.Fatalf("ImportRepo: %v", err)
	}
	final := waitImportDone(t, rf, job.ID)
	if final.Status != importStatusCompleted {
		t.Fatalf("import status = %q (%s), want completed", final.Status, final.ErrorMessage)
	}
	repo, err := rf.GetRepositoryByName(ctx, "imported")
	if err != nil {
		t.Fatalf("GetRepositoryByName: %v", err)
	}
	mainBranch, err := rf.GetBranchByName(ctx, repo.ID, "main")
	if err != nil {
		t.Fatalf("GetBranchByName main: %v", err)
	}
	featureBranch, err := rf.GetBranchByName(ctx, repo.ID, "feature")
	if err != nil {
		t.Fatalf("GetBranchByName feature: %v", err)
	}
	waitBranchFetched(t, m, mainBranch.ID)
	return importedTransport{rf: rf, m: m, src: src, repo: repo, main: mainBranch, feature: featureBranch}
}

func branchStatusOf(t *testing.T, m *gitManager, branchID string) string {
	t.Helper()
	var row models.Branch
	if err := m.db.Table(m.tables.Branches).Where("id = ?", branchID).First(&row).Error; err != nil {
		t.Fatalf("read branch row: %v", err)
	}
	return row.Status
}

func logMessagesOf(t *testing.T, rf *RepoFile, branchID string) []string {
	t.Helper()
	entries, err := rf.Log(context.Background(), branchID, LogFilter{})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	msgs := make([]string, 0, len(entries))
	for _, e := range entries {
		msgs = append(msgs, strings.TrimSpace(e.Message))
	}
	return msgs
}

func branchNamesOf(t *testing.T, rf *RepoFile, repoID string) []string {
	t.Helper()
	branches, err := rf.ListBranches(context.Background(), repoID)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	names := make([]string, 0, len(branches))
	for _, b := range branches {
		names = append(names, b.Name)
	}
	return names
}

func containsAll(have []string, want ...string) bool {
	set := map[string]bool{}
	for _, h := range have {
		set[h] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

func countRows(t *testing.T, m *gitManager, table string) int64 {
	t.Helper()
	var n int64
	if err := m.db.Table(table).Count(&n).Error; err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

type transportServer struct {
	rf   *RepoFile
	m    *gitManager
	repo models.Repository
	main models.Branch
	url  string
}

func newTransportServer(t *testing.T) transportServer {
	t.Helper()
	transportRequireGit(t)
	rf, repo, mainBranch := newFeatureRepo(t)
	m := transportManager(t, rf)
	srv := httptest.NewServer(rf.SmartHTTPHandler())
	t.Cleanup(srv.Close)
	t.Cleanup(func() { removeTransportClones(m) })
	return transportServer{rf: rf, m: m, repo: repo, main: mainBranch, url: srv.URL + "/" + featureRepoName}
}

func (s transportServer) workingCopy(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "wc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir wc: %v", err)
	}
	transportGit(t, dir, "init", "-b", "main")
	transportGit(t, dir, "remote", "add", "origin", s.url)
	return dir
}

func TestRepoFileFeatures_ImportRepo(t *testing.T) {
	imp := importTransportSource(t)
	ctx := context.Background()

	if imp.repo.Name != "imported" {
		t.Errorf("repository name = %q, want imported", imp.repo.Name)
	}
	if imp.repo.SourceURL != imp.src.BareDir {
		t.Errorf("repository source = %q, want %q", imp.repo.SourceURL, imp.src.BareDir)
	}
	if imp.repo.DefaultBranch != "main" {
		t.Errorf("default branch = %q, want main", imp.repo.DefaultBranch)
	}

	names := branchNamesOf(t, imp.rf, imp.repo.ID)
	if len(names) != 2 || !containsAll(names, "main", "feature") {
		t.Fatalf("imported branches = %v, want exactly main and feature", names)
	}
	if imp.main.SHA != imp.src.SecondSHA {
		t.Errorf("main tip = %q, want %q", imp.main.SHA, imp.src.SecondSHA)
	}
	if imp.feature.SHA != imp.src.FeatureSHA {
		t.Errorf("feature tip = %q, want %q", imp.feature.SHA, imp.src.FeatureSHA)
	}
	if status := branchStatusOf(t, imp.m, imp.feature.ID); status != branchStatusStub {
		t.Errorf("feature status = %q, want %q", status, branchStatusStub)
	}

	for path, want := range map[string]string{"a.txt": "alpha\n", "docs/b.txt": "beta\n"} {
		blob, err := imp.rf.ReadFile(ctx, imp.main.ID, path)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", path, err)
		}
		if blob.Path != path || blob.Content != want {
			t.Errorf("ReadFile %s = path %q content %q, want content %q", path, blob.Path, blob.Content, want)
		}
	}

	if got, want := logMessagesOf(t, imp.rf, imp.main.ID), []string{"second", "first"}; !reflect.DeepEqual(got, want) {
		t.Errorf("main log = %v, want %v", got, want)
	}
}

func TestRepoFileFeatures_ImportRepo_RefusedWhenARepositoryExists(t *testing.T) {
	src := newTransportSource(t)
	rf, _, _ := newFeatureRepo(t)
	_, err := rf.ImportRepo(context.Background(), ImportRepoRequest{Name: "imported", SourceURL: src.BareDir, DefaultBranch: "main"})
	if !errors.Is(err, ErrRepoAlreadyExists) {
		t.Fatalf("ImportRepo into a file that holds a repository: got %v, want ErrRepoAlreadyExists", err)
	}
	repos, err := rf.ListRepositories(context.Background())
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repos) != 1 || repos[0].Name != featureRepoName {
		t.Errorf("repositories after refused import = %+v, want only %s", repos, featureRepoName)
	}
}

func TestRepoFileFeatures_ImportRepo_SecondImportAfterCompletionIsRefused(t *testing.T) {
	imp := importTransportSource(t)
	_, err := imp.rf.ImportRepo(context.Background(), ImportRepoRequest{Name: "again", SourceURL: imp.src.BareDir, DefaultBranch: "main"})
	if !errors.Is(err, ErrRepoAlreadyExists) {
		t.Fatalf("second ImportRepo: got %v, want ErrRepoAlreadyExists", err)
	}
}

func TestRepoFileFeatures_GetImportStatus(t *testing.T) {
	src := newTransportSource(t)
	rf, _ := newEmptyTransportFile(t)
	ctx := context.Background()

	if _, err := rf.GetImportStatus(ctx, "no-such-job"); !errors.Is(err, ErrImportJobNotFound) {
		t.Fatalf("GetImportStatus unknown job: got %v, want ErrImportJobNotFound", err)
	}

	job, err := rf.ImportRepo(ctx, ImportRepoRequest{Name: "imported", SourceURL: src.BareDir, DefaultBranch: "main", Description: "mirror"})
	if err != nil {
		t.Fatalf("ImportRepo: %v", err)
	}
	if job.ID == "" || job.Status != importStatusPending || job.Name != "imported" || job.SourceURL != src.BareDir || job.DefaultBranch != "main" {
		t.Fatalf("returned job = %+v, want pending job for imported from %s", job, src.BareDir)
	}

	final := waitImportDone(t, rf, job.ID)
	if final.Status != importStatusCompleted || final.ErrorMessage != "" {
		t.Fatalf("final job = status %q error %q, want completed", final.Status, final.ErrorMessage)
	}
	if final.ID != job.ID || final.Name != "imported" || final.SourceURL != src.BareDir {
		t.Errorf("final job identity = %+v, want id %s name imported", final, job.ID)
	}
	if final.UpdatedAt == "" || final.CreatedAt == "" {
		t.Errorf("final job timestamps empty: %+v", final)
	}
}

func TestRepoFileFeatures_GetImportStatus_UnreachableSourceFails(t *testing.T) {
	transportRequireGit(t)
	rf, _ := newEmptyTransportFile(t)
	ctx := context.Background()
	missing := filepath.Join(t.TempDir(), "nowhere.git")

	job, err := rf.ImportRepo(ctx, ImportRepoRequest{Name: "ghost", SourceURL: missing, DefaultBranch: "main"})
	if err != nil {
		t.Fatalf("ImportRepo: %v", err)
	}
	final := waitImportDone(t, rf, job.ID)
	if final.Status != importStatusFailed {
		t.Fatalf("status = %q, want failed", final.Status)
	}
	if !strings.Contains(final.ErrorMessage, "bare shallow clone") {
		t.Errorf("error message = %q, want it to name the failed clone", final.ErrorMessage)
	}
	repos, err := rf.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("repositories after failed import = %+v, want none", repos)
	}

	retry := importAfterFailure(t, rf, ctx)
	if retry.Status != importStatusCompleted {
		t.Errorf("import after a failed one: status %q (%s), want completed", retry.Status, retry.ErrorMessage)
	}
}

func importAfterFailure(t *testing.T, rf *RepoFile, ctx context.Context) models.ImportJob {
	t.Helper()
	src := newTransportSource(t)
	job, err := rf.ImportRepo(ctx, ImportRepoRequest{Name: "second-try", SourceURL: src.BareDir, DefaultBranch: "main"})
	if err != nil {
		t.Fatalf("ImportRepo after failure: %v", err)
	}
	return waitImportDone(t, rf, job.ID)
}

func insertPendingImportJob(t *testing.T, m *gitManager) models.ImportJob {
	t.Helper()
	now := models.NowRFC3339()
	row := models.ImportJob{Name: "queued", SourceURL: "/nowhere", DefaultBranch: "main", Status: importStatusPending, CreatedAt: now, UpdatedAt: now}
	ensureID(&row)
	if err := m.db.Table(m.tables.ImportJobs).Create(&row).Error; err != nil {
		t.Fatalf("insert pending job: %v", err)
	}
	return row
}

func TestRepoFileFeatures_CancelImport(t *testing.T) {
	rf, m := newEmptyTransportFile(t)
	ctx := context.Background()

	if err := rf.CancelImport(ctx, "no-such-job"); !errors.Is(err, ErrImportJobNotFound) {
		t.Fatalf("CancelImport unknown job: got %v, want ErrImportJobNotFound", err)
	}

	pending := insertPendingImportJob(t, m)
	_, err := rf.ImportRepo(ctx, ImportRepoRequest{Name: "blocked", SourceURL: "/nowhere", DefaultBranch: "main"})
	if !errors.Is(err, ErrImportInProgress) {
		t.Fatalf("ImportRepo while a job is pending: got %v, want ErrImportInProgress", err)
	}

	if err := rf.CancelImport(ctx, pending.ID); err != nil {
		t.Fatalf("CancelImport pending job: %v", err)
	}
	job, err := rf.GetImportStatus(ctx, pending.ID)
	if err != nil {
		t.Fatalf("GetImportStatus: %v", err)
	}
	if job.Status != importStatusCancelled {
		t.Errorf("status after cancel = %q, want cancelled", job.Status)
	}
	if err := rf.CancelImport(ctx, pending.ID); !errors.Is(err, ErrImportJobNotCancellable) {
		t.Errorf("CancelImport on a cancelled job: got %v, want ErrImportJobNotCancellable", err)
	}
}

func TestRepoFileFeatures_CancelImport_CompletedJobCannotBeCancelled(t *testing.T) {
	src := newTransportSource(t)
	rf, _ := newEmptyTransportFile(t)
	ctx := context.Background()
	job, err := rf.ImportRepo(ctx, ImportRepoRequest{Name: "imported", SourceURL: src.BareDir, DefaultBranch: "main"})
	if err != nil {
		t.Fatalf("ImportRepo: %v", err)
	}
	if final := waitImportDone(t, rf, job.ID); final.Status != importStatusCompleted {
		t.Fatalf("import status = %q (%s), want completed", final.Status, final.ErrorMessage)
	}
	if err := rf.CancelImport(ctx, job.ID); !errors.Is(err, ErrImportJobNotCancellable) {
		t.Fatalf("CancelImport on a completed job: got %v, want ErrImportJobNotCancellable", err)
	}
	after, err := rf.GetImportStatus(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetImportStatus: %v", err)
	}
	if after.Status != importStatusCompleted {
		t.Errorf("status after refused cancel = %q, want completed", after.Status)
	}
}

func TestRepoFileFeatures_FetchBranch(t *testing.T) {
	imp := importTransportSource(t)
	ctx := context.Background()

	job, err := imp.rf.FetchBranch(ctx, FetchBranchRequest{RepoID: imp.repo.ID, BranchID: imp.feature.ID})
	if err != nil {
		t.Fatalf("FetchBranch feature: %v", err)
	}
	if job.ID == "" || job.RepoID != imp.repo.ID || job.BranchName != "feature" {
		t.Fatalf("returned job = %+v, want a job for feature in %s", job, imp.repo.ID)
	}

	final := waitFetchJobDone(t, imp.rf, job.ID)
	if final.Status != fetchJobStatusCompleted || final.ErrorMessage != "" {
		t.Fatalf("fetch job = status %q error %q, want completed", final.Status, final.ErrorMessage)
	}
	waitBranchFetched(t, imp.m, imp.feature.ID)

	blob, err := imp.rf.ReadFile(ctx, imp.feature.ID, "c.txt")
	if err != nil {
		t.Fatalf("ReadFile c.txt on feature: %v", err)
	}
	if blob.Content != "gamma\n" {
		t.Errorf("c.txt content = %q, want gamma", blob.Content)
	}
	inherited, err := imp.rf.ReadFile(ctx, imp.feature.ID, "docs/b.txt")
	if err != nil {
		t.Fatalf("ReadFile docs/b.txt on feature: %v", err)
	}
	if inherited.Content != "beta\n" {
		t.Errorf("docs/b.txt content on feature = %q, want beta", inherited.Content)
	}

	if got, want := logMessagesOf(t, imp.rf, imp.feature.ID), []string{"feature work", "second", "first"}; !reflect.DeepEqual(got, want) {
		t.Errorf("feature log = %v, want %v", got, want)
	}

	reloaded, err := imp.rf.GetBranchByName(ctx, imp.repo.ID, "feature")
	if err != nil {
		t.Fatalf("GetBranchByName feature: %v", err)
	}
	if reloaded.SHA != imp.src.FeatureSHA || reloaded.HeadCommitID == "" {
		t.Errorf("feature after fetch = sha %q head %q, want sha %q and a head commit", reloaded.SHA, reloaded.HeadCommitID, imp.src.FeatureSHA)
	}
}

func TestRepoFileFeatures_FetchBranch_RefusesFetchedAndUnknownBranches(t *testing.T) {
	imp := importTransportSource(t)
	ctx := context.Background()

	_, err := imp.rf.FetchBranch(ctx, FetchBranchRequest{RepoID: imp.repo.ID, BranchID: imp.main.ID})
	if !errors.Is(err, ErrBranchAlreadyFetched) {
		t.Errorf("FetchBranch on the already fetched default branch: got %v, want ErrBranchAlreadyFetched", err)
	}
	if _, err := imp.rf.FetchBranch(ctx, FetchBranchRequest{RepoID: imp.repo.ID, BranchID: "no-such-branch"}); err == nil {
		t.Error("FetchBranch on an unknown branch: got nil error")
	}

	job, err := imp.rf.FetchBranch(ctx, FetchBranchRequest{RepoID: imp.repo.ID, BranchID: imp.feature.ID})
	if err != nil {
		t.Fatalf("FetchBranch feature: %v", err)
	}
	waitFetchJobDone(t, imp.rf, job.ID)
	if _, err := imp.rf.FetchBranch(ctx, FetchBranchRequest{RepoID: imp.repo.ID, BranchID: imp.feature.ID}); !errors.Is(err, ErrBranchAlreadyFetched) {
		t.Errorf("FetchBranch on a branch already fetched: got %v, want ErrBranchAlreadyFetched", err)
	}
}

func TestRepoFileFeatures_GetFetchBranchStatus(t *testing.T) {
	imp := importTransportSource(t)
	ctx := context.Background()

	if _, err := imp.rf.GetFetchBranchStatus(ctx, "no-such-job"); !errors.Is(err, ErrImportJobNotFound) {
		t.Fatalf("GetFetchBranchStatus unknown job: got %v, want ErrImportJobNotFound", err)
	}

	job, err := imp.rf.FetchBranch(ctx, FetchBranchRequest{RepoID: imp.repo.ID, BranchID: imp.feature.ID})
	if err != nil {
		t.Fatalf("FetchBranch: %v", err)
	}
	final := waitFetchJobDone(t, imp.rf, job.ID)
	if final.ID != job.ID || final.RepoID != imp.repo.ID || final.BranchName != "feature" || final.Status != fetchJobStatusCompleted {
		t.Errorf("fetch status = %+v, want completed job %s for feature", final, job.ID)
	}
	if final.CreatedAt == "" || final.UpdatedAt == "" {
		t.Errorf("fetch status timestamps empty: %+v", final)
	}
}

func TestRepoFileFeatures_GetFetchBranchStatus_PushIndexedBranchCompletesAtOnce(t *testing.T) {
	s := newTransportServer(t)
	ctx := context.Background()
	wc := s.workingCopy(t)
	transportCommit(t, wc, "a.txt", "alpha\n", "first")
	transportGit(t, wc, "push", "origin", "main")

	pushed, err := s.rf.GetBranchByName(ctx, s.repo.ID, "main")
	if err != nil {
		t.Fatalf("GetBranchByName main: %v", err)
	}
	if pushed.HeadCommitID == "" {
		t.Fatal("pushed main has no head commit")
	}

	job, err := s.rf.FetchBranch(ctx, FetchBranchRequest{RepoID: s.repo.ID, BranchID: pushed.ID})
	if err != nil {
		t.Fatalf("FetchBranch on a push-indexed branch: %v", err)
	}
	if job.Status != fetchJobStatusCompleted || job.BranchName != "main" {
		t.Errorf("returned job = %+v, want completed job for main", job)
	}
	status, err := s.rf.GetFetchBranchStatus(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetFetchBranchStatus: %v", err)
	}
	if status.Status != fetchJobStatusCompleted {
		t.Errorf("stored status = %q, want completed", status.Status)
	}
	if got := branchStatusOf(t, s.m, pushed.ID); got != branchStatusFetched {
		t.Errorf("branch status = %q, want %q", got, branchStatusFetched)
	}
}

func TestRepoFileFeatures_IndexPushedBranch(t *testing.T) {
	s := newTransportServer(t)
	ctx := context.Background()
	wc := s.workingCopy(t)
	first := transportCommit(t, wc, "a.txt", "alpha\n", "first")
	second := transportCommit(t, wc, "docs/b.txt", "beta\n", "second")
	transportGit(t, wc, "push", "origin", "main")

	repo, err := s.rf.GetRepositoryByName(ctx, featureRepoName)
	if err != nil {
		t.Fatalf("GetRepositoryByName: %v", err)
	}
	if repo.BareClonePath == "" {
		t.Fatal("repository has no bare clone after a push")
	}
	bare, err := gogit.PlainOpen(repo.BareClonePath)
	if err != nil {
		t.Fatalf("open bare clone: %v", err)
	}
	topic := gogitplumbing.NewHashReference(gogitplumbing.NewBranchReferenceName("topic"), gogitplumbing.NewHash(first))
	if err := bare.Storer.SetReference(topic); err != nil {
		t.Fatalf("set topic ref: %v", err)
	}

	commitsBefore := countRows(t, s.m, s.m.tables.Commits)
	blobsBefore := countRows(t, s.m, s.m.tables.Blobs)
	if err := s.rf.IndexPushedBranch(ctx, featureRepoName, "refs/heads/topic", transportZeroSHA, first); err != nil {
		t.Fatalf("IndexPushedBranch topic: %v", err)
	}
	branch, err := s.rf.GetBranchByName(ctx, repo.ID, "topic")
	if err != nil {
		t.Fatalf("GetBranchByName topic: %v", err)
	}
	if branch.SHA != first || branch.HeadCommitID == "" {
		t.Errorf("topic = sha %q head %q, want sha %q and a head commit", branch.SHA, branch.HeadCommitID, first)
	}
	if got, want := logMessagesOf(t, s.rf, branch.ID), []string{"first"}; !reflect.DeepEqual(got, want) {
		t.Errorf("topic log = %v, want %v", got, want)
	}
	blob, err := s.rf.ReadFile(ctx, branch.ID, "a.txt")
	if err != nil {
		t.Fatalf("ReadFile a.txt on topic: %v", err)
	}
	if blob.Content != "alpha\n" {
		t.Errorf("a.txt on topic = %q, want alpha", blob.Content)
	}
	if got := countRows(t, s.m, s.m.tables.Commits); got != commitsBefore {
		t.Errorf("commit rows after re-indexing an indexed commit = %d, want %d", got, commitsBefore)
	}
	if got := countRows(t, s.m, s.m.tables.Blobs); got != blobsBefore {
		t.Errorf("blob rows after re-indexing an indexed tree = %d, want %d", got, blobsBefore)
	}

	if err := s.rf.IndexPushedBranch(ctx, featureRepoName, "refs/heads/topic", transportZeroSHA, first); err != nil {
		t.Fatalf("second IndexPushedBranch topic: %v", err)
	}
	if got := countRows(t, s.m, s.m.tables.Commits); got != commitsBefore {
		t.Errorf("commit rows after repeating the index = %d, want %d", got, commitsBefore)
	}
	topics, err := s.rf.ListBranchesFiltered(ctx, repo.ID, BranchFilter{})
	if err != nil {
		t.Fatalf("ListBranchesFiltered: %v", err)
	}
	if len(topics) != 2 {
		t.Errorf("branches = %d, want main and topic only", len(topics))
	}

	mainBranch, err := s.rf.GetBranchByName(ctx, repo.ID, "main")
	if err != nil {
		t.Fatalf("GetBranchByName main: %v", err)
	}
	if mainBranch.SHA != second {
		t.Errorf("main tip = %q, want %q", mainBranch.SHA, second)
	}
	if got, want := logMessagesOf(t, s.rf, mainBranch.ID), []string{"second", "first"}; !reflect.DeepEqual(got, want) {
		t.Errorf("main log = %v, want %v", got, want)
	}
}

func TestRepoFileFeatures_IndexPushedBranch_NonBranchAndUnknownRefs(t *testing.T) {
	s := newTransportServer(t)
	ctx := context.Background()
	wc := s.workingCopy(t)
	first := transportCommit(t, wc, "a.txt", "alpha\n", "first")
	transportGit(t, wc, "push", "origin", "main")

	before, err := s.rf.ListBranches(ctx, s.repo.ID)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if err := s.rf.IndexPushedBranch(ctx, featureRepoName, "refs/notes/commits", transportZeroSHA, first); err != nil {
		t.Errorf("IndexPushedBranch on a notes ref: got %v, want nil", err)
	}
	after, err := s.rf.ListBranches(ctx, s.repo.ID)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(after) != len(before) {
		t.Errorf("branches after a notes ref = %d, want %d", len(after), len(before))
	}
	if err := s.rf.IndexPushedBranch(ctx, "other", "refs/heads/main", transportZeroSHA, first); err == nil {
		t.Error("IndexPushedBranch for a repository the file does not hold: got nil error")
	}
}

func TestRepoFileFeatures_IndexPushedBranch_PushedTagIsATag(t *testing.T) {
	s := newTransportServer(t)
	ctx := context.Background()
	wc := s.workingCopy(t)
	first := transportCommit(t, wc, "a.txt", "alpha\n", "first")
	transportGit(t, wc, "push", "origin", "main")
	transportGit(t, wc, "tag", "v1")
	transportGit(t, wc, "push", "origin", "v1")

	tags, err := s.rf.ListTags(ctx, s.repo.ID)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "v1" || tags[0].SHA != first {
		t.Fatalf("tags = %+v, want one tag v1 at %s", tags, first)
	}
	names := branchNamesOf(t, s.rf, s.repo.ID)
	if len(names) != 1 || names[0] != "main" {
		t.Errorf("branches after a tag push = %v, want only main", names)
	}
}

func TestRepoFileFeatures_SmartHTTPHandler(t *testing.T) {
	s := newTransportServer(t)
	ctx := context.Background()
	wc := s.workingCopy(t)
	transportCommit(t, wc, "a.txt", "alpha\n", "first")
	second := transportCommit(t, wc, "docs/b.txt", "beta\n", "second")
	transportGit(t, wc, "push", "origin", "main")

	repo, err := s.rf.GetRepositoryByName(ctx, featureRepoName)
	if err != nil {
		t.Fatalf("GetRepositoryByName: %v", err)
	}
	if repo.ID != s.repo.ID {
		t.Errorf("push created repository %s, want it to use the file's existing %s", repo.ID, s.repo.ID)
	}
	repos, err := s.rf.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("repositories after push = %d, want 1", len(repos))
	}

	branch, err := s.rf.GetBranchByName(ctx, repo.ID, "main")
	if err != nil {
		t.Fatalf("GetBranchByName main: %v", err)
	}
	if branch.ID != s.main.ID {
		t.Errorf("push created branch %s, want it to advance the existing main %s", branch.ID, s.main.ID)
	}
	if branch.SHA != second {
		t.Errorf("main tip = %q, want %q", branch.SHA, second)
	}
	if got, want := logMessagesOf(t, s.rf, branch.ID), []string{"second", "first"}; !reflect.DeepEqual(got, want) {
		t.Errorf("main log = %v, want %v", got, want)
	}
	for path, want := range map[string]string{"a.txt": "alpha\n", "docs/b.txt": "beta\n"} {
		blob, err := s.rf.ReadFile(ctx, branch.ID, path)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", path, err)
		}
		if blob.Content != want {
			t.Errorf("%s = %q, want %q", path, blob.Content, want)
		}
	}

	third := transportCommit(t, wc, "docs/c.txt", "gamma\n", "third")
	transportGit(t, wc, "push", "origin", "main")
	advanced, err := s.rf.GetBranchByName(ctx, repo.ID, "main")
	if err != nil {
		t.Fatalf("GetBranchByName main: %v", err)
	}
	if advanced.SHA != third {
		t.Errorf("main tip after second push = %q, want %q", advanced.SHA, third)
	}
	if got, want := logMessagesOf(t, s.rf, advanced.ID), []string{"third", "second", "first"}; !reflect.DeepEqual(got, want) {
		t.Errorf("main log after second push = %v, want %v", got, want)
	}
	var shaRows int64
	if err := s.m.db.Table(s.m.tables.Commits).Where("sha = ?", second).Count(&shaRows).Error; err != nil {
		t.Fatalf("count commits: %v", err)
	}
	if shaRows != 1 {
		t.Errorf("commit rows for %s = %d, want 1", second, shaRows)
	}
}

func TestRepoFileFeatures_SmartHTTPHandler_Clone(t *testing.T) {
	s := newTransportServer(t)
	wc := s.workingCopy(t)
	transportCommit(t, wc, "a.txt", "alpha\n", "first")
	second := transportCommit(t, wc, "docs/b.txt", "beta\n", "second")
	transportGit(t, wc, "push", "origin", "main")

	parent := t.TempDir()
	transportGit(t, parent, "clone", s.url, "copy")
	clone := filepath.Join(parent, "copy")

	if got := transportGit(t, clone, "rev-parse", "HEAD"); got != second {
		t.Errorf("clone HEAD = %q, want %q", got, second)
	}
	if got := transportGit(t, clone, "log", "--format=%s"); got != "second\nfirst" {
		t.Errorf("clone log = %q, want second then first", got)
	}
	for path, want := range map[string]string{"a.txt": "alpha\n", "docs/b.txt": "beta\n"} {
		data, err := os.ReadFile(filepath.Join(clone, path))
		if err != nil {
			t.Fatalf("read cloned %s: %v", path, err)
		}
		if string(data) != want {
			t.Errorf("cloned %s = %q, want %q", path, data, want)
		}
	}
}

func TestRepoFileFeatures_SmartHTTPHandler_AnotherCloneSeesLaterPush(t *testing.T) {
	s := newTransportServer(t)
	wc := s.workingCopy(t)
	transportCommit(t, wc, "a.txt", "alpha\n", "first")
	transportGit(t, wc, "push", "origin", "main")

	parent := t.TempDir()
	transportGit(t, parent, "clone", s.url, "copy")
	clone := filepath.Join(parent, "copy")

	later := transportCommit(t, wc, "later.txt", "later\n", "later")
	transportGit(t, wc, "push", "origin", "main")
	transportGit(t, clone, "pull", "origin", "main")
	if got := transportGit(t, clone, "rev-parse", "HEAD"); got != later {
		t.Errorf("clone HEAD after pull = %q, want %q", got, later)
	}
}

func TestRepoFileFeatures_SmartHTTPHandler_BranchPushAndDelete(t *testing.T) {
	s := newTransportServer(t)
	ctx := context.Background()
	wc := s.workingCopy(t)
	transportCommit(t, wc, "a.txt", "alpha\n", "first")
	transportGit(t, wc, "push", "origin", "main")
	transportGit(t, wc, "checkout", "-b", "topic")
	topicSHA := transportCommit(t, wc, "t.txt", "topic\n", "topic work")
	transportGit(t, wc, "push", "origin", "topic")

	topic, err := s.rf.GetBranchByName(ctx, s.repo.ID, "topic")
	if err != nil {
		t.Fatalf("GetBranchByName topic: %v", err)
	}
	if topic.SHA != topicSHA {
		t.Errorf("topic tip = %q, want %q", topic.SHA, topicSHA)
	}
	if got, want := logMessagesOf(t, s.rf, topic.ID), []string{"topic work", "first"}; !reflect.DeepEqual(got, want) {
		t.Errorf("topic log = %v, want %v", got, want)
	}

	transportGit(t, wc, "push", "origin", "--delete", "topic")
	if _, err := s.rf.GetBranchByName(ctx, s.repo.ID, "topic"); err == nil {
		t.Error("topic still resolvable after a delete push")
	}
	if _, err := s.rf.GetBranchByName(ctx, s.repo.ID, "main"); err != nil {
		t.Errorf("main lost after deleting topic: %v", err)
	}
}

func TestRepoFileFeatures_SmartHTTPHandler_PushToAnotherRepositoryNameIsRefused(t *testing.T) {
	s := newTransportServer(t)
	ctx := context.Background()
	wc := s.workingCopy(t)
	transportCommit(t, wc, "a.txt", "alpha\n", "first")
	transportGit(t, wc, "remote", "set-url", "origin", strings.TrimSuffix(s.url, "/"+featureRepoName)+"/other")

	if out, err := transportGitMayFail(wc, "push", "origin", "main"); err == nil {
		t.Fatalf("push to a second repository name succeeded in a single-repository file:\n%s", out)
	}
	repos, err := s.rf.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repos) != 1 || repos[0].Name != featureRepoName {
		t.Errorf("repositories after refused push = %+v, want only %s", repos, featureRepoName)
	}
	if _, err := s.rf.GetRepositoryByName(ctx, "other"); err == nil {
		t.Error("repository other exists after a refused push")
	}
}
