package models

type MergeRequest struct {
	ID               string `json:"id"`
	RepositoryID     string `json:"repository_id"`
	Title            string `json:"title"`
	Description      string `json:"description,omitempty"`
	SourceBranchID   string `json:"source_branch_id"`
	SourceBranchName string `json:"source_branch_name,omitempty"`
	TargetBranchID   string `json:"target_branch_id,omitempty"`
	TargetBranchName string `json:"target_branch_name,omitempty"`
	Status           string `json:"status"`
	MergedCommitSHA  string `json:"merged_commit_sha,omitempty"`
	AuthorName       string `json:"author_name,omitempty"`
	ErrorMessage     string `json:"error_message,omitempty"`
	WorkflowRunID    string `json:"workflow_run_id,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
	Deleted          bool   `json:"-"`
}

const (
	MergeRequestStatusOpen       = "open"
	MergeRequestStatusMerged     = "merged"
	MergeRequestStatusClosed     = "closed"
	MergeRequestStatusFailed     = "failed"
	MergeRequestStatusRolledBack = "rolled_back"
)
