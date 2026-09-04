// keyword.go — HTTP routes over the Keyword taxonomy: CreateKeyword,
// ListKeywords, GetKeywordTree, GetKeyword, UpdateKeyword, DeleteKeyword.
package routes

import (
	"net/http"
	"strconv"

	mwanachamagit "github.com/aosanya/mwanachama-backend-git"
)

// KeywordRoutes is the six Keyword operations. The static "/keywords/tree"
// path is registered ahead of the "/keywords/{keywordID}" wildcard — an
// exact literal match takes precedence under Go 1.22+ ServeMux.
func KeywordRoutes(gm mwanachamagit.GitManager) []Route {
	return []Route{
		{Method: http.MethodPost, Path: "/keywords", Handler: CreateKeyword(gm)},
		{Method: http.MethodGet, Path: "/keywords", Handler: ListKeywords(gm)},
		{Method: http.MethodGet, Path: "/keywords/tree", Handler: GetKeywordTree(gm)},
		{Method: http.MethodGet, Path: "/keywords/{keywordID}", Handler: GetKeyword(gm)},
		{Method: http.MethodPut, Path: "/keywords/{keywordID}", Handler: UpdateKeyword(gm)},
		{Method: http.MethodDelete, Path: "/keywords/{keywordID}", Handler: DeleteKeyword(gm)},
	}
}

// CreateKeyword handles POST /keywords.
func CreateKeyword(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in mwanachamagit.CreateKeywordRequest
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		out, err := gm.CreateKeyword(r.Context(), in)
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}
}

// ListKeywords handles GET /keywords?scope=&parent_id=&limit=.
func ListKeywords(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		filter := mwanachamagit.KeywordFilter{Scope: q.Get("scope"), ParentID: q.Get("parent_id")}
		if l := q.Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil {
				filter.Limit = n
			}
		}
		out, err := gm.ListKeywords(r.Context(), filter)
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// GetKeywordTree handles GET /keywords/tree?root=.
func GetKeywordTree(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := gm.GetKeywordTree(r.Context(), r.URL.Query().Get("root"))
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// GetKeyword handles GET /keywords/{keywordID}.
func GetKeyword(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := gm.GetKeyword(r.Context(), r.PathValue("keywordID"))
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// UpdateKeyword handles PUT /keywords/{keywordID}.
func UpdateKeyword(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in mwanachamagit.UpdateKeywordRequest
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		out, err := gm.UpdateKeyword(r.Context(), r.PathValue("keywordID"), in)
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// DeleteKeyword handles DELETE /keywords/{keywordID}.
func DeleteKeyword(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := gm.DeleteKeyword(r.Context(), r.PathValue("keywordID")); err != nil {
			writeGitErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
