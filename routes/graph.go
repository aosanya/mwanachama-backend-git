// graph.go — HTTP routes over graph queries: GetNeighborhood,
// SearchByKeywords, QueryGraph.
package routes

import (
	"net/http"
	"strconv"

	mwanachamagit "github.com/aosanya/mwanachama-backend-git"
)

// GraphRoutes is the three graph-query operations — reads only, no mutation.
func GraphRoutes(gm mwanachamagit.GitManager) []Route {
	return []Route{
		{Method: http.MethodGet, Path: "/branches/{branchID}/neighborhood/{entityID}", Handler: GetNeighborhood(gm)},
		{Method: http.MethodPost, Path: "/search/keywords", Handler: SearchByKeywords(gm)},
		{Method: http.MethodPost, Path: "/graph/query", Handler: QueryGraph(gm)},
	}
}

// GetNeighborhood handles GET /branches/{branchID}/neighborhood/{entityID}?depth=.
func GetNeighborhood(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		depth := 1
		if d := r.URL.Query().Get("depth"); d != "" {
			if n, err := strconv.Atoi(d); err == nil {
				depth = n
			}
		}
		out, err := gm.GetNeighborhood(r.Context(), r.PathValue("branchID"), r.PathValue("entityID"), depth)
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// SearchByKeywords handles POST /search/keywords.
func SearchByKeywords(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in mwanachamagit.SearchByKeywordsRequest
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		out, err := gm.SearchByKeywords(r.Context(), in)
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// QueryGraph handles POST /graph/query. The body is optional — an empty
// request returns the top signal-ranked Blob nodes.
func QueryGraph(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in mwanachamagit.QueryGraphRequest
		if r.ContentLength > 0 {
			if err := readJSON(r, &in); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		out, err := gm.QueryGraph(r.Context(), in)
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}
