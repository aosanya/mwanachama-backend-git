package mwanachamagit

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
)

type pushCommand struct {
	ref string
	old string
	new string
}

func packFor(t *testing.T, workdir, tip, exclude string) []byte {
	t.Helper()
	cmd := exec.Command("git", "pack-objects", "--stdout", "--revs", "--thin")
	cmd.Dir = workdir
	input := tip + "\n"
	if exclude != "" {
		input += "^" + exclude + "\n"
	}
	cmd.Stdin = strings.NewReader(input)
	pack, err := cmd.Output()
	if err != nil {
		t.Fatalf("pack-objects: %v", err)
	}
	return pack
}

func receivePackBody(t *testing.T, workdir, exclude string, commands ...pushCommand) *bytes.Buffer {
	t.Helper()
	req := packp.NewReferenceUpdateRequest()
	req.Capabilities.Set("report-status")
	for _, c := range commands {
		req.Commands = append(req.Commands, &packp.Command{
			Name: plumbing.ReferenceName(c.ref),
			Old:  plumbing.NewHash(c.old),
			New:  plumbing.NewHash(c.new),
		})
	}
	req.Packfile = io.NopCloser(bytes.NewReader(packFor(t, workdir, commands[0].new, exclude)))
	var body bytes.Buffer
	if err := req.Encode(&body); err != nil {
		t.Fatalf("encode request: %v", err)
	}
	return &body
}

func reportStatuses(t *testing.T, code int, status string, body io.Reader) map[string]string {
	t.Helper()
	statuses := map[string]string{}
	if code != http.StatusOK {
		statuses["http"] = status
		return statuses
	}
	report := packp.NewReportStatus()
	if err := report.Decode(body); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	for _, cs := range report.CommandStatuses {
		statuses[cs.ReferenceName.String()] = cs.Status
	}
	return statuses
}

func sendCommands(t *testing.T, url, workdir, exclude string, commands ...pushCommand) map[string]string {
	t.Helper()
	resp, err := http.Post(url+"/git-receive-pack", "application/x-git-receive-pack-request", receivePackBody(t, workdir, exclude, commands...))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	return reportStatuses(t, resp.StatusCode, resp.Status, resp.Body)
}

func pushedTopic(t *testing.T, s fileGitServer) (work, tip string) {
	t.Helper()
	work = s.clone(t)
	transportGit(t, work, "checkout", "-b", "topic")
	tip = transportCommit(t, work, "a.txt", "alpha\n", "alpha")
	transportGit(t, work, "push", "origin", "topic")
	return work, tip
}

func TestPushGuard_AStalePushAfterAToolWriteIsRefusedAndTheToolCommitStaysOnTheBranch(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()
	work, pushed := pushedTopic(t, s)
	topic, err := s.rf.GetBranchByName(ctx, s.repo, "topic")
	if err != nil {
		t.Fatal(err)
	}
	tool, err := s.rf.WriteFile(ctx, WriteFileRequest{BranchID: topic.ID, Path: "tool.txt", Content: "tool\n"})
	if err != nil {
		t.Fatal(err)
	}
	next := transportCommit(t, work, "b.txt", "beta\n", "beta")

	statuses := sendCommands(t, s.url, work, pushed, pushCommand{"refs/heads/topic", pushed, next})

	if statuses["refs/heads/topic"] == "ok" || statuses["refs/heads/topic"] == "" {
		t.Errorf("stale push status = %q, want a plain refusal", statuses["refs/heads/topic"])
	}
	after, err := s.rf.GetBranchByName(ctx, s.repo, "topic")
	if err != nil {
		t.Fatal(err)
	}
	if after.SHA != tool.SHA {
		t.Errorf("topic = %s, want the tool commit %s", after.SHA, tool.SHA)
	}
	file, err := s.rf.ReadFile(ctx, after.ID, "tool.txt")
	if err != nil || file.Content != "tool\n" {
		t.Errorf("tool.txt after the stale push = %q, %v", file.Content, err)
	}
	fresh := s.clone(t, "--branch", "topic")
	if got := transportGit(t, fresh, "rev-parse", "HEAD"); got != tool.SHA {
		t.Errorf("clone HEAD = %s, want %s", got, tool.SHA)
	}
}

func TestPushGuard_ACurrentPushStillMovesTheBranch(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()
	work, pushed := pushedTopic(t, s)
	next := transportCommit(t, work, "b.txt", "beta\n", "beta")

	statuses := sendCommands(t, s.url, work, pushed, pushCommand{"refs/heads/topic", pushed, next})

	if statuses["refs/heads/topic"] != "ok" {
		t.Fatalf("status = %q", statuses["refs/heads/topic"])
	}
	after, _ := s.rf.GetBranchByName(ctx, s.repo, "topic")
	if after.SHA != next {
		t.Errorf("topic = %s, want %s", after.SHA, next)
	}
}

func TestPushGuard_ANewBranchPushAgainstAnExistingBranchIsRefused(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()
	work, pushed := pushedTopic(t, s)
	transportGit(t, work, "checkout", "-b", "other", "origin/main")
	other := transportCommit(t, work, "o.txt", "o\n", "other")
	mainTip := transportGit(t, work, "rev-parse", "origin/main")

	statuses := sendCommands(t, s.url, work, mainTip, pushCommand{"refs/heads/topic", transportZeroSHA, other})

	if statuses["refs/heads/topic"] == "ok" {
		t.Errorf("a create against an existing branch was accepted")
	}
	after, _ := s.rf.GetBranchByName(ctx, s.repo, "topic")
	if after.SHA != pushed {
		t.Errorf("topic = %s, want %s", after.SHA, pushed)
	}
}

func TestPushGuard_AnUpdateOfABranchThatDoesNotExistIsRefusedAndNoBranchAppears(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()
	work, pushed := pushedTopic(t, s)
	next := transportCommit(t, work, "b.txt", "beta\n", "beta")

	statuses := sendCommands(t, s.url, work, pushed, pushCommand{"refs/heads/ghost", pushed, next})

	if statuses["refs/heads/ghost"] == "ok" {
		t.Errorf("an update of a missing branch reported ok")
	}
	if _, err := s.rf.GetBranchByName(ctx, s.repo, "topic"); err != nil {
		t.Errorf("topic lost: %v", err)
	}
	if _, err := s.rf.GetBranchByName(ctx, s.repo, "ghost"); err == nil {
		t.Errorf("a branch row was created for a command the server did not accept")
	}
}

func TestPushGuard_OneStaleRefDoesNotBlockTheOtherRefOfTheSamePush(t *testing.T) {
	s := newFileGitServer(t)
	ctx := context.Background()
	work, pushed := pushedTopic(t, s)
	topic, _ := s.rf.GetBranchByName(ctx, s.repo, "topic")
	tool, err := s.rf.WriteFile(ctx, WriteFileRequest{BranchID: topic.ID, Path: "tool.txt", Content: "tool\n"})
	if err != nil {
		t.Fatal(err)
	}
	next := transportCommit(t, work, "b.txt", "beta\n", "beta")

	statuses := sendCommands(t, s.url, work, pushed,
		pushCommand{"refs/heads/topic", pushed, next},
		pushCommand{"refs/heads/fresh", transportZeroSHA, next},
	)

	if statuses["refs/heads/topic"] == "ok" {
		t.Errorf("stale ref reported ok")
	}
	if statuses["refs/heads/fresh"] != "ok" {
		t.Errorf("fresh ref status = %q", statuses["refs/heads/fresh"])
	}
	afterTopic, _ := s.rf.GetBranchByName(ctx, s.repo, "topic")
	if afterTopic.SHA != tool.SHA {
		t.Errorf("topic = %s, want %s", afterTopic.SHA, tool.SHA)
	}
	fresh, err := s.rf.GetBranchByName(ctx, s.repo, "fresh")
	if err != nil || fresh.SHA != next {
		t.Errorf("fresh = %v, %v", fresh.SHA, err)
	}
}
