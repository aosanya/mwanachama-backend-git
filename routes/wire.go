package routes

import (
	"encoding/json"
	"errors"
	"net/http"

	mwanachamagit "github.com/aosanya/mwanachama-backend-git"
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// readJSON decodes a JSON body into v, refusing unknown fields.
func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// gitStatusFor maps GitManager's sentinel errors to a status code — the
// same triage mwanachama-backend-api-gateway's writeGitErr already does.
func gitStatusFor(err error) int {
	var conflict *mwanachamagit.ErrMergeConflict
	if errors.As(err, &conflict) {
		return http.StatusConflict
	}
	switch {
	case errors.Is(err, mwanachamagit.ErrRepoAlreadyExists),
		errors.Is(err, mwanachamagit.ErrBranchExists),
		errors.Is(err, mwanachamagit.ErrTagAlreadyExists),
		errors.Is(err, mwanachamagit.ErrKeywordAlreadyExists),
		errors.Is(err, mwanachamagit.ErrImportInProgress),
		errors.Is(err, mwanachamagit.ErrMergeRequestNotOpen),
		errors.Is(err, mwanachamagit.ErrDefaultBranchDeleteForbidden),
		errors.Is(err, mwanachamagit.ErrMergeConcurrencyConflict),
		errors.Is(err, mwanachamagit.ErrImportJobNotCancellable),
		errors.Is(err, mwanachamagit.ErrBranchAlreadyFetched):
		return http.StatusConflict
	case errors.Is(err, mwanachamagit.ErrRepoNotInitialised),
		errors.Is(err, mwanachamagit.ErrBranchNotFound),
		errors.Is(err, mwanachamagit.ErrTagNotFound),
		errors.Is(err, mwanachamagit.ErrMergeRequestNotFound),
		errors.Is(err, mwanachamagit.ErrKeywordNotFound),
		errors.Is(err, mwanachamagit.ErrImportJobNotFound),
		errors.Is(err, mwanachamagit.ErrFileNotFound),
		errors.Is(err, mwanachamagit.ErrRefNotFound),
		errors.Is(err, mwanachamagit.ErrEdgeNotFound),
		errors.Is(err, mwanachamagit.ErrEntityNotFound):
		return http.StatusNotFound
	case errors.Is(err, mwanachamagit.ErrWorkflowRunIDRequired),
		errors.Is(err, mwanachamagit.ErrInvalidRelationship),
		errors.Is(err, mwanachamagit.ErrBlobContentUnavailable):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// writeGitErr answers err on the wire. *mwanachamagit.ErrMergeConflict gets
// a richer body (task_id, conflicting_files) matching the gateway's own
// special case for MergeBranch's conflict response.
func writeGitErr(w http.ResponseWriter, err error) {
	var conflict *mwanachamagit.ErrMergeConflict
	if errors.As(err, &conflict) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":             err.Error(),
			"task_id":           conflict.TaskID,
			"conflicting_files": conflict.ConflictingFiles,
		})
		return
	}
	code := gitStatusFor(err)
	if code == http.StatusInternalServerError {
		writeErr(w, code, "internal error")
		return
	}
	writeErr(w, code, err.Error())
}
