// tag.go — HTTP routes over Tag management: CreateTag, GetTag, ListTags,
// DeleteTag.
package routes

import (
	"net/http"

	mwanachamagit "github.com/aosanya/mwanachama-backend-git"
)

// TagRoutes is the four Tag operations.
func TagRoutes(gm mwanachamagit.GitManager) []Route {
	return []Route{
		{Method: http.MethodPost, Path: "/repos/{repoID}/tags", Handler: CreateTag(gm)},
		{Method: http.MethodGet, Path: "/repos/{repoID}/tags", Handler: ListTags(gm)},
		{Method: http.MethodGet, Path: "/tags/{tagID}", Handler: GetTag(gm)},
		{Method: http.MethodDelete, Path: "/tags/{tagID}", Handler: DeleteTag(gm)},
	}
}

// CreateTag handles POST /repos/{repoID}/tags.
func CreateTag(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in mwanachamagit.CreateTagRequest
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		in.RepositoryID = r.PathValue("repoID")
		out, err := gm.CreateTag(r.Context(), in)
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}
}

// ListTags handles GET /repos/{repoID}/tags.
func ListTags(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := gm.ListTags(r.Context(), r.PathValue("repoID"))
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// GetTag handles GET /tags/{tagID}.
func GetTag(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := gm.GetTag(r.Context(), r.PathValue("tagID"))
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// DeleteTag handles DELETE /tags/{tagID}.
func DeleteTag(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := gm.DeleteTag(r.Context(), r.PathValue("tagID")); err != nil {
			writeGitErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
