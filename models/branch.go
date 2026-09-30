package models

type Branch struct {
	ID            string `json:"id"`
	RepositoryID  string `json:"repository_id"`
	Name          string `json:"name"`
	IsDefault     bool   `json:"is_default,omitempty"`
	HeadCommitID  string `json:"head_commit_id,omitempty"`
	SHA           string `json:"sha,omitempty"`
	Status        string `json:"-"`
	SourceURL     string `json:"-"`
	ErrorMessage  string `json:"-"`
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	Deleted       bool   `json:"-"`
}

const (
	BranchStatusStub        = "stub"
	BranchStatusFetching    = "fetching"
	BranchStatusFetched     = "fetched"
	BranchStatusFetchFailed = "fetch_failed"
)
