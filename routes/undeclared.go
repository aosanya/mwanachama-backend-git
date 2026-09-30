package routes

import (
	"errors"
	"net/http"

	"github.com/aosanya/mwanachama-backend-shared/httpwire"

	mwanachamagit "github.com/aosanya/mwanachama-backend-git"
)

func undeclared(gm mwanachamagit.GitManager) []Route {
	return []Route{
		{Method: http.MethodGet, Path: "/repos", Handler: listOrGetRepositoryByName(gm)},
		{Method: http.MethodGet, Path: "/repos/{repoID}/branches", Handler: listBranches(gm)},
		{Method: http.MethodPost, Path: "/branches/{branchID}/merge", Handler: mergeBranch(gm)},
	}
}

func listOrGetRepositoryByName(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if name := r.URL.Query().Get("name"); name != "" {
			out, err := gm.GetRepositoryByName(r.Context(), name)
			answer(w, out, err, http.StatusOK)
			return
		}
		out, err := gm.ListRepositories(r.Context())
		answer(w, out, err, http.StatusOK)
	}
}

func listBranches(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoID := r.PathValue("repoID")
		q := r.URL.Query()
		if name := q.Get("name"); name != "" {
			out, err := gm.GetBranchByName(r.Context(), repoID, name)
			answer(w, out, err, http.StatusOK)
			return
		}
		if runID := q.Get("workflow_run_id"); runID != "" {
			out, err := gm.ListBranchesFiltered(r.Context(), repoID, mwanachamagit.BranchFilter{WorkflowRunID: runID})
			answer(w, out, err, http.StatusOK)
			return
		}
		out, err := gm.ListBranches(r.Context(), repoID)
		answer(w, out, err, http.StatusOK)
	}
}

func mergeBranch(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := gm.MergeBranch(r.Context(), r.PathValue("branchID"))
		var conflict *mwanachamagit.ErrMergeConflict
		if errors.As(err, &conflict) {
			httpwire.WriteJSON(w, http.StatusConflict, map[string]any{
				"error":             err.Error(),
				"task_id":           conflict.TaskID,
				"conflicting_files": conflict.ConflictingFiles,
			})
			return
		}
		answer(w, out, err, http.StatusOK)
	}
}

func answer(w http.ResponseWriter, out any, err error, code int) {
	if err != nil {
		status := httpwire.StatusFor(err, statuses(), http.StatusInternalServerError)
		if status == http.StatusInternalServerError {
			httpwire.WriteErr(w, status, "internal error")
			return
		}
		httpwire.WriteErr(w, status, err.Error())
		return
	}
	httpwire.WriteJSON(w, code, out)
}

func statuses() map[error]int {
	spec, err := Table.Spec()
	if err != nil {
		return nil
	}
	out := make(map[error]int, len(spec.Errors))
	for name, code := range spec.Errors {
		if sentinel, ok := Sentinels[name]; ok {
			out[sentinel] = code
		}
	}
	return out
}
