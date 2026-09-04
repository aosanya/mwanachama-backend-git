package models

// FetchBranchJob represents the state of an async on-demand branch fetch
// operation triggered by GitManager.FetchBranch.
//
// Status transitions: "pending" -> "running" -> "completed" | "failed".
// Call GitManager.GetFetchBranchStatus to poll for progress.
type FetchBranchJob struct {
	ID         string `json:"id"`
	RepoID     string `json:"repo_id"`
	BranchName string `json:"branch_name"`

	// Status is one of: "pending", "running", "completed", "failed".
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
