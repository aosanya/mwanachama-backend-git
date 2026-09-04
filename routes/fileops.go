// fileops.go — HTTP routes over branch-scoped file operations and history:
// WriteFile, ReadFile, DeleteFile, ListDirectory, Log, Diff.
//
// FileEntry, CommitEntry, and FileDiff carry no JSON tags of their own (they
// are plain Go-to-Go value types) — this file wraps each in a snake_case
// JSON shape, mirroring mwanachama-backend-api-gateway's own local wrapper
// DTOs.
package routes

import (
	"net/http"
	"strconv"
	"time"

	mwanachamagit "github.com/aosanya/mwanachama-backend-git"
)

// FileRoutes is the six file-operation/history operations.
func FileRoutes(gm mwanachamagit.GitManager) []Route {
	return []Route{
		{Method: http.MethodPost, Path: "/branches/{branchID}/files", Handler: WriteFile(gm)},
		{Method: http.MethodGet, Path: "/branches/{branchID}/files", Handler: ReadFile(gm)},
		{Method: http.MethodDelete, Path: "/branches/{branchID}/files", Handler: DeleteFile(gm)},
		{Method: http.MethodGet, Path: "/branches/{branchID}/directory", Handler: ListDirectory(gm)},
		{Method: http.MethodGet, Path: "/branches/{branchID}/log", Handler: Log(gm)},
		{Method: http.MethodGet, Path: "/diff", Handler: Diff(gm)},
	}
}

type fileEntryJSON struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

func fileEntriesJSON(es []mwanachamagit.FileEntry) []fileEntryJSON {
	out := make([]fileEntryJSON, len(es))
	for i, e := range es {
		out[i] = fileEntryJSON{Name: e.Name, Path: e.Path, IsDir: e.IsDir, Size: e.Size}
	}
	return out
}

type commitEntryJSON struct {
	SHA       string `json:"sha"`
	Author    string `json:"author"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

func commitEntriesJSON(cs []mwanachamagit.CommitEntry) []commitEntryJSON {
	out := make([]commitEntryJSON, len(cs))
	for i, c := range cs {
		out[i] = commitEntryJSON{SHA: c.SHA, Author: c.Author, Message: c.Message, Timestamp: c.Timestamp.Format(time.RFC3339)}
	}
	return out
}

type fileDiffJSON struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
	Patch     string `json:"patch,omitempty"`
}

func fileDiffsJSON(ds []mwanachamagit.FileDiff) []fileDiffJSON {
	out := make([]fileDiffJSON, len(ds))
	for i, d := range ds {
		out[i] = fileDiffJSON{Path: d.Path, Operation: d.Operation, Patch: d.Patch}
	}
	return out
}

// WriteFile handles POST /branches/{branchID}/files.
func WriteFile(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in mwanachamagit.WriteFileRequest
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		in.BranchID = r.PathValue("branchID")
		out, err := gm.WriteFile(r.Context(), in)
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}
}

// ReadFile handles GET /branches/{branchID}/files?path=.
func ReadFile(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			writeErr(w, http.StatusBadRequest, "path is required")
			return
		}
		out, err := gm.ReadFile(r.Context(), r.PathValue("branchID"), path)
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// DeleteFile handles DELETE /branches/{branchID}/files. The path may come
// from the body or, when the request has no body, from ?path=.
func DeleteFile(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in mwanachamagit.DeleteFileRequest
		if r.ContentLength > 0 {
			if err := readJSON(r, &in); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		if in.Path == "" {
			in.Path = r.URL.Query().Get("path")
		}
		if in.Path == "" {
			writeErr(w, http.StatusBadRequest, "path is required")
			return
		}
		in.BranchID = r.PathValue("branchID")
		out, err := gm.DeleteFile(r.Context(), in)
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// ListDirectory handles GET /branches/{branchID}/directory?path=.
func ListDirectory(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := gm.ListDirectory(r.Context(), r.PathValue("branchID"), r.URL.Query().Get("path"))
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, fileEntriesJSON(out))
	}
}

// Log handles GET /branches/{branchID}/log?path=&limit=.
func Log(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		filter := mwanachamagit.LogFilter{Path: q.Get("path")}
		if l := q.Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil {
				filter.Limit = n
			}
		}
		out, err := gm.Log(r.Context(), r.PathValue("branchID"), filter)
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, commitEntriesJSON(out))
	}
}

// Diff handles GET /diff?from=&to=.
func Diff(gm mwanachamagit.GitManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		from, to := q.Get("from"), q.Get("to")
		if from == "" || to == "" {
			writeErr(w, http.StatusBadRequest, "from and to are required")
			return
		}
		out, err := gm.Diff(r.Context(), from, to)
		if err != nil {
			writeGitErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, fileDiffsJSON(out))
	}
}
