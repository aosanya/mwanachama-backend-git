package models

type Repository struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	DefaultBranch string `json:"default_branch"`
	BareClonePath string `json:"-"`
	SourceURL     string `json:"source_url,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	Deleted       bool   `json:"-"`
}
