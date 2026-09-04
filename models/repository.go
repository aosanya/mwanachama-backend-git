package models

// Repository is a versioned codebase owned by an [Agency]. An agency can
// have multiple repositories; each is linked to its owning agency via the
// gormstore row's AgencyID column. Sub-resources (Branches, Tags, Commits)
// are separate rows linked by their own RepositoryID foreign key.
type Repository struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	DefaultBranch string `json:"default_branch"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`

	// SourceURL is the remote Git URL the repository was imported from.
	// Empty for repositories created locally via InitRepo.
	SourceURL string `json:"source_url,omitempty"`
}
