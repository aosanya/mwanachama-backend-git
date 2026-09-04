package models

// Tree is an immutable git tree entity. A tree represents a directory
// listing at a specific point in time. The root tree of a commit is linked
// via the owning [Commit] row's TreeID column; nested subdirectory trees are
// linked via the gormstore.TreeSubtreeRow join table.
type Tree struct {
	ID string `json:"id"`
	// SHA is the full 40-character hex Git tree hash.
	SHA string `json:"sha"`

	// Path is the directory path within the commit tree hierarchy.
	// An empty string ("") denotes the root tree of a commit.
	Path string `json:"path,omitempty"`

	// CommitID is the ID of the owning Commit. Not a stored column on this
	// side — it is the inverse of Commit.TreeID, resolved by query only when
	// a caller actually needs it (nothing does today).
	CommitID string `json:"commit_id,omitempty"`

	// BlobIDs are the IDs of direct [Blob] children. Not a stored column —
	// read from gormstore.TreeBlobRow, written via
	// gormstore.TreeBlobsToRows alongside the Tree row itself.
	BlobIDs []string `json:"blob_ids,omitempty"`

	// SubtreeIDs are the IDs of nested [Tree] children. Not a stored column
	// — read from gormstore.TreeSubtreeRow, written via
	// gormstore.TreeSubtreesToRows alongside the Tree row itself.
	SubtreeIDs []string `json:"subtree_ids,omitempty"`

	CreatedAt string `json:"created_at"`
}
