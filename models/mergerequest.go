package models

// MergeRequest captures a request to merge a source branch into a target
// branch. Status transitions: "open" -> "merged" | "closed" | "failed".
// WorkflowRunID links the MR back to its originating WorkflowRun
// (FEAT-20260602-001) so rollback and the closure view can enumerate every
// MR a single run produced.
type MergeRequest struct {
	ID           string `json:"id"`
	RepositoryID string `json:"repository_id"`

	// Title is the human-readable summary of the change. Required at create time.
	Title string `json:"title"`

	Description string `json:"description,omitempty"`

	SourceBranchID   string `json:"source_branch_id"`
	SourceBranchName string `json:"source_branch_name,omitempty"`

	// TargetBranchID defaults to the repository's default branch when empty
	// at create time; once persisted it always names a real branch.
	TargetBranchID   string `json:"target_branch_id,omitempty"`
	TargetBranchName string `json:"target_branch_name,omitempty"`

	// Status is one of: "open", "merged", "closed", "failed", "rolled_back".
	Status string `json:"status"`

	// MergedCommitSHA is populated when Status transitions to "merged" and
	// holds the SHA of the resulting target-branch HEAD commit.
	MergedCommitSHA string `json:"merged_commit_sha,omitempty"`

	AuthorName string `json:"author_name,omitempty"`

	// ErrorMessage is populated only when Status == "failed".
	ErrorMessage string `json:"error_message,omitempty"`

	// WorkflowRunID links this MR to its originating WorkflowRun
	// (FEAT-20260602-001). Empty for manually opened MRs.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// MergeRequestStatus values for [MergeRequest.Status].
const (
	MergeRequestStatusOpen       = "open"
	MergeRequestStatusMerged     = "merged"
	MergeRequestStatusClosed     = "closed"
	MergeRequestStatusFailed     = "failed"
	MergeRequestStatusRolledBack = "rolled_back"
)
