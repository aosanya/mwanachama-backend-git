package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	git "github.com/aosanya/mwanachama-backend-git"
)

var version = "dev"

func main() {
	repoFolder := os.Getenv("REPO_FOLDER")
	backupFolder := os.Getenv("BACKUP_FOLDER")
	if repoFolder == "" || backupFolder == "" {
		fmt.Fprintf(os.Stderr, "repobackup %s: REPO_FOLDER and BACKUP_FOLDER are required\n", version)
		os.Exit(2)
	}

	status, err := git.BackupRepositoryFolder(
		context.Background(),
		repoFolder,
		backupFolder,
		func() time.Time { return time.Now().UTC() },
	)

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if encodeErr := encoder.Encode(status); encodeErr != nil {
		fmt.Fprintln(os.Stderr, encodeErr)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
