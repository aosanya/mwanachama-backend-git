package models

type Commit struct {
	ID             string   `json:"id"`
	RepositoryID   string   `json:"repository_id"`
	SHA            string   `json:"sha"`
	Message        string   `json:"message"`
	AuthorName     string   `json:"author_name,omitempty"`
	AuthorEmail    string   `json:"author_email,omitempty"`
	AuthorAt       string   `json:"author_at,omitempty"`
	CommitterName  string   `json:"committer_name,omitempty"`
	CommitterEmail string   `json:"committer_email,omitempty"`
	CommittedAt    string   `json:"committed_at,omitempty"`
	TreeID         string   `json:"tree_id,omitempty"`
	Data           string   `json:"-"`
	Size           int64    `json:"-"`
	CreatedAt      string   `json:"created_at"`
	Deleted        bool     `json:"-"`
	ParentIDs      []string `json:"parent_ids,omitempty" spec:"-" gorm:"-"`
}
