package routes

import (
	"net/http"

	mwanachamagit "github.com/aosanya/mwanachama-backend-git"
)

// Route is one address this package answers, relative to wherever the
// mounting process prefixes it (e.g. "/v1/git"). Path uses net/http's
// ServeMux pattern syntax ("{repoID}" etc., Go 1.22+), so the mounting
// process only ever needs prefix+rt.Path, never its own copy of the path
// text.
type Route struct {
	Method  string
	Path    string
	Handler http.HandlerFunc
}

// Pattern returns the http.ServeMux registration pattern for this route once
// mounted under prefix.
func (r Route) Pattern(prefix string) string {
	return r.Method + " " + prefix + r.Path
}

// Routes is every address this package answers: every resource group's
// routes concatenated. A mounting process that wants all of it in one loop
// uses this; one that wants to gate different groups differently (the
// gateway does, today — CapGitRead vs CapGitWrite) calls the per-group
// functions below directly instead.
func Routes(gm mwanachamagit.GitManager) []Route {
	var out []Route
	out = append(out, RepositoryRoutes(gm)...)
	out = append(out, BranchRoutes(gm)...)
	out = append(out, TagRoutes(gm)...)
	out = append(out, MergeRequestRoutes(gm)...)
	out = append(out, FileRoutes(gm)...)
	out = append(out, ImportRoutes(gm)...)
	out = append(out, KeywordRoutes(gm)...)
	out = append(out, EdgeRoutes(gm)...)
	out = append(out, GraphRoutes(gm)...)
	out = append(out, FetchBranchRoutes(gm)...)
	out = append(out, BlobSearchRoutes(gm)...)
	return out
}
