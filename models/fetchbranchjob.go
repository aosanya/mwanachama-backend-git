package models

type FetchBranchJob struct {
	ID           string `json:"id"`
	RepoID       string `json:"repo_id"`
	BranchName   string `json:"branch_name"`
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	Deleted      bool   `json:"-"`
}

const (
	FetchJobStatusPending   = "pending"
	FetchJobStatusRunning   = "running"
	FetchJobStatusCompleted = "completed"
	FetchJobStatusFailed    = "failed"
)
