package models

type ImportJob struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	SourceURL     string   `json:"source_url"`
	DefaultBranch string   `json:"default_branch"`
	Status        string   `json:"status"`
	ErrorMessage  string   `json:"error_message,omitempty"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
	Deleted       bool     `json:"-"`
	ProgressSteps []string `json:"progress_steps,omitempty" spec:"-"`
}

const (
	ImportStatusPending   = "pending"
	ImportStatusRunning   = "running"
	ImportStatusCompleted = "completed"
	ImportStatusFailed    = "failed"
	ImportStatusCancelled = "cancelled"
)
