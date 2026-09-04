// blobsearch.go — HTTP route over blob full-text search: SearchBlobs.
package routes

import (
	"net/http"

	mwanachamagit "github.com/aosanya/mwanachama-backend-git"
)

// BlobSearchRoutes is the one blob-search operation.
func BlobSearchRoutes(gm mwanachamagit.GitManager) []Route {
	return []Route{
		{Method: http.MethodPost, Path: "/search/blobs", Handler: SearchBlobs(gm)},
	}
}

// SearchBlobs handles POST /search/blobs.
func SearchBlobs(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in mwanachamagit.SearchBlobsRequest
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		out, err := gm.SearchBlobs(r.Context(), in)
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}
