// edge.go — HTTP routes over branch-scoped documentation edges: CreateEdge,
// DeleteEdge.
package routes

import (
	"net/http"

	mwanachamagit "github.com/aosanya/mwanachama-backend-git"
)

// EdgeRoutes is the two documentation-edge operations.
func EdgeRoutes(gm mwanachamagit.GitManager) []Route {
	return []Route{
		{Method: http.MethodPost, Path: "/edges", Handler: CreateEdge(gm)},
		{Method: http.MethodDelete, Path: "/edges", Handler: DeleteEdge(gm)},
	}
}

// CreateEdge handles POST /edges.
func CreateEdge(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in mwanachamagit.CreateEdgeRequest
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := gm.CreateEdge(r.Context(), in); err != nil {
			writeGitErr(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

// DeleteEdge handles DELETE /edges.
func DeleteEdge(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in mwanachamagit.DeleteEdgeRequest
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := gm.DeleteEdge(r.Context(), in); err != nil {
			writeGitErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
