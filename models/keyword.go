package models

type Keyword struct {
	ID          string   `json:"id"`
	ParentID    string   `json:"parent_id,omitempty"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	Deleted     bool     `json:"-"`
	ChildIDs    []string `json:"child_ids,omitempty" spec:"-"`
}
