package mwanachamagit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	BackupStatusFileName = "backup-status.json"

	repoFileExtension = ".db"
	backupTempPattern = ".backup-*.tmp"
)

type BackupFileStatus struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

type BackupStatus struct {
	LastAttemptAt time.Time          `json:"last_attempt_at"`
	LastSuccessAt time.Time          `json:"last_success_at"`
	Succeeded     bool               `json:"succeeded"`
	Error         string             `json:"error,omitempty"`
	Files         []BackupFileStatus `json:"files"`
}

func BackupRepoFile(ctx context.Context, srcPath, dstPath string) (err error) {
	if _, err := os.Stat(srcPath); err != nil {
		return fmt.Errorf("BackupRepoFile %s: %w", srcPath, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dstPath), backupTempPattern)
	if err != nil {
		return fmt.Errorf("BackupRepoFile %s: %w", srcPath, err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("BackupRepoFile %s: %w", srcPath, err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := vacuumInto(ctx, srcPath, tmpPath); err != nil {
		return fmt.Errorf("BackupRepoFile %s: %w", srcPath, err)
	}
	if err := os.Rename(tmpPath, dstPath); err != nil {
		return fmt.Errorf("BackupRepoFile %s: %w", srcPath, err)
	}
	return nil
}

func vacuumInto(ctx context.Context, srcPath, tmpPath string) (err error) {
	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(%d)", srcPath, repoFileBusyTimeoutMillis)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); err == nil {
			err = closeErr
		}
	}()
	quoted := strings.ReplaceAll(tmpPath, "'", "''")
	_, err = db.ExecContext(ctx, fmt.Sprintf("VACUUM INTO '%s'", quoted))
	return err
}

func BackupRepositoryFolder(ctx context.Context, srcDir, dstDir string, now func() time.Time) (BackupStatus, error) {
	attemptedAt := now()
	previous, err := ReadBackupStatus(dstDir)
	if err != nil {
		previous = BackupStatus{}
	}
	status := BackupStatus{LastAttemptAt: attemptedAt, Files: []BackupFileStatus{}}

	failures := backupEveryRepoFile(ctx, srcDir, dstDir, &status)
	status.Succeeded = len(failures) == 0
	if status.Succeeded {
		status.LastSuccessAt = attemptedAt
	} else {
		status.LastSuccessAt = previous.LastSuccessAt
		status.Error = joinFailureMessages(failures)
	}

	if err := writeBackupStatus(dstDir, status); err != nil {
		failures = append(failures, err)
	}
	return status, errors.Join(failures...)
}

func backupEveryRepoFile(ctx context.Context, srcDir, dstDir string, status *BackupStatus) []error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return []error{fmt.Errorf("BackupRepositoryFolder: %w", err)}
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return []error{fmt.Errorf("BackupRepositoryFolder: %w", err)}
	}
	var failures []error
	for _, entry := range entries {
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != repoFileExtension {
			continue
		}
		dstPath := filepath.Join(dstDir, entry.Name())
		if err := BackupRepoFile(ctx, filepath.Join(srcDir, entry.Name()), dstPath); err != nil {
			failures = append(failures, err)
			continue
		}
		info, err := os.Stat(dstPath)
		if err != nil {
			failures = append(failures, fmt.Errorf("BackupRepositoryFolder: %w", err))
			continue
		}
		status.Files = append(status.Files, BackupFileStatus{Name: entry.Name(), Bytes: info.Size()})
	}
	return failures
}

func joinFailureMessages(failures []error) string {
	messages := make([]string, len(failures))
	for i, failure := range failures {
		messages[i] = failure.Error()
	}
	return strings.Join(messages, "; ")
}

func ReadBackupStatus(dstDir string) (BackupStatus, error) {
	data, err := os.ReadFile(filepath.Join(dstDir, BackupStatusFileName))
	if errors.Is(err, os.ErrNotExist) {
		return BackupStatus{}, nil
	}
	if err != nil {
		return BackupStatus{}, fmt.Errorf("ReadBackupStatus: %w", err)
	}
	var status BackupStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return BackupStatus{}, fmt.Errorf("ReadBackupStatus: %w", err)
	}
	return status, nil
}

func writeBackupStatus(dstDir string, status BackupStatus) (err error) {
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("write backup status: %w", err)
	}
	tmp, err := os.CreateTemp(dstDir, backupTempPattern)
	if err != nil {
		return fmt.Errorf("write backup status: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write backup status: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write backup status: %w", err)
	}
	if err := os.Rename(tmpPath, filepath.Join(dstDir, BackupStatusFileName)); err != nil {
		return fmt.Errorf("write backup status: %w", err)
	}
	return nil
}
