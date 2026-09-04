package models

// Keyword is a hierarchical discovery label used to tag Blobs. Keywords
// form a free-form taxonomy tree via the row's ParentID self-reference.
// Querying a parent Keyword cascades to all descendant Keywords by default.
type Keyword struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Scope is an optional grouping label (e.g. "domain", "layer", "technology").
	Scope string `json:"scope,omitempty"`

	// ParentID is the ID of the parent Keyword, or empty if this is a root keyword.
	ParentID string `json:"parent_id,omitempty"`

	// ChildIDs are the IDs of direct child Keywords. Not a stored column —
	// read via a query on gormstore.KeywordRow.ParentID.
	ChildIDs []string `json:"child_ids,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
