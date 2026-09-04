// fetchbranch.go — HTTP routes over the lazy on-demand branch fetch:
// FetchBranch, GetFetchBranchStatus.
package routes

import (
	"net/http"

	mwanachamagit "github.com/aosanya/mwanachama-backend-git"
)

// FetchBranchRoutes is the two lazy-fetch operations.
func FetchBranchRoutes(gm mwanachamagit.GitManager) []Route {
	return []Route{
		{Method: http.MethodPost, Path: "/branches/{branchID}/fetch", Handler: FetchBranch(gm)},
		{Method: http.MethodGet, Path: "/fetch-jobs/{jobID}", Handler: GetFetchBranchStatus(gm)},
	}
}

// FetchBranch handles POST /branches/{branchID}/fetch?repo_id=.
func FetchBranch(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoID := r.URL.Query().Get("repo_id")
		if repoID == "" {
			writeErr(w, http.StatusBadRequest, "repo_id is required")
			return
		}
		out, err := gm.FetchBranch(r.Context(), mwanachamagit.FetchBranchRequest{
			RepoID: repoID, BranchID: r.PathValue("branchID"),
		})
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, out)
	}
}

// GetFetchBranchStatus handles GET /fetch-jobs/{jobID}.
func GetFetchBranchStatus(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := gm.GetFetchBranchStatus(r.Context(), r.PathValue("jobID"))
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}
