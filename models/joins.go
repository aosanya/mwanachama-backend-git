package models

type CommitParent struct {
	CommitID    string `json:"commit_id"`
	ParentID    string `json:"parent_id"`
	ParentIndex int    `json:"parent_index"`
}

type TreeBlob struct {
	TreeID string `json:"tree_id"`
	BlobID string `json:"blob_id"`
}

type TreeSubtree struct {
	TreeID    string `json:"tree_id"`
	SubtreeID string `json:"subtree_id"`
}

type BlobKeywordTag struct {
	BranchID  string `json:"branch_id"`
	BlobID    string `json:"blob_id"`
	KeywordID string `json:"keyword_id"`
	Signal    string `json:"signal,omitempty"`
	Note      string `json:"note,omitempty"`
	CreatedAt string `json:"created_at"`
}

type BlobReference struct {
	BranchID   string `json:"branch_id"`
	FromBlobID string `json:"from_blob_id"`
	Name       string `json:"name"`
	ToBlobID   string `json:"to_blob_id"`
	Descriptor string `json:"descriptor,omitempty"`
	CreatedAt  string `json:"created_at"`
}

const (
	EdgeTaggedWith   = "tagged_with"
	EdgeReferences   = "references"
	EdgeReferencedBy = "referenced_by"
	EdgeDocuments    = "documents"
	EdgeDocumentedBy = "documented_by"
	EdgeDependsOn    = "depends_on"
	EdgeImportedBy   = "imported_by"
)
