// import.go — HTTP routes over async repository import: ImportRepo,
// GetImportStatus, CancelImport.
package routes

import (
	"net/http"

	mwanachamagit "github.com/aosanya/mwanachama-backend-git"
)

// ImportRoutes is the three async-import operations.
func ImportRoutes(gm mwanachamagit.GitManager) []Route {
	return []Route{
		{Method: http.MethodPost, Path: "/imports", Handler: ImportRepo(gm)},
		{Method: http.MethodGet, Path: "/imports/{jobID}", Handler: GetImportStatus(gm)},
		{Method: http.MethodPost, Path: "/imports/{jobID}/cancel", Handler: CancelImport(gm)},
	}
}

// importRepoRequestJSON is the wire shape for ImportRepo —
// mwanachamagit.ImportRepoRequest carries no JSON tags of its own since it
// was designed for direct Go-to-Go use.
type importRepoRequestJSON struct {
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	SourceURL     string `json:"source_url"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

// ImportRepo handles POST /imports.
func ImportRepo(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body importRepoRequestJSON
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		out, err := gm.ImportRepo(r.Context(), mwanachamagit.ImportRepoRequest{
			Name: body.Name, Description: body.Description,
			SourceURL: body.SourceURL, DefaultBranch: body.DefaultBranch,
		})
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, out)
	}
}

// GetImportStatus handles GET /imports/{jobID}.
func GetImportStatus(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := gm.GetImportStatus(r.Context(), r.PathValue("jobID"))
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// CancelImport handles POST /imports/{jobID}/cancel.
func CancelImport(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := gm.CancelImport(r.Context(), r.PathValue("jobID")); err != nil {
			writeGitErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
