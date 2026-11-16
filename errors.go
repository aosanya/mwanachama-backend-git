package mwanachamagit

import "errors"

var ErrMergeConflicted = errors.New("merge conflict")

var ErrRepoAlreadyExists = errors.New("repository already exists")

var ErrBranchNotFound = errors.New("branch not found")

var ErrBranchExists = errors.New("branch already exists")

var ErrFileNotFound = errors.New("file not found")

var ErrRefNotFound = errors.New("ref not found (branch, tag, or SHA)")

var ErrRepoNotInitialised = errors.New("repository not initialised")

var ErrTagAlreadyExists = errors.New("tag already exists")

var ErrTagNameNotAllowed = errors.New("tag name matches no declared tag pattern")

var ErrTagNotFound = errors.New("tag not found")

var ErrDefaultBranchDeleteForbidden = errors.New("cannot delete the default branch")

var ErrImportJobNotFound = errors.New("import job not found")

var ErrImportInProgress = errors.New("import already in progress")

var ErrImportJobNotCancellable = errors.New("import job is not cancellable")

var ErrKeywordNotFound = errors.New("keyword not found")

var ErrKeywordAlreadyExists = errors.New("keyword already exists")

var ErrEdgeNotFound = errors.New("documentation edge not found")

var ErrBranchAlreadyFetched = errors.New("branch is already fetched or fetch is in progress")

var ErrBlobContentUnavailable = errors.New("blob content not yet available; trigger FetchBranch and retry")

var ErrInvalidRelationship = errors.New("invalid documentation relationship name")

var ErrMergeConcurrencyConflict = errors.New("merge conflict: default branch HEAD changed concurrently")

var ErrMergeRequestNotFound = errors.New("merge request not found")

var ErrMergeRequestNotOpen = errors.New("merge request is not open")

var ErrWorkflowRunIDRequired = errors.New("workflow_run_id is required")

var ErrEntityNotFound = errors.New("entity not found")

var ErrFileTooLarge = errors.New("file exceeds the size limit")

var ErrMergeNotPermitted = errors.New("merge not permitted")

var ErrQueryRequired = errors.New("query must not be empty")

var ErrTitleRequired = errors.New("title is required")

var ErrSourceBranchRequired = errors.New("source_branch_id is required")

var ErrRepositoryNotRegistered = errors.New("repository file has no catalog entry")
