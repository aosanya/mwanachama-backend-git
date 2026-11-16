package mwanachamagit

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

const (
	notABranchRefusal = "only branches can be pushed to the hub"
	withOthersRefusal = "not applied because another ref in this push was refused"
)

func TestDecidedGitPush_ARefOutsideRefsHeadsIsRefusedPerRefAndTheBranchBesideItIsNotApplied(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()
	work := s.clone(t)
	mainTip := transportGit(t, work, "rev-parse", "HEAD")
	transportGit(t, work, "checkout", "-b", "topic")
	tip := transportCommit(t, work, "a.txt", "alpha\n", "alpha")

	statuses := sendCommands(t, s.url, work, mainTip,
		pushCommand{"refs/notes/review", transportZeroSHA, tip},
		pushCommand{"refs/heads/topic", transportZeroSHA, tip},
	)

	if statuses["refs/notes/review"] != notABranchRefusal {
		t.Errorf("refs/notes/review status = %q, want %q", statuses["refs/notes/review"], notABranchRefusal)
	}
	if statuses["refs/heads/topic"] != withOthersRefusal {
		t.Errorf("refs/heads/topic status = %q, want %q", statuses["refs/heads/topic"], withOthersRefusal)
	}
	if _, err := s.rf.GetBranchByName(ctx, s.repo, "topic"); err == nil {
		t.Error("topic was created although the push was refused")
	}
}

func TestDecidedGitPush_ABranchPushedOverGitIsMergedThroughAMergeRequestIntoMain(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()
	work := s.clone(t)
	transportGit(t, work, "checkout", "-b", "feature")
	transportCommit(t, work, "docs/new.md", "new page\n", "add a page")
	tip := transportCommit(t, work, "src/app.txt", "app\n", "add app")
	transportGit(t, work, "push", "origin", "feature")

	feature, err := s.rf.GetBranchByName(ctx, s.repo, "feature")
	if err != nil {
		t.Fatalf("GetBranchByName feature: %v", err)
	}
	if feature.SHA != tip {
		t.Fatalf("feature tip = %s, want %s", feature.SHA, tip)
	}
	mr, err := s.rf.CreateMergeRequest(ctx, CreateMergeRequestRequest{
		RepositoryID:   s.repo,
		Title:          "pushed feature",
		SourceBranchID: feature.ID,
		TargetBranchID: s.main,
		AuthorName:     "pusher",
	})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}
	done, err := s.rf.CompleteMergeRequest(ctx, mr.ID, "pusher")
	if err != nil {
		t.Fatalf("CompleteMergeRequest: %v", err)
	}
	if done.Status != models.MergeRequestStatusMerged {
		t.Fatalf("merge request status = %q, want merged", done.Status)
	}

	for path, want := range map[string]string{"docs/new.md": "new page\n", "src/app.txt": "app\n", "README.md": "# widgets\n"} {
		file, err := s.rf.ReadFile(ctx, s.main, path)
		if err != nil || file.Content != want {
			t.Errorf("main %s = %q, %v, want %q", path, file.Content, err, want)
		}
	}
	fresh := s.clone(t)
	for path, want := range map[string]string{"docs/new.md": "new page\n", "src/app.txt": "app\n"} {
		if got := readCloned(t, fresh, path); got != want {
			t.Errorf("clone of main %s = %q, want %q", path, got, want)
		}
	}
}

func TestDecidedGitClone_AGitDepthCloneIsRefusedTodayAndTheShallowWorkMustChangeThisTestOnPurpose(t *testing.T) {
	s := newFileGitServer(t)

	out, err := transportGitMayFail(t.TempDir(), "clone", "--depth", "1", s.url, "shallow")
	if err == nil {
		t.Fatalf("a shallow clone succeeded; the shallow work has landed, so replace this test with one that checks the depth:\n%s", out)
	}
	if !strings.Contains(out, "Server does not support shallow clients") {
		t.Errorf("the shallow clone did not fail with the plain client message:\n%s", out)
	}
}

func TestDecidedGitClone_AnUploadPackRequestWithADepthIsAnswered501UntilTheShallowWorkMustChangeThisTestOnPurpose(t *testing.T) {
	s := newFileGitServer(t)
	work := s.clone(t)
	mainTip := transportGit(t, work, "rev-parse", "HEAD")

	body := strings.NewReader(pktLine("want "+mainTip+" deepen-relative\n") + pktLine("deepen 1\n") + "0000" + pktLine("done\n"))
	resp, err := http.Post(s.url+"/git-upload-pack", "application/x-git-upload-pack-request", body)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("a depth request answered %d, want 501 until the shallow work lands", resp.StatusCode)
	}
}

func pktLine(payload string) string {
	length := len(payload) + 4
	const hex = "0123456789abcdef"
	return string([]byte{hex[length>>12&15], hex[length>>8&15], hex[length>>4&15], hex[length&15]}) + payload
}
