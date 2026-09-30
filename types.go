package mwanachamagit

import (
	"encoding/json"
	"fmt"
	"time"
)

type FileEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

type CommitEntry struct {
	SHA       string    `json:"sha"`
	Author    string    `json:"author"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

func (c CommitEntry) MarshalJSON() ([]byte, error) {
	type wire struct {
		SHA       string `json:"sha"`
		Author    string `json:"author"`
		Message   string `json:"message"`
		Timestamp string `json:"timestamp"`
	}
	return json.Marshal(wire{
		SHA:       c.SHA,
		Author:    c.Author,
		Message:   c.Message,
		Timestamp: c.Timestamp.Format(time.RFC3339),
	})
}

type FileDiff struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
	Patch     string `json:"patch,omitempty"`
}

type ErrMergeConflict struct {
	TaskID           string
	ConflictingFiles []string
}

func (e *ErrMergeConflict) Error() string {
	return fmt.Sprintf("merge conflict on task branch %q: conflicting files %v", e.TaskID, e.ConflictingFiles)
}

type ImportRepoRequest struct {
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	SourceURL     string `json:"source_url"`
	DefaultBranch string `json:"default_branch,omitempty"`
}
