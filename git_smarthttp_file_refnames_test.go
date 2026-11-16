package mwanachamagit

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	gogitplumbing "github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/capability"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func (s fileGitServer) branchNames(t *testing.T) []string {
	t.Helper()
	rows, err := s.rf.ListBranches(context.Background(), s.repo)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	return names
}

func (s fileGitServer) requireOnlyMain(t *testing.T) {
	t.Helper()
	names := s.branchNames(t)
	if len(names) != 1 || names[0] != "main" {
		t.Errorf("branches after a refused push = %v, want only main", names)
	}
}

func (s fileGitServer) requireFreshCloneWorks(t *testing.T) {
	t.Helper()
	fresh := s.clone(t)
	if got := readCloned(t, fresh, "README.md"); got != "# widgets\n" {
		t.Errorf("fresh clone README.md = %q", got)
	}
	transportGit(t, fresh, "fetch", "origin")
}

func (s fileGitServer) rawPush(t *testing.T, newSHA string, refs ...string) map[string]string {
	t.Helper()
	req := packp.NewReferenceUpdateRequest()
	req.Capabilities.Set(capability.ReportStatus) //nolint:errcheck
	for _, ref := range refs {
		req.Commands = append(req.Commands, &packp.Command{
			Name: gogitplumbing.ReferenceName(ref),
			Old:  gogitplumbing.ZeroHash,
			New:  gogitplumbing.NewHash(newSHA),
		})
	}
	var body bytes.Buffer
	if err := req.Encode(&body); err != nil {
		t.Fatalf("encode push request: %v", err)
	}
	resp, err := http.Post(s.url+"/git-receive-pack", "application/x-git-receive-pack-request", &body)
	if err != nil {
		t.Fatalf("post receive-pack: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("receive-pack status %d: %s", resp.StatusCode, raw)
	}
	report := packp.NewReportStatus()
	if err := report.Decode(resp.Body); err != nil {
		t.Fatalf("decode report-status: %v", err)
	}
	out := map[string]string{}
	for _, cs := range report.CommandStatuses {
		out[cs.ReferenceName.String()] = cs.Status
	}
	return out
}

func (s fileGitServer) mainTip(t *testing.T) string {
	t.Helper()
	row, err := s.rf.GetBranch(context.Background(), s.main)
	if err != nil {
		t.Fatalf("GetBranch main: %v", err)
	}
	return row.SHA
}

func TestRepoFileGitHTTP_PushOfABranchUnderTheDefaultBranchIsRefusedAndClonesStillWork(t *testing.T) {
	s := newFileGitServer(t)
	work := s.clone(t)
	transportGit(t, work, "checkout", "-b", "topic")
	transportCommit(t, work, "a.txt", "alpha\n", "alpha")

	out, err := transportGitMayFail(work, "push", "origin", "HEAD:refs/heads/main/x")
	if err == nil {
		t.Fatalf("push of main/x succeeded:\n%s", out)
	}
	if !strings.Contains(out, "clashes with the existing branch") {
		t.Errorf("refusal did not carry the plain message:\n%s", out)
	}

	s.requireOnlyMain(t)
	s.requireFreshCloneWorks(t)
}

func TestRepoFileGitHTTP_PushOfABranchThatIsADirectoryOfAnExistingBranchIsRefused(t *testing.T) {
	s := newFileGitServer(t)
	work := s.clone(t)
	transportGit(t, work, "checkout", "-b", "topic")
	transportCommit(t, work, "a.txt", "alpha\n", "alpha")
	transportGit(t, work, "push", "origin", "HEAD:refs/heads/team/alice")

	out, err := transportGitMayFail(work, "push", "origin", "HEAD:refs/heads/team")
	if err == nil {
		t.Fatalf("push of team succeeded while team/alice exists:\n%s", out)
	}
	if !strings.Contains(out, "clashes with the existing branch") {
		t.Errorf("refusal did not carry the plain message:\n%s", out)
	}
	out, err = transportGitMayFail(work, "push", "origin", "HEAD:refs/heads/team/alice/deep")
	if err == nil {
		t.Fatalf("push of team/alice/deep succeeded while team/alice exists:\n%s", out)
	}

	if names := s.branchNames(t); len(names) != 2 {
		t.Errorf("branches = %v, want main and team/alice", names)
	}
	s.requireFreshCloneWorks(t)
}

func TestRepoFileGitHTTP_PushOfASiblingUnderAnExistingDirectoryIsAccepted(t *testing.T) {
	s := newFileGitServer(t)
	work := s.clone(t)
	transportGit(t, work, "checkout", "-b", "topic")
	transportCommit(t, work, "a.txt", "alpha\n", "alpha")
	transportGit(t, work, "push", "origin", "HEAD:refs/heads/team/alice")
	transportGit(t, work, "push", "origin", "HEAD:refs/heads/team/bob")

	if names := s.branchNames(t); len(names) != 3 {
		t.Errorf("branches = %v, want main, team/alice and team/bob", names)
	}
	s.requireFreshCloneWorks(t)
}

func TestRepoFileGitHTTP_TwoClashingRefsInOnePushAreBothNotApplied(t *testing.T) {
	s := newFileGitServer(t)
	work := s.clone(t)
	transportGit(t, work, "checkout", "-b", "topic")
	transportCommit(t, work, "a.txt", "alpha\n", "alpha")

	out, err := transportGitMayFail(work, "push", "origin", "HEAD:refs/heads/p", "HEAD:refs/heads/p/q")
	if err == nil {
		t.Fatalf("push of p and p/q together succeeded:\n%s", out)
	}
	s.requireOnlyMain(t)
	s.requireFreshCloneWorks(t)
}

func TestRepoFileGitHTTP_InvalidBranchNamesAreRefusedPerRefWithAPlainMessage(t *testing.T) {
	names := []string{
		"refs/heads/",
		"refs/heads/a..b",
		"refs/heads/main.",
		"refs/heads/x.lock",
		"refs/heads/team/x.lock",
		"refs/heads//main",
		"refs/heads/team//x",
		"refs/heads/../heads/main",
		"refs/heads/.hidden",
		"refs/heads/team/.hidden",
		"refs/heads/ctrl\x01char",
		"refs/heads/del\x7fchar",
		"refs/heads/tilde~1",
		"refs/heads/caret^",
		"refs/heads/colon:name",
		"refs/heads/star*",
		"refs/heads/question?",
		"refs/heads/bracket[",
		"refs/heads/back\\slash",
		"refs/heads/at@{brace",
		"refs/heads/@",
		"refs/heads/-dash",
		"refs/heads/trailing/",
	}
	for _, name := range names {
		t.Run(strings.NewReplacer("\x01", "ctrl", "\x7f", "del").Replace(name), func(t *testing.T) {
			s := newFileGitServer(t)
			statuses := s.rawPush(t, s.mainTip(t), name)
			got, listed := statuses[name]
			if !listed {
				t.Fatalf("no status for %q in %v", name, statuses)
			}
			if !strings.Contains(got, "not a valid branch name") {
				t.Errorf("status for %q = %q, want the plain invalid-name message", name, got)
			}
			s.requireOnlyMain(t)
			s.requireFreshCloneWorks(t)
		})
	}
}

func TestRepoFileGitHTTP_OneRefusedNameKeepsTheOtherRefsOfThePushFromBeingApplied(t *testing.T) {
	s := newFileGitServer(t)
	statuses := s.rawPush(t, s.mainTip(t), "refs/heads/good", "refs/heads/bad..name")

	if !strings.Contains(statuses["refs/heads/bad..name"], "not a valid branch name") {
		t.Errorf("bad name status = %q", statuses["refs/heads/bad..name"])
	}
	if !strings.Contains(statuses["refs/heads/good"], "not applied") {
		t.Errorf("good name status = %q, want not applied", statuses["refs/heads/good"])
	}
	s.requireOnlyMain(t)
	s.requireFreshCloneWorks(t)
}

func TestRepoFileGitHTTP_ValidUnusualBranchNamesAreStillAccepted(t *testing.T) {
	s := newFileGitServer(t)
	work := s.clone(t)
	transportGit(t, work, "checkout", "-b", "topic")
	transportCommit(t, work, "a.txt", "alpha\n", "alpha")
	for _, name := range []string{"feature/ISS-91", "v1.2.3", "Main", "a-b_c"} {
		transportGit(t, work, "push", "origin", "HEAD:refs/heads/"+name)
	}
	if names := s.branchNames(t); len(names) != 5 {
		t.Errorf("branches = %v, want main plus four", names)
	}
	s.requireFreshCloneWorks(t)
}

func TestRepoFileGitHTTP_AStoredBranchWithAnInvalidOrClashingNameDoesNotBreakClones(t *testing.T) {
	s := newFileGitServer(t)
	m := s.rf.GitManager.(*gitManager)
	tip := s.mainTip(t)
	now := models.NowRFC3339()
	for _, name := range []string{"main/x", "", "a..b", "x.lock", "team", "team/alice", "team/alice/deep"} {
		row := models.Branch{Name: name, SHA: tip, CreatedAt: now, UpdatedAt: now}
		row.RepositoryID = s.repo
		ensureID(&row)
		if err := m.db.Table(m.tables.Branches).Create(&row).Error; err != nil {
			t.Fatalf("insert poisoned branch %q: %v", name, err)
		}
	}

	fresh := s.clone(t)
	if got := readCloned(t, fresh, "README.md"); got != "# widgets\n" {
		t.Errorf("clone README.md = %q", got)
	}
	remote := transportGit(t, fresh, "for-each-ref", "--format=%(refname)", "refs/remotes/origin")
	for _, bad := range []string{"origin/main/x", "origin/a..b", "origin/x.lock", "origin/team/alice/deep"} {
		if strings.Contains(remote, bad+"\n") || strings.HasSuffix(remote, bad) {
			t.Errorf("remote refs advertise the poisoned branch %q:\n%s", bad, remote)
		}
	}
	if !strings.Contains(remote, "refs/remotes/origin/main") {
		t.Errorf("remote refs lost main:\n%s", remote)
	}
	transportGit(t, fresh, "fetch", "origin")
}

func TestRepoFileGitHTTP_AStoredBranchWithAnInvalidNameCanStillBeDeletedByPush(t *testing.T) {
	s := newFileGitServer(t)
	m := s.rf.GitManager.(*gitManager)
	now := models.NowRFC3339()
	row := models.Branch{Name: "main/x", SHA: s.mainTip(t), CreatedAt: now, UpdatedAt: now}
	row.RepositoryID = s.repo
	ensureID(&row)
	if err := m.db.Table(m.tables.Branches).Create(&row).Error; err != nil {
		t.Fatalf("insert poisoned branch: %v", err)
	}

	req := packp.NewReferenceUpdateRequest()
	req.Capabilities.Set(capability.ReportStatus) //nolint:errcheck
	req.Commands = []*packp.Command{{Name: "refs/heads/main/x", Old: gogitplumbing.NewHash(row.SHA), New: gogitplumbing.ZeroHash}}
	var body bytes.Buffer
	if err := req.Encode(&body); err != nil {
		t.Fatalf("encode: %v", err)
	}
	resp, err := http.Post(s.url+"/git-receive-pack", "application/x-git-receive-pack-request", &body)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	_ = resp.Body.Close()

	s.requireOnlyMain(t)
}
