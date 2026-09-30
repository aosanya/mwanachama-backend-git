package models

type Blob struct {
	ID        string `json:"id"`
	SHA       string `json:"sha"`
	Path      string `json:"path"`
	Name      string `json:"name,omitempty"`
	Extension string `json:"extension,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Encoding  string `json:"encoding,omitempty"`
	Content   string `json:"content,omitempty"`
	Data      string `json:"-"`
	CreatedAt string `json:"created_at"`
	Deleted   bool   `json:"-"`
	TreeID    string `json:"tree_id,omitempty" spec:"-" gorm:"-"`
}
