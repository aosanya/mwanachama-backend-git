// mergerequest.go — HTTP routes over MergeRequest lifecycle:
// CreateMergeRequest, ListMergeRequests, GetMergeRequest,
// CompleteMergeRequest, CloseMergeRequest, RollbackByWorkflowRun.
package routes

import (
	"net/http"

	mwanachamagit "github.com/aosanya/mwanachama-backend-git"
)

// MergeRequestRoutes is the six MergeRequest/rollback operations.
func MergeRequestRoutes(gm mwanachamagit.GitManager) []Route {
	return []Route{
		{Method: http.MethodPost, Path: "/merge-requests", Handler: CreateMergeRequest(gm)},
		{Method: http.MethodGet, Path: "/merge-requests", Handler: ListMergeRequests(gm)},
		{Method: http.MethodGet, Path: "/merge-requests/{mrID}", Handler: GetMergeRequest(gm)},
		{Method: http.MethodPost, Path: "/merge-requests/{mrID}/complete", Handler: CompleteMergeRequest(gm)},
		{Method: http.MethodPost, Path: "/merge-requests/{mrID}/close", Handler: CloseMergeRequest(gm)},
		{Method: http.MethodPost, Path: "/workflow-runs/{workflowRunID}/rollback", Handler: RollbackByWorkflowRun(gm)},
	}
}

// CreateMergeRequest handles POST /merge-requests.
func CreateMergeRequest(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in mwanachamagit.CreateMergeRequestRequest
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		out, err := gm.CreateMergeRequest(r.Context(), in)
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}
}

// ListMergeRequests handles GET /merge-requests, filtered by the optional
// ?repository_id=, ?status=, ?workflow_run_id= query parameters.
func ListMergeRequests(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		filter := mwanachamagit.MergeRequestFilter{
			RepositoryID:  q.Get("repository_id"),
			Status:        q.Get("status"),
			WorkflowRunID: q.Get("workflow_run_id"),
		}
		out, err := gm.ListMergeRequests(r.Context(), filter)
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// GetMergeRequest handles GET /merge-requests/{mrID}.
func GetMergeRequest(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := gm.GetMergeRequest(r.Context(), r.PathValue("mrID"))
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// CompleteMergeRequest handles POST /merge-requests/{mrID}/complete.
func CompleteMergeRequest(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := gm.CompleteMergeRequest(r.Context(), r.PathValue("mrID"))
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// CloseMergeRequest handles POST /merge-requests/{mrID}/close.
func CloseMergeRequest(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := gm.CloseMergeRequest(r.Context(), r.PathValue("mrID"))
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// RollbackByWorkflowRun handles POST /workflow-runs/{workflowRunID}/rollback.
func RollbackByWorkflowRun(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := gm.RollbackByWorkflowRun(r.Context(), r.PathValue("workflowRunID"))
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}
