package models

// ImportJob represents the state of an async repository import operation.
// Call GitManager.GetImportStatus to poll for progress.
type ImportJob struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	SourceURL     string `json:"source_url"`
	DefaultBranch string `json:"default_branch"`

	// Status is one of: "pending", "running", "completed", "failed", "cancelled".
	Status string `json:"status"`
	// ErrorMessage is populated when Status == "failed".
	ErrorMessage string `json:"error_message,omitempty"`

	// ProgressSteps is an ordered list of human-readable progress messages
	// appended as the import goroutine executes. Not a stored column — kept
	// in an in-process map only, attached by GetImportStatus while the
	// goroutine is active.
	ProgressSteps []string `json:"progress_steps,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
