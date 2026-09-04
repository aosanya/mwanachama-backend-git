package models

// Branch is a named ref pointing to a [Commit]. The target commit is a real
// foreign key (gormstore's BranchRow.HeadCommitID), updated atomically on
// each push or merge. Branches are mutable; the task-branch workflow creates
// one Branch per task and deletes it after the task branch is merged.
type Branch struct {
	ID           string `json:"id"`
	RepositoryID string `json:"repository_id"`
	Name         string `json:"name"`
	IsDefault    bool   `json:"is_default,omitempty"`
	HeadCommitID string `json:"head_commit_id,omitempty"`

	// SHA is the git commit hash at the branch tip, stored directly on the
	// row so callers can read the tip SHA without an extra lookup.
	SHA string `json:"sha,omitempty"`

	// WorkflowRunID links this branch to its originating WorkflowRun
	// (FEAT-20260602-001). Empty for branches created outside any
	// orchestrated run — e.g. the default branch created by InitRepo or
	// imports.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
