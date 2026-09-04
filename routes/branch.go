// branch.go — HTTP routes over Branch management: CreateBranch, GetBranch,
// ListBranches, GetBranchByName, ListBranchesFiltered, DeleteBranch,
// MergeBranch.
package routes

import (
	"net/http"

	mwanachamagit "github.com/aosanya/mwanachama-backend-git"
)

// BranchRoutes is the five Branch operations.
func BranchRoutes(gm mwanachamagit.GitManager) []Route {
	return []Route{
		{Method: http.MethodPost, Path: "/repos/{repoID}/branches", Handler: CreateBranch(gm)},
		{Method: http.MethodGet, Path: "/repos/{repoID}/branches", Handler: ListBranches(gm)},
		{Method: http.MethodGet, Path: "/branches/{branchID}", Handler: GetBranch(gm)},
		{Method: http.MethodDelete, Path: "/branches/{branchID}", Handler: DeleteBranch(gm)},
		{Method: http.MethodPost, Path: "/branches/{branchID}/merge", Handler: MergeBranch(gm)},
	}
}

// CreateBranch handles POST /repos/{repoID}/branches.
func CreateBranch(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in mwanachamagit.CreateBranchRequest
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		in.RepositoryID = r.PathValue("repoID")
		out, err := gm.CreateBranch(r.Context(), in)
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}
}

// ListBranches handles GET /repos/{repoID}/branches — every branch, a single
// one by ?name=, or filtered by ?workflow_run_id=.
func ListBranches(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoID := r.PathValue("repoID")
		q := r.URL.Query()
		if name := q.Get("name"); name != "" {
			out, err := gm.GetBranchByName(r.Context(), repoID, name)
			if err != nil {
				writeGitErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, out)
			return
		}
		if runID := q.Get("workflow_run_id"); runID != "" {
			out, err := gm.ListBranchesFiltered(r.Context(), repoID, mwanachamagit.BranchFilter{WorkflowRunID: runID})
			if err != nil {
				writeGitErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, out)
			return
		}
		out, err := gm.ListBranches(r.Context(), repoID)
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// GetBranch handles GET /branches/{branchID}.
func GetBranch(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := gm.GetBranch(r.Context(), r.PathValue("branchID"))
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// DeleteBranch handles DELETE /branches/{branchID}.
func DeleteBranch(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := gm.DeleteBranch(r.Context(), r.PathValue("branchID")); err != nil {
			writeGitErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// MergeBranch handles POST /branches/{branchID}/merge.
func MergeBranch(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := gm.MergeBranch(r.Context(), r.PathValue("branchID"))
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}
