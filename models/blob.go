package models

// Blob is an immutable git blob entity. Blobs are content-addressed by
// [Blob.SHA] and represent individual file contents. Text file content is
// stored as-is; binary file content is base64-encoded and [Blob.Encoding] is
// set to "base64".
type Blob struct {
	ID  string `json:"id"`
	SHA string `json:"sha"`

	// Path is the file path relative to the repository root,
	// e.g. "src/handlers/server.go".
	Path string `json:"path"`
	// Name is the base file name including extension, e.g. "Test.txt".
	Name string `json:"name,omitempty"`
	// Extension is the file extension without the leading dot, e.g. "txt".
	Extension string `json:"extension,omitempty"`
	Size      int64  `json:"size,omitempty"`
	// Encoding is "utf-8" for text files or "base64" for binary files.
	Encoding string `json:"encoding,omitempty"`
	// Content holds the raw file content. Base64-encoded when Encoding == "base64".
	Content string `json:"content,omitempty"`

	// TreeID is carried on the domain type for JSON-contract compatibility
	// only. It is not a column on gormstore.BlobRow and is never populated:
	// a blob can be reachable from many trees (content dedup by SHA), so
	// tree membership is a many-to-many gormstore.TreeBlobRow join, not a
	// single owning tree — unchanged from the entitygraph-era behavior,
	// where this field was likewise declared but never set.
	TreeID string `json:"tree_id,omitempty"`

	CreatedAt string `json:"created_at"`
}
