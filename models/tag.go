package models

// Tag is an immutable named ref pointing to a [Commit]. Once created, the
// target commit must never change. Lightweight tags record only a name and
// SHA; annotated tags also carry a message and tagger metadata.
type Tag struct {
	ID           string `json:"id"`
	RepositoryID string `json:"repository_id"`
	Name         string `json:"name"`

	// SHA is the commit SHA this tag resolves to.
	SHA string `json:"sha"`

	// Message is the annotation message for annotated tags; empty for
	// lightweight tags.
	Message    string `json:"message,omitempty"`
	TaggerName string `json:"tagger_name,omitempty"`
	TaggerAt   string `json:"tagger_at,omitempty"`
	CreatedAt  string `json:"created_at"`
}
