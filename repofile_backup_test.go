package mwanachamagit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"
)

const (
	backupLivePath          = "live.txt"
	backupSeedCommits       = 40
	backupSeedPayloadRepeat = 200
)

type backupSeededRepo struct {
	fileName string
	branchID string
	messages []string
}

func backupWriteRevision(ctx context.Context, rf *RepoFile, branchID, label string, revision int) (string, error) {
	text := fmt.Sprintf("%s revision %d", label, revision)
	_, err := rf.WriteFile(ctx, WriteFileRequest{
		BranchID: branchID,
		Path:     backupLivePath,
		Content:  text,
		Message:  text,
	})
	return text, err
}

func backupSeedRepo(t *testing.T, dir, fileName, label string, commits int) backupSeededRepo {
	t.Helper()
	ctx := context.Background()
	rf := openRepoFileForTest(t, filepath.Join(dir, fileName))
	defer closeRepoFileForTest(t, rf)
	repo, err := rf.InitRepo(ctx, CreateRepoRequest{Name: label})
	if err != nil {
		t.Fatalf("InitRepo %s: %v", label, err)
	}
	branch := historyDefaultBranch(t, rf, repo.ID)
	seeded := backupSeededRepo{fileName: fileName, branchID: branch.ID}
	for revision := 1; revision <= commits; revision++ {
		text, err := backupWriteRevision(ctx, rf, branch.ID, label, revision)
		if err != nil {
			t.Fatalf("WriteFile %s revision %d: %v", label, revision, err)
		}
		seeded.messages = append(seeded.messages, text)
	}
	return seeded
}

func backupSeedBulkyRepo(t *testing.T, dir, fileName string) backupSeededRepo {
	t.Helper()
	ctx := context.Background()
	rf := openRepoFileForTest(t, filepath.Join(dir, fileName))
	defer closeRepoFileForTest(t, rf)
	repo, err := rf.InitRepo(ctx, CreateRepoRequest{Name: "bulky"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branch := historyDefaultBranch(t, rf, repo.ID)
	seeded := backupSeededRepo{fileName: fileName, branchID: branch.ID}
	for revision := 1; revision <= backupSeedCommits; revision++ {
		text := fmt.Sprintf("seed revision %d", revision)
		payload := ""
		for i := 0; i < backupSeedPayloadRepeat; i++ {
			payload += fmt.Sprintf("seed %d line %d of a longer body\n", revision, i)
		}
		if _, err := rf.WriteFile(ctx, WriteFileRequest{BranchID: branch.ID, Path: fmt.Sprintf("body-%d.txt", revision), Content: payload, Message: text}); err != nil {
			t.Fatalf("WriteFile body %d: %v", revision, err)
		}
		if _, err := backupWriteRevision(ctx, rf, branch.ID, "seed", revision); err != nil {
			t.Fatalf("WriteFile live %d: %v", revision, err)
		}
	}
	return seeded
}

func backupLogMessages(t *testing.T, rf *RepoFile, branchID string) []string {
	t.Helper()
	entries := historyLog(t, rf, branchID, LogFilter{})
	messages := make([]string, len(entries))
	for i, e := range entries {
		messages[i] = e.Message
	}
	return messages
}

func backupIntegrityCheck(t *testing.T, rf *RepoFile) string {
	t.Helper()
	var result string
	if err := rf.sql.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		t.Fatalf("PRAGMA integrity_check: %v", err)
	}
	return result
}

func backupDirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func backupFileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Size()
}

func backupStatusFiles(status BackupStatus) map[string]int64 {
	files := make(map[string]int64, len(status.Files))
	for _, f := range status.Files {
		files[f.Name] = f.Bytes
	}
	return files
}

func backupAssertSameHistory(t *testing.T, label string, rf *RepoFile, branchID string, want []string) {
	t.Helper()
	got := backupLogMessages(t, rf, branchID)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: history = %d commits %v, want %d commits %v", label, len(got), got, len(want), want)
	}
	blob, err := rf.ReadFile(context.Background(), branchID, backupLivePath)
	if err != nil {
		t.Fatalf("%s: ReadFile %s: %v", label, backupLivePath, err)
	}
	if blob.Content != want[0] {
		t.Fatalf("%s: head file content = %q, want the head commit message %q", label, blob.Content, want[0])
	}
}

func backupNewestFirst(messages []string) []string {
	out := make([]string, len(messages))
	for i, m := range messages {
		out[len(messages)-1-i] = m
	}
	return out
}

func TestRepoFileBackup_FolderBackupHoldsTheRepositoryHistoryUpToTheCopy(t *testing.T) {
	ctx := context.Background()
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	seeded := backupSeedRepo(t, srcDir, "widgets.db", "widgets", 6)

	status, err := BackupRepositoryFolder(ctx, srcDir, dstDir, time.Now)
	if err != nil {
		t.Fatalf("BackupRepositoryFolder: %v", err)
	}
	if !status.Succeeded {
		t.Fatalf("status = %+v, want Succeeded", status)
	}

	copied := openRepoFileForTest(t, filepath.Join(dstDir, seeded.fileName))
	defer closeRepoFileForTest(t, copied)
	repos, err := copied.ListRepositories(ctx)
	if err != nil || len(repos) != 1 || repos[0].Name != "widgets" {
		t.Fatalf("copy repositories = %+v (err=%v), want only widgets", repos, err)
	}
	backupAssertSameHistory(t, "folder copy", copied, seeded.branchID, backupNewestFirst(seeded.messages))
	if got := backupIntegrityCheck(t, copied); got != "ok" {
		t.Fatalf("integrity_check on the copy = %q, want ok", got)
	}
}

func TestRepoFileBackup_FolderBackupCopiesEveryRepositoryFileInTheFolder(t *testing.T) {
	ctx := context.Background()
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	alpha := backupSeedRepo(t, srcDir, "alpha.db", "alpha", 3)
	beta := backupSeedRepo(t, srcDir, "beta.db", "beta", 5)

	if _, err := BackupRepositoryFolder(ctx, srcDir, dstDir, time.Now); err != nil {
		t.Fatalf("BackupRepositoryFolder: %v", err)
	}

	for _, seeded := range []backupSeededRepo{alpha, beta} {
		copied := openRepoFileForTest(t, filepath.Join(dstDir, seeded.fileName))
		backupAssertSameHistory(t, seeded.fileName, copied, seeded.branchID, backupNewestFirst(seeded.messages))
		closeRepoFileForTest(t, copied)
	}
}

func TestRepoFileBackup_CopyTakenWhileCommitsContinueIsWholeAndNotTorn(t *testing.T) {
	ctx := context.Background()
	srcDir := t.TempDir()
	dstPath := filepath.Join(t.TempDir(), "bulky.db")
	seeded := backupSeedBulkyRepo(t, srcDir, "bulky.db")
	srcPath := filepath.Join(srcDir, seeded.fileName)

	writer := openRepoFileForTest(t, srcPath)
	defer closeRepoFileForTest(t, writer)

	firstCommitsDone := make(chan struct{})
	stop := make(chan struct{})
	var writerDone sync.WaitGroup
	writerDone.Add(1)
	go func() {
		defer writerDone.Done()
		signalled := false
		for revision := 1; ; revision++ {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := backupWriteRevision(ctx, writer, seeded.branchID, "live", revision); err != nil {
				t.Errorf("writer revision %d: %v", revision, err)
				if !signalled {
					close(firstCommitsDone)
				}
				return
			}
			if revision == 3 && !signalled {
				signalled = true
				close(firstCommitsDone)
			}
		}
	}()

	select {
	case <-firstCommitsDone:
	case <-time.After(time.Minute):
		close(stop)
		writerDone.Wait()
		t.Fatalf("writer made no commits before the backup")
	}

	beforeCopy := backupLogMessages(t, writer, seeded.branchID)
	copyErr := BackupRepoFile(ctx, srcPath, dstPath)
	close(stop)
	writerDone.Wait()
	if copyErr != nil {
		t.Fatalf("BackupRepoFile: %v", copyErr)
	}
	afterCopy := backupLogMessages(t, writer, seeded.branchID)
	if len(afterCopy) < len(beforeCopy) {
		t.Fatalf("source history shrank from %d to %d commits", len(beforeCopy), len(afterCopy))
	}

	copied := openRepoFileForTest(t, dstPath)
	defer closeRepoFileForTest(t, copied)
	if got := backupIntegrityCheck(t, copied); got != "ok" {
		t.Fatalf("integrity_check on the copy = %q, want ok", got)
	}

	copiedLog := backupLogMessages(t, copied, seeded.branchID)
	if len(copiedLog) < len(beforeCopy) {
		t.Fatalf("copy holds %d commits, want at least the %d made before the backup started", len(copiedLog), len(beforeCopy))
	}
	if len(copiedLog) > len(afterCopy) {
		t.Fatalf("copy holds %d commits, more than the %d the source ended with", len(copiedLog), len(afterCopy))
	}
	sourceRun := afterCopy[len(afterCopy)-len(copiedLog):]
	if !reflect.DeepEqual(copiedLog, sourceRun) {
		t.Fatalf("copy history is not a complete run of the source history:\ncopy   %v\nsource %v", copiedLog, sourceRun)
	}
	if oldest := copiedLog[len(copiedLog)-1]; oldest != "seed revision 1" {
		t.Fatalf("oldest commit in the copy = %q, want the first seed commit", oldest)
	}
	blob, err := copied.ReadFile(ctx, seeded.branchID, backupLivePath)
	if err != nil {
		t.Fatalf("ReadFile %s on the copy: %v", backupLivePath, err)
	}
	if blob.Content != copiedLog[0] {
		t.Fatalf("head file content = %q, want the head commit message %q", blob.Content, copiedLog[0])
	}
	for revision := 1; revision <= backupSeedCommits; revision++ {
		body, err := copied.ReadFile(ctx, seeded.branchID, fmt.Sprintf("body-%d.txt", revision))
		if err != nil {
			t.Fatalf("ReadFile body-%d.txt on the copy: %v", revision, err)
		}
		if body.Size == 0 || body.Content == "" {
			t.Fatalf("body-%d.txt on the copy is empty", revision)
		}
	}
	t.Logf("copy holds %d commits; %d before the copy started, %d in the source after", len(copiedLog), len(beforeCopy), len(afterCopy))
}

func TestRepoFileBackup_BackupStatusRecordsASuccessfulRun(t *testing.T) {
	ctx := context.Background()
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	backupSeedRepo(t, srcDir, "alpha.db", "alpha", 3)
	backupSeedRepo(t, srcDir, "beta.db", "beta", 4)
	ranAt := time.Date(2026, 11, 14, 2, 0, 0, 0, time.UTC)

	status, err := BackupRepositoryFolder(ctx, srcDir, dstDir, func() time.Time { return ranAt })
	if err != nil {
		t.Fatalf("BackupRepositoryFolder: %v", err)
	}

	if !status.Succeeded || status.Error != "" {
		t.Fatalf("returned status = %+v, want Succeeded with no Error", status)
	}
	if !status.LastAttemptAt.Equal(ranAt) || !status.LastSuccessAt.Equal(ranAt) {
		t.Fatalf("returned times = attempt %v success %v, want both %v", status.LastAttemptAt, status.LastSuccessAt, ranAt)
	}

	read, err := ReadBackupStatus(dstDir)
	if err != nil {
		t.Fatalf("ReadBackupStatus: %v", err)
	}
	if !read.Succeeded || read.Error != "" {
		t.Fatalf("read status = %+v, want Succeeded with no Error", read)
	}
	if !read.LastAttemptAt.Equal(ranAt) || !read.LastSuccessAt.Equal(ranAt) {
		t.Fatalf("read times = attempt %v success %v, want both %v", read.LastAttemptAt, read.LastSuccessAt, ranAt)
	}
	wantFiles := map[string]int64{
		"alpha.db": backupFileSize(t, filepath.Join(dstDir, "alpha.db")),
		"beta.db":  backupFileSize(t, filepath.Join(dstDir, "beta.db")),
	}
	if got := backupStatusFiles(read); !reflect.DeepEqual(got, wantFiles) {
		t.Fatalf("status files = %v, want each copied .db with the bytes of its copy %v", got, wantFiles)
	}
	if _, err := os.Stat(filepath.Join(dstDir, BackupStatusFileName)); err != nil {
		t.Fatalf("status file %s missing from the backup folder: %v", BackupStatusFileName, err)
	}
}

func TestRepoFileBackup_ReadBackupStatusOfAFolderNeverBackedUpIsZero(t *testing.T) {
	status, err := ReadBackupStatus(t.TempDir())
	if err != nil {
		t.Fatalf("ReadBackupStatus: %v", err)
	}
	if status.Succeeded || status.Error != "" || !status.LastAttemptAt.IsZero() || !status.LastSuccessAt.IsZero() || len(status.Files) != 0 {
		t.Fatalf("status = %+v, want the zero status", status)
	}
}

func TestRepoFileBackup_FailedRunKeepsLastSuccessAndStillCopiesTheGoodFiles(t *testing.T) {
	ctx := context.Background()
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	good := backupSeedRepo(t, srcDir, "good.db", "good", 3)
	firstRun := time.Date(2026, 11, 14, 2, 0, 0, 0, time.UTC)
	secondRun := firstRun.Add(24 * time.Hour)

	if _, err := BackupRepositoryFolder(ctx, srcDir, dstDir, func() time.Time { return firstRun }); err != nil {
		t.Fatalf("first BackupRepositoryFolder: %v", err)
	}

	rf := openRepoFileForTest(t, filepath.Join(srcDir, good.fileName))
	newest, err := backupWriteRevision(ctx, rf, good.branchID, "good", 4)
	if err != nil {
		t.Fatalf("WriteFile after the first run: %v", err)
	}
	closeRepoFileForTest(t, rf)
	if err := os.WriteFile(filepath.Join(srcDir, "broken.db"), []byte("this is not a sqlite database, not even close"), 0o644); err != nil {
		t.Fatalf("write broken.db: %v", err)
	}

	status, err := BackupRepositoryFolder(ctx, srcDir, dstDir, func() time.Time { return secondRun })
	if err == nil {
		t.Fatalf("BackupRepositoryFolder with a corrupt file returned no error")
	}
	if status.Succeeded || status.Error == "" {
		t.Fatalf("returned status = %+v, want Succeeded false and an Error", status)
	}

	read, readErr := ReadBackupStatus(dstDir)
	if readErr != nil {
		t.Fatalf("ReadBackupStatus: %v", readErr)
	}
	if read.Succeeded || read.Error == "" {
		t.Fatalf("read status = %+v, want Succeeded false and an Error", read)
	}
	if !read.LastAttemptAt.Equal(secondRun) {
		t.Fatalf("LastAttemptAt = %v, want %v", read.LastAttemptAt, secondRun)
	}
	if !read.LastSuccessAt.Equal(firstRun) {
		t.Fatalf("LastSuccessAt = %v, want the earlier success %v", read.LastSuccessAt, firstRun)
	}

	copied := openRepoFileForTest(t, filepath.Join(dstDir, good.fileName))
	defer closeRepoFileForTest(t, copied)
	want := backupNewestFirst(append(append([]string{}, good.messages...), newest))
	backupAssertSameHistory(t, "good copy after the failed run", copied, good.branchID, want)
	if _, err := os.Stat(filepath.Join(dstDir, "broken.db")); !os.IsNotExist(err) {
		t.Fatalf("broken.db was left in the backup folder (stat err=%v)", err)
	}
}

func TestRepoFileBackup_FilesWithoutTheDbExtensionAreNotCopied(t *testing.T) {
	ctx := context.Background()
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	backupSeedRepo(t, srcDir, "widgets.db", "widgets", 2)
	for name, body := range map[string]string{
		"notes.txt":         "operator notes",
		"widgets.db-shm":    "scratch",
		"widgets.db.bak":    "older copy",
		"widgets.db-backup": "another",
	} {
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := os.Mkdir(filepath.Join(srcDir, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	backupSeedRepo(t, filepath.Join(srcDir, "nested"), "deep.db", "deep", 1)

	status, err := BackupRepositoryFolder(ctx, srcDir, dstDir, time.Now)
	if err != nil {
		t.Fatalf("BackupRepositoryFolder: %v", err)
	}

	want := []string{BackupStatusFileName, "widgets.db"}
	sort.Strings(want)
	if got := backupDirNames(t, dstDir); !reflect.DeepEqual(got, want) {
		t.Fatalf("backup folder holds %v, want only %v", got, want)
	}
	if got := backupStatusFiles(status); len(got) != 1 || got["widgets.db"] == 0 {
		t.Fatalf("status files = %v, want only widgets.db", got)
	}
}
