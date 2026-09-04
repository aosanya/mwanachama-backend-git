package gormstore

// BlobKeywordTagRow is the GORM row for one branch-scoped "tagged_with"
// edge (Blob -> Keyword), replacing the old entitygraph relationship of the
// same name. The composite primary key gives (branch, blob, keyword)
// uniqueness for free.
type BlobKeywordTagRow struct {
	BranchID  string `gorm:"primaryKey"`
	BlobID    string `gorm:"primaryKey;index"`
	KeywordID string `gorm:"primaryKey;index"`
	// Signal is the depth at which this Blob covers the Keyword. Well-known
	// values: "surface", "index", "structural", "contributor", "authority".
	Signal    string
	Note      string
	CreatedAt string
}

// BlobReferenceRow is the GORM row for one branch-scoped generic Blob->Blob
// edge, replacing the old "references"/"referenced_by" entitygraph
// relationships. Name is a constrained label (see the root package's
// validDocEdges) — this is a fixed-shape, both-ends-typed table, not a
// generic polymorphic edge table: it cannot express e.g. Commit->Keyword.
// Both directions are stored as separate rows (matching what entitygraph
// did — CreateRelationship auto-created the inverse), so a lookup from
// either endpoint is a plain indexed query.
type BlobReferenceRow struct {
	BranchID   string `gorm:"primaryKey"`
	FromBlobID string `gorm:"primaryKey;index"`
	Name       string `gorm:"primaryKey"`
	ToBlobID   string `gorm:"primaryKey;index"`
	Descriptor string
	CreatedAt  string
}
