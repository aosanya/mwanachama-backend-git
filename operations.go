package mwanachamagit

import (
	_ "embed"
	"sync"

	"github.com/aosanya/mwanachama-backend-shared/dispatch"
)

//go:embed git.operations.json
var operationsJSON []byte

var loadOperations = sync.OnceValues(func() (*dispatch.Spec, error) {
	return dispatch.Parse(operationsJSON)
})

func Operations() (*dispatch.Spec, error) { return loadOperations() }

func OperationsJSON() []byte { return operationsJSON }

func Sentinels() map[string]error {
	return map[string]error{
		"ErrRepoAlreadyExists":            ErrRepoAlreadyExists,
		"ErrBranchExists":                 ErrBranchExists,
		"ErrTagAlreadyExists":             ErrTagAlreadyExists,
		"ErrKeywordAlreadyExists":         ErrKeywordAlreadyExists,
		"ErrImportInProgress":             ErrImportInProgress,
		"ErrMergeRequestNotOpen":          ErrMergeRequestNotOpen,
		"ErrDefaultBranchDeleteForbidden": ErrDefaultBranchDeleteForbidden,
		"ErrMergeConcurrencyConflict":     ErrMergeConcurrencyConflict,
		"ErrImportJobNotCancellable":      ErrImportJobNotCancellable,
		"ErrBranchAlreadyFetched":         ErrBranchAlreadyFetched,
		"ErrRepoNotInitialised":           ErrRepoNotInitialised,
		"ErrBranchNotFound":               ErrBranchNotFound,
		"ErrTagNotFound":                  ErrTagNotFound,
		"ErrMergeRequestNotFound":         ErrMergeRequestNotFound,
		"ErrKeywordNotFound":              ErrKeywordNotFound,
		"ErrImportJobNotFound":            ErrImportJobNotFound,
		"ErrFileNotFound":                 ErrFileNotFound,
		"ErrRefNotFound":                  ErrRefNotFound,
		"ErrEdgeNotFound":                 ErrEdgeNotFound,
		"ErrEntityNotFound":               ErrEntityNotFound,
		"ErrWorkflowRunIDRequired":        ErrWorkflowRunIDRequired,
		"ErrInvalidRelationship":          ErrInvalidRelationship,
		"ErrBlobContentUnavailable":       ErrBlobContentUnavailable,
	}
}
