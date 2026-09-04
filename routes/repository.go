// repository.go — HTTP routes over Repository lifecycle: InitRepo,
// ListRepositories, GetRepository, GetRepositoryByName, DeleteRepo, PurgeRepo.
package routes

import (
	"net/http"

	mwanachamagit "github.com/aosanya/mwanachama-backend-git"
)

// RepositoryRoutes is the five Repository lifecycle operations.
func RepositoryRoutes(gm mwanachamagit.GitManager) []Route {
	return []Route{
		{Method: http.MethodPost, Path: "/repos", Handler: InitRepo(gm)},
		{Method: http.MethodGet, Path: "/repos", Handler: ListOrGetRepositoryByName(gm)},
		{Method: http.MethodGet, Path: "/repos/{repoID}", Handler: GetRepository(gm)},
		{Method: http.MethodDelete, Path: "/repos/{repoID}", Handler: DeleteRepo(gm)},
		{Method: http.MethodPost, Path: "/repos/{repoID}/purge", Handler: PurgeRepo(gm)},
	}
}

// InitRepo handles POST /repos.
func InitRepo(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in mwanachamagit.CreateRepoRequest
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		out, err := gm.InitRepo(r.Context(), in)
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}
}

// ListOrGetRepositoryByName handles GET /repos — every repository, or a
// single one when ?name= is given (avoids a second static path colliding
// with /repos/{repoID}'s wildcard segment).
func ListOrGetRepositoryByName(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if name := r.URL.Query().Get("name"); name != "" {
			out, err := gm.GetRepositoryByName(r.Context(), name)
			if err != nil {
				writeGitErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, out)
			return
		}
		out, err := gm.ListRepositories(r.Context())
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// GetRepository handles GET /repos/{repoID}.
func GetRepository(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := gm.GetRepository(r.Context(), r.PathValue("repoID"))
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// DeleteRepo handles DELETE /repos/{repoID}.
func DeleteRepo(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := gm.DeleteRepo(r.Context(), r.PathValue("repoID")); err != nil {
			writeGitErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// PurgeRepo handles POST /repos/{repoID}/purge.
func PurgeRepo(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := gm.PurgeRepo(r.Context(), r.PathValue("repoID")); err != nil {
			writeGitErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
