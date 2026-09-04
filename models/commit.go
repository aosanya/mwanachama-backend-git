package models

// Commit is an immutable git commit entity. It is content-addressed by
// [Commit.SHA] and never mutated after creation. The root [Tree] is linked
// via the row's TreeID column; parent commits are linked via the
// gormstore.CommitParentRow join table (0 parents for the initial commit, 1
// for a normal commit, 2+ for merge commits).
type Commit struct {
	ID           string `json:"id"`
	RepositoryID string `json:"repository_id"`

	// SHA is the full 40-character hex Git commit hash.
	SHA     string `json:"sha"`
	Message string `json:"message"`

	AuthorName  string `json:"author_name,omitempty"`
	AuthorEmail string `json:"author_email,omitempty"`
	// AuthorAt is the ISO 8601 author timestamp.
	AuthorAt string `json:"author_at,omitempty"`

	CommitterName  string `json:"committer_name,omitempty"`
	CommitterEmail string `json:"committer_email,omitempty"`
	// CommittedAt is the ISO 8601 committer timestamp.
	CommittedAt string `json:"committed_at,omitempty"`

	// TreeID is the ID of the root Tree, read from the row's TreeID column.
	TreeID string `json:"tree_id,omitempty"`

	// ParentIDs are the IDs of parent Commits. Not a stored column — read
	// from gormstore.CommitParentRow ordered by ParentIndex, and written via
	// gormstore.CommitParentsToRows alongside the Commit row itself.
	ParentIDs []string `json:"parent_ids,omitempty"`

	CreatedAt string `json:"created_at"`
}
