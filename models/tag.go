package models

type Tag struct {
	ID           string `json:"id"`
	RepositoryID string `json:"repository_id"`
	CommitID     string `json:"-"`
	Name         string `json:"name"`
	SHA          string `json:"sha"`
	Message      string `json:"message,omitempty"`
	TaggerName   string `json:"tagger_name,omitempty"`
	TaggerAt     string `json:"tagger_at,omitempty"`
	CreatedAt    string `json:"created_at"`
	Deleted      bool   `json:"-"`
}
