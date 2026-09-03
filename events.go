package mwanachamagit

// topicPrefix is the domain prefix for every topic mwanachama-backend-git publishes.
// Mirrors CodeValdGit's eventbus.DomainGit ("git.") — kept local since this
// repo has no cross-service domain-prefix registry to import.
const topicPrefix = "git."

// Event topic constants — the closed set mwanachama-backend-git publishes via the
// [events.Publisher] injected into the concrete GitManager implementation.
//
// Dropped from the CodeValdGit original: TopicBranchCreate and
// TopicFileWrite, plus their ConsumedTopics() list and BranchCreatePayload /
// FileWritePayload / FileWriteKeyword payload types. Those existed so
// CodeValdGit could subscribe to a CodeValdCortex pub/sub bus and act on
// inbound LLM-emitted actions; mwanachama-backend-api-gateway calls GitManager
// methods directly from its own HTTP handlers, so there is no bus to
// subscribe to and no inbound payload shape to define.
const (
	// TopicRepoCreated fires after a Repository entity is created by InitRepo.
	// Payload: [RepoCreatedPayload].
	TopicRepoCreated = topicPrefix + "repo.created"

	// TopicRepoImported fires when an async ImportRepo job completes successfully.
	// Payload: [RepoImportedPayload].
	TopicRepoImported = topicPrefix + "repo.imported"

	// TopicRepoImportFailed fires when an async ImportRepo job fails.
	// Payload: [RepoImportFailedPayload].
	TopicRepoImportFailed = topicPrefix + "repo.import.failed"

	// TopicRepoImportCancelled fires when an async ImportRepo job is cancelled.
	// Payload: [RepoImportCancelledPayload].
	TopicRepoImportCancelled = topicPrefix + "repo.import.cancelled"

	// TopicBranchFetched fires when an async FetchBranch job completes successfully.
	// Payload: [BranchFetchedPayload].
	TopicBranchFetched = topicPrefix + "branch.fetched"

	// TopicBranchMerged fires after a branch is successfully merged into the
	// repository default branch. Payload: [BranchMergedPayload].
	TopicBranchMerged = topicPrefix + "branch.merged"

	// TopicMergeConflict fires when MergeBranch encounters a conflict that
	// cannot be auto-resolved. Payload: [MergeConflictPayload].
	TopicMergeConflict = topicPrefix + "conflict.detected"

	// TopicMergeRequested fires when a new [MergeRequest] is opened.
	// Payload: [MergeRequestRequestedPayload].
	TopicMergeRequested = topicPrefix + "merge.requested"

	// TopicMergeCompleted fires when a [MergeRequest] is successfully merged
	// into its target branch. Payload: [MergeRequestCompletedPayload].
	TopicMergeCompleted = topicPrefix + "merge.completed"

	// TopicMergeFailed fires when a [MergeRequest] terminates in the failed
	// state. Payload: [MergeRequestFailedPayload].
	TopicMergeFailed = topicPrefix + "merge.failed"

	// TopicFileWritten fires after a successful [GitManager.WriteFile].
	// Payload: [FileWrittenPayload].
	TopicFileWritten = topicPrefix + "file.written"

	// TopicMergeRolledBack fires for each [MergeRequest] whose status is flipped
	// to "rolled_back" by [GitManager.RollbackByWorkflowRun].
	// Payload: [MergeRequestRolledBackPayload].
	TopicMergeRolledBack = topicPrefix + "merge.rolled_back"

	// TopicWorkflowRunRolledBack fires once per
	// [GitManager.RollbackByWorkflowRun] call carrying the aggregate counters.
	// Payload: [WorkflowRunRolledBackPayload].
	TopicWorkflowRunRolledBack = topicPrefix + "workflow_run.rolled_back"
)

// AllTopics is the closed list of topics mwanachama-backend-git publishes.
func AllTopics() []string {
	return []string{
		TopicRepoCreated,
		TopicRepoImported,
		TopicRepoImportFailed,
		TopicRepoImportCancelled,
		TopicBranchFetched,
		TopicBranchMerged,
		TopicMergeConflict,
		TopicMergeRequested,
		TopicMergeCompleted,
		TopicMergeFailed,
		TopicMergeRolledBack,
		TopicWorkflowRunRolledBack,
		TopicFileWritten,
	}
}

// RepoCreatedPayload is the [events.Publisher] payload for [TopicRepoCreated].
type RepoCreatedPayload struct {
	RepoID string `json:"repo_id"`
	Name   string `json:"name"`
	// WorkflowRunID links this event to its originating WorkflowRun, when one
	// exists (FEAT-20260602-001). Empty for repos created outside a run.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// RepoImportedPayload is the [events.Publisher] payload for [TopicRepoImported].
type RepoImportedPayload struct {
	JobID  string `json:"job_id"`
	RepoID string `json:"repo_id"`
	// WorkflowRunID links this event to its originating WorkflowRun, when one
	// exists (FEAT-20260602-001).
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// RepoImportFailedPayload is the [events.Publisher] payload for [TopicRepoImportFailed].
type RepoImportFailedPayload struct {
	JobID        string `json:"job_id"`
	ErrorMessage string `json:"error_message"`
	// WorkflowRunID links this event to its originating WorkflowRun, when one
	// exists (FEAT-20260602-001).
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// RepoImportCancelledPayload is the [events.Publisher] payload for [TopicRepoImportCancelled].
type RepoImportCancelledPayload struct {
	JobID string `json:"job_id"`
	// WorkflowRunID links this event to its originating WorkflowRun, when one
	// exists (FEAT-20260602-001).
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// BranchFetchedPayload is the [events.Publisher] payload for [TopicBranchFetched].
type BranchFetchedPayload struct {
	JobID    string `json:"job_id,omitempty"`
	BranchID string `json:"branch_id"`
	RepoID   string `json:"repo_id"`
	// WorkflowRunID links this event to its originating WorkflowRun
	// (FEAT-20260602-001). Empty for branches fetched outside any run.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// BranchMergedPayload is the [events.Publisher] payload for [TopicBranchMerged].
type BranchMergedPayload struct {
	BranchID string `json:"branch_id"`
	RepoID   string `json:"repo_id"`
	// WorkflowRunID links this event to its originating WorkflowRun
	// (FEAT-20260602-001). Copied from the merged Branch entity.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// MergeConflictPayload is the [events.Publisher] payload for [TopicMergeConflict].
type MergeConflictPayload struct {
	BranchID         string   `json:"branch_id"`
	ConflictingFiles []string `json:"conflicting_files"`
	// WorkflowRunID links this event to its originating WorkflowRun
	// (FEAT-20260602-001).
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// MergeRequestRequestedPayload is the [events.Publisher] payload for [TopicMergeRequested].
type MergeRequestRequestedPayload struct {
	MergeRequestID string `json:"merge_request_id"`
	RepoID         string `json:"repo_id"`
	// SourceBranchID is the branch whose commits are being requested for merge.
	SourceBranchID string `json:"source_branch_id"`
	// TargetBranchID is the branch the source will be merged into.
	TargetBranchID string `json:"target_branch_id,omitempty"`
	Title          string `json:"title"`
	// WorkflowRunID links the MR to its originating WorkflowRun (FEAT-20260602-001).
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// MergeRequestCompletedPayload is the [events.Publisher] payload for [TopicMergeCompleted].
type MergeRequestCompletedPayload struct {
	MergeRequestID  string `json:"merge_request_id"`
	RepoID          string `json:"repo_id"`
	SourceBranchID  string `json:"source_branch_id"`
	TargetBranchID  string `json:"target_branch_id,omitempty"`
	MergedCommitSHA string `json:"merged_commit_sha,omitempty"`
	// WorkflowRunID links the MR to its originating WorkflowRun (FEAT-20260602-001).
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// MergeRequestFailedPayload is the [events.Publisher] payload for [TopicMergeFailed].
type MergeRequestFailedPayload struct {
	MergeRequestID string `json:"merge_request_id"`
	RepoID         string `json:"repo_id"`
	SourceBranchID string `json:"source_branch_id"`
	ErrorMessage   string `json:"error_message"`
	// WorkflowRunID links the MR to its originating WorkflowRun (FEAT-20260602-001).
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// MergeRequestRolledBackPayload is the [events.Publisher] payload for
// [TopicMergeRolledBack]. Emitted once per [MergeRequest] flipped to
// "rolled_back" by [GitManager.RollbackByWorkflowRun] (FEAT-20260602-004).
// PriorStatus carries the status the MR was in before the rollback so
// downstream consumers can distinguish "we undid a merged MR" from "we
// abandoned an open MR".
type MergeRequestRolledBackPayload struct {
	MergeRequestID  string `json:"merge_request_id"`
	RepoID          string `json:"repo_id"`
	SourceBranchID  string `json:"source_branch_id"`
	PriorStatus     string `json:"prior_status"`
	MergedCommitSHA string `json:"merged_commit_sha,omitempty"`
	WorkflowRunID   string `json:"workflow_run_id"`
}

// WorkflowRunRolledBackPayload is the [events.Publisher] payload for
// [TopicWorkflowRunRolledBack]. Emitted once per
// [GitManager.RollbackByWorkflowRun] call, regardless of whether the run
// produced any Git artifacts. Counters mirror [RollbackResult].
type WorkflowRunRolledBackPayload struct {
	WorkflowRunID           string `json:"workflow_run_id"`
	BranchesDeleted         int    `json:"branches_deleted"`
	MergeRequestsRolledBack int    `json:"merge_requests_rolled_back"`
	DefaultBranchesSkipped  int    `json:"default_branches_skipped"`
}

// FileWrittenPayload is the [events.Publisher] payload for [TopicFileWritten].
// Published by mwanachama-backend-git after a successful [GitManager.WriteFile].
type FileWrittenPayload struct {
	// Repository is the repository the file was written to.
	Repository string `json:"repository"`
	// BranchName is the branch the commit was made on.
	BranchName string `json:"branch_name"`
	// Path is the file path that was written.
	Path string `json:"path"`
	// CommitSHA is the SHA of the new commit.
	CommitSHA string `json:"commit_sha"`
	// WorkflowRunID is carried through from the originating [WriteFileRequest],
	// when the branch it targets carries one. Empty when no run context applies.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}
