package models

type Tree struct {
	ID         string   `json:"id"`
	SHA        string   `json:"sha"`
	Path       string   `json:"path,omitempty"`
	Entries    string   `json:"-"`
	Data       string   `json:"-"`
	Size       int64    `json:"-"`
	CreatedAt  string   `json:"created_at"`
	Deleted    bool     `json:"-"`
	CommitID   string   `json:"commit_id,omitempty" spec:"-"`
	BlobIDs    []string `json:"blob_ids,omitempty" spec:"-"`
	SubtreeIDs []string `json:"subtree_ids,omitempty" spec:"-"`
}
