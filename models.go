// models.go holds the request/filter/graph DTOs used as [GitManager] method
// arguments and return shapes. These are not persisted entities — the
// persisted domain types (Repository, Branch, Commit, Tree, Blob, Keyword,
// etc.) live in mwanachama-backend-git/models, alongside their GORM row
// counterparts in gormstore/. See doc.go for the full package layout.
package mwanachamagit

import "github.com/aosanya/mwanachama-backend-git/models"

// ── Request / filter types ────────────────────────────────────────────────────
//
// These value types are used as arguments to [GitManager] methods.
// All fields are plain scalars; no pointers — use zero values to indicate
// omission where noted in the field comments.

// CreateRepoRequest carries the parameters for [GitManager.InitRepo].
type CreateRepoRequest struct {
	// Name is the human-readable label for the repository, used as the repo
	// key. Required.
	Name string `json:"name"`

	// Description is an optional free-text description of the repository.
	Description string `json:"description,omitempty"`

	// DefaultBranch is the name of the primary branch to create (e.g. "main").
	// Defaults to "main" when empty.
	DefaultBranch string `json:"default_branch,omitempty"`
}

// CreateBranchRequest carries the parameters for [GitManager.CreateBranch].
type CreateBranchRequest struct {
	// RepositoryID is the ID of the [models.Repository] that will own this
	// branch. Required.
	RepositoryID string `json:"repository_id"`

	// Name is the full branch name (e.g. "task/abc-001"). Required.
	Name string `json:"name"`

	// FromBranchID is the ID of the source branch from which the
	// new branch is created. When empty, the repository's default branch is used.
	FromBranchID string `json:"from_branch_id,omitempty"`

	// WorkflowRunID links the new branch to its originating WorkflowRun
	// (FEAT-20260602-001). When empty the branch carries no run context.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// CreateMergeRequestRequest carries the parameters for [GitManager.CreateMergeRequest].
type CreateMergeRequestRequest struct {
	// RepositoryID is the ID of the owning Repository. Required.
	RepositoryID string `json:"repository_id"`

	// Title is the human-readable summary of the change. Required.
	Title string `json:"title"`

	// Description is an optional longer-form explanation.
	Description string `json:"description,omitempty"`

	// SourceBranchID is the ID of the source branch. Required.
	SourceBranchID string `json:"source_branch_id"`

	// TargetBranchID is the ID of the target branch. When empty
	// the repository's default branch is used.
	TargetBranchID string `json:"target_branch_id,omitempty"`

	// AuthorName records the agent or user opening the MR. Optional.
	AuthorName string `json:"author_name,omitempty"`

	// WorkflowRunID links the MR to its originating WorkflowRun. Optional.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// MergeRequestFilter constrains the result set returned by
// [GitManager.ListMergeRequests]. All fields are optional.
type MergeRequestFilter struct {
	// RepositoryID restricts results to a single repository when set.
	RepositoryID string `json:"repository_id,omitempty"`

	// Status restricts results to MRs with the given status when set.
	// Valid values: "open", "merged", "closed", "failed".
	Status string `json:"status,omitempty"`

	// WorkflowRunID restricts results to MRs created within the given
	// orchestrated run when set. Empty disables the filter.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// BranchFilter constrains the result set returned by [GitManager.ListBranches].
// All fields are optional; zero values mean "no constraint".
type BranchFilter struct {
	// WorkflowRunID restricts results to branches created within the given
	// orchestrated run. Empty disables the filter.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// RollbackResult summarises the per-workflow-run rollback executed by
// [GitManager.RollbackByWorkflowRun]. All counters are filled even when zero so
// callers can distinguish a no-op rollback (run produced nothing in Git) from
// an actual unwind.
type RollbackResult struct {
	// WorkflowRunID is the run that was rolled back. Echoed back for clients
	// that aggregate results across services.
	WorkflowRunID string `json:"workflow_run_id"`

	// BranchesDeleted is the number of non-default Branch entities hard-deleted
	// because they were created during the run. Default branches are always
	// preserved even when they carry a matching workflow_run_id.
	BranchesDeleted int `json:"branches_deleted"`

	// MergeRequestsRolledBack is the number of MergeRequest entities whose
	// status was transitioned to "rolled_back". Includes MRs that were
	// previously open, merged, closed, or failed — the rollback is the
	// terminal-but-recoverable audit state and is idempotent on re-entry.
	MergeRequestsRolledBack int `json:"merge_requests_rolled_back"`

	// DefaultBranchesSkipped is the number of default branches that carried a
	// matching workflow_run_id but were preserved. Non-zero values are
	// informational only — callers that consume the value should log it as
	// a warning so operators can investigate why a default branch was tagged
	// to a transient run.
	DefaultBranchesSkipped int `json:"default_branches_skipped"`
}

// CreateTagRequest carries the parameters for [GitManager.CreateTag].
type CreateTagRequest struct {
	// RepositoryID is the ID of the [models.Repository] that will own this
	// tag. Required.
	RepositoryID string `json:"repository_id"`

	// Name is the human-readable tag label (e.g. "v1.0.0"). Required.
	Name string `json:"name"`

	// CommitID is the ID of the [models.Commit] this tag points to. Required.
	CommitID string `json:"commit_id"`

	// Message is the annotation message for annotated tags. Empty for
	// lightweight tags.
	Message string `json:"message,omitempty"`

	// TaggerName is the name of the person or agent creating the tag.
	TaggerName string `json:"tagger_name,omitempty"`
}

// WriteFileRequest carries the parameters for [GitManager.WriteFile].
type WriteFileRequest struct {
	// BranchID is the ID of the target [models.Branch]. Required.
	BranchID string `json:"branch_id"`

	// Path is the file path relative to the repository root (e.g.
	// "output/report.md"). Required.
	Path string `json:"path"`

	// Content is the full file content to commit.
	// Binary content must be base64-encoded and Encoding set to "base64".
	Content string `json:"content"`

	// Encoding is "utf-8" (default) or "base64" for binary content.
	Encoding string `json:"encoding,omitempty"`

	// AuthorName is the name or agent ID of the commit author.
	AuthorName string `json:"author_name,omitempty"`

	// AuthorEmail is the email address recorded in the Git commit.
	AuthorEmail string `json:"author_email,omitempty"`

	// Message is the commit message. Defaults to "Update {path}" when empty.
	Message string `json:"message,omitempty"`
}

// DeleteFileRequest carries the parameters for [GitManager.DeleteFile].
type DeleteFileRequest struct {
	// BranchID is the ID of the target [models.Branch]. Required.
	BranchID string `json:"branch_id"`

	// Path is the file path relative to the repository root. Required.
	Path string `json:"path"`

	// AuthorName is the name or agent ID recorded in the deletion commit.
	AuthorName string `json:"author_name,omitempty"`

	// AuthorEmail is the email address recorded in the deletion commit.
	AuthorEmail string `json:"author_email,omitempty"`

	// Message is the commit message. Defaults to "Delete {path}" when empty.
	Message string `json:"message,omitempty"`
}

// LogFilter constrains the result set returned by [GitManager.Log].
// All fields are optional; zero values mean "no constraint".
type LogFilter struct {
	// Path restricts the log to commits that modified the file at this path.
	// Empty means return the full branch history.
	Path string `json:"path,omitempty"`

	// Limit caps the number of commits returned. 0 means no limit.
	Limit int `json:"limit,omitempty"`
}

// ── Documentation layer types (GIT-019) ──────────────────────────────────────

// CreateKeywordRequest carries the parameters for [GitManager.CreateKeyword].
type CreateKeywordRequest struct {
	// Name is the human-readable keyword label. Required.
	Name string `json:"name"`

	// Description is an optional plain-text summary.
	Description string `json:"description,omitempty"`

	// Scope is an optional grouping label.
	Scope string `json:"scope,omitempty"`

	// ParentID is the ID of the parent Keyword. Empty for a root keyword.
	ParentID string `json:"parent_id,omitempty"`
}

// UpdateKeywordRequest carries the mutable fields for [GitManager.UpdateKeyword].
type UpdateKeywordRequest struct {
	// Name is the updated human-readable label. Required.
	Name string `json:"name"`

	// Description is the updated plain-text summary.
	Description string `json:"description,omitempty"`

	// Scope is the updated grouping label.
	Scope string `json:"scope,omitempty"`
}

// KeywordFilter constrains the result set returned by [GitManager.ListKeywords].
type KeywordFilter struct {
	// Scope filters to keywords with the given scope. Empty means all scopes.
	Scope string `json:"scope,omitempty"`

	// ParentID filters to direct children of the given keyword. Empty means
	// return all root-level keywords (no parent).
	ParentID string `json:"parent_id,omitempty"`

	// Limit caps the number of keywords returned. 0 means no limit.
	Limit int `json:"limit,omitempty"`
}

// KeywordTreeNode is a recursive node in the keyword taxonomy tree returned
// by [GitManager.GetKeywordTree].
type KeywordTreeNode struct {
	// Keyword is the keyword entity for this node.
	Keyword models.Keyword `json:"keyword"`

	// Children are the direct child nodes in the taxonomy tree.
	Children []KeywordTreeNode `json:"children,omitempty"`
}

// CreateEdgeRequest carries the parameters for [GitManager.CreateEdge].
// Edges created via this method are branch-scoped and follow the DR-010
// lifecycle (replicated to main on merge, deleted on branch delete).
type CreateEdgeRequest struct {
	// BranchID is the ID of the branch that scopes this edge.
	// The from entity must be a Blob that exists on this branch. Required.
	BranchID string `json:"branch_id"`

	// FromEntityID is the ID of the source entity. Required.
	// Must be a Blob that belongs to the specified branch.
	FromEntityID string `json:"from_entity_id"`

	// RelationshipName is the edge type to create. Must be one of:
	// "tagged_with", "references", "documents", "documented_by", "depends_on",
	// "imported_by". Required.
	RelationshipName string `json:"relationship_name"`

	// ToEntityID is the ID of the target entity. Required.
	// For "tagged_with": must be a Keyword.
	// For "references": must be a Blob; requires a "descriptor" property.
	// For other types: must be a Blob.
	ToEntityID string `json:"to_entity_id"`

	// Properties are optional metadata stored on the edge. For "references"
	// edges the "descriptor" key is required and holds the semantic label
	// (e.g. "documents", "depends_on", "contradicts").
	Properties map[string]any `json:"properties,omitempty"`
}

// DeleteEdgeRequest carries the parameters for [GitManager.DeleteEdge].
type DeleteEdgeRequest struct {
	// BranchID is the ID of the branch that scopes this edge.
	// Required.
	BranchID string `json:"branch_id"`

	// FromEntityID is the ID of the source entity. Required.
	FromEntityID string `json:"from_entity_id"`

	// RelationshipName is the edge type to delete. Required.
	RelationshipName string `json:"relationship_name"`

	// ToEntityID is the ID of the target entity. Required.
	ToEntityID string `json:"to_entity_id"`
}

// ── Graph Query types (GIT-020) ───────────────────────────────────────────────

// GraphNode is a vertex returned by [GitManager.GetNeighborhood] or
// [GitManager.SearchByKeywords]. It carries the entity ID, type, and a
// snapshot of its properties at query time.
type GraphNode struct {
	// ID is the unique identifier for this entity.
	ID string `json:"id"`

	// TypeID is the entity's type name (e.g. "Blob", "Keyword").
	TypeID string `json:"type_id"`

	// Properties holds the current state values of the entity.
	Properties map[string]any `json:"properties,omitempty"`
}

// GraphEdge is a directed edge returned in a graph query result.
type GraphEdge struct {
	// ID is a synthetic identifier for this edge ("<name>:<fromID>:<toID>")
	// — not a real storage key. FK- and join-derived edges have no row ID of
	// their own; this value exists only to dedupe edges during traversal and
	// must not be treated as a fetchable reference.
	ID string `json:"id"`

	// Name is the relationship type label (e.g. "tagged_with", "documents").
	Name string `json:"name"`

	// FromID is the ID of the source vertex.
	FromID string `json:"from_id"`

	// ToID is the ID of the target vertex.
	ToID string `json:"to_id"`
}

// GraphResult is the generic graph response returned by neighborhood and
// keyword search queries. Both Nodes and Edges are populated; the caller
// can render the full subgraph from this data.
type GraphResult struct {
	// Nodes are the vertices reachable within the query scope.
	// The starting entity is always included as the first node.
	Nodes []GraphNode `json:"nodes"`

	// Edges are the traversed relationships between Nodes.
	Edges []GraphEdge `json:"edges"`
}

// KeywordMatchMode controls how multiple keyword IDs are combined when
// searching with [GitManager.SearchByKeywords].
type KeywordMatchMode string

const (
	// KeywordMatchModeAND requires a result entity to be tagged with ALL
	// specified keywords (or their descendants when Cascade is true).
	KeywordMatchModeAND KeywordMatchMode = "AND"

	// KeywordMatchModeOR requires a result entity to be tagged with AT LEAST
	// ONE of the specified keywords (or their descendants when Cascade is true).
	KeywordMatchModeOR KeywordMatchMode = "OR"
)

// SearchByKeywordsRequest carries the parameters for [GitManager.SearchByKeywords].
type SearchByKeywordsRequest struct {
	// BranchID is the ID of the branch to search within.
	// Only entities reachable from this branch are considered.
	BranchID string `json:"branch_id"`

	// Keywords is the list of Keyword entity IDs to search for.
	// At least one keyword ID is required.
	Keywords []string `json:"keywords"`

	// MatchMode controls whether ALL (AND) or ANY (OR) keywords must match.
	// Defaults to [KeywordMatchModeOR] when zero-valued.
	MatchMode KeywordMatchMode `json:"match_mode,omitempty"`

	// Cascade when true expands each keyword to include all its descendants in
	// the taxonomy tree before matching. When false only the exact keyword IDs
	// are used.
	Cascade bool `json:"cascade,omitempty"`
}

// QueryGraphRequest carries the parameters for [GitManager.QueryGraph].
// All filter fields are optional; omitting a field disables that filter dimension.
type QueryGraphRequest struct {
	// BranchID is the ID of the branch scope.
	BranchID string `json:"branch_id"`

	// Limit is the maximum number of nodes to return. Defaults to 50.
	Limit int `json:"limit,omitempty"`

	// SortBy controls the sort order. Only "signal" is supported in v1.
	SortBy string `json:"sort_by,omitempty"`

	// Signals restricts to Blob nodes whose highest tagged_with signal is in
	// this set. An empty slice disables the signal filter.
	Signals []string `json:"signals,omitempty"`

	// KeywordIDs restricts to Blob nodes tagged with at least one of these
	// keyword entity IDs. An empty slice disables the keyword filter.
	KeywordIDs []string `json:"keyword_ids,omitempty"`

	// FileTypes restricts Blob nodes by file extension (suffix match on path).
	// Example: [".ts", ".go"]. An empty slice disables the file-type filter.
	FileTypes []string `json:"file_types,omitempty"`

	// Folders restricts Blob nodes whose path starts with any of these prefixes.
	// An empty slice disables the folder filter.
	Folders []string `json:"folders,omitempty"`

	// Relationships restricts edges to those whose label or descriptor property
	// is in this set. An empty slice returns all edges between returned nodes.
	Relationships []string `json:"relationships,omitempty"`
}

// ── Lazy Import v2 (GIT-023b) ─────────────────────────────────────────────────

// FetchBranchRequest carries the parameters for [GitManager.FetchBranch].
// It targets a single stub branch (status == "stub") within a repository and
// triggers an async on-demand fetch of its full commit history and file tree.
type FetchBranchRequest struct {
	// RepoID is the ID of the Repository that owns the branch.
	RepoID string `json:"repo_id"`

	// BranchID is the ID of the Branch to fetch.
	// The branch must currently have status == "stub".
	BranchID string `json:"branch_id"`
}

// SearchBlobsRequest carries the parameters for [GitManager.SearchBlobs].
type SearchBlobsRequest struct {
	// Query is the search term tokenised against blob name and content. Required.
	Query string `json:"query"`
	// RepositoryName scopes the search to a single repository. Required.
	RepositoryName string `json:"repository_name"`
	// BranchName optionally scopes results to blobs reachable on the named branch.
	// When empty all blobs in the repository are searched.
	BranchName string `json:"branch_name,omitempty"`
	// Limit caps the number of results. Defaults to 20 when 0.
	Limit int `json:"limit,omitempty"`
}

// BlobSearchResult is a single ranked match returned by [GitManager.SearchBlobs].
type BlobSearchResult struct {
	// ID is the identifier of the matching Blob.
	ID string `json:"id"`
	// Path is the file path relative to the repository root.
	Path string `json:"path"`
	// Name is the base file name including extension.
	Name string `json:"name"`
	// Extension is the file extension without the leading dot.
	Extension string `json:"extension,omitempty"`
	// Snippet is a short excerpt of content around the first match.
	// May be empty when the match is on the file name only.
	Snippet string `json:"snippet,omitempty"`
	// Score is the search relevance score (higher = more relevant).
	Score float64 `json:"score"`
}
