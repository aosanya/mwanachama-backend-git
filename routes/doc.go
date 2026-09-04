// Package routes is mwanachama-backend-git's own HTTP surface: decode a
// request, call one [mwanachamagit.GitManager] method, encode the response —
// reachable from an HTTP mux without a mounting process reimplementing the
// request/response shape a second time.
//
// Mirrors the route table mwanachama-backend-api-gateway's
// internal/api/http/git_handlers*.go already hand-registers today (repos,
// branches, tags, merge requests, file operations, history, async import,
// keyword taxonomy, branch-scoped documentation edges, graph queries, lazy
// on-demand fetch, blob search) — same paths, same methods, same status
// codes, same error-sentinel-to-status mapping, so a mounting process can
// swap its own handler wiring for [Routes] without changing its API surface.
//
// Unlike mwanachama-backend-actor's routes package, there is no
// ResourceNames indirection here: none of these path nouns ("repos",
// "branches", "merge-requests", ...) are an org-configurable label the way
// actor's "Group" (aliased to "chapters" by the gateway) is — git vocabulary
// is git vocabulary regardless of deployment, so the paths are fixed.
//
// A route built from this package still needs a caller-identity/capability
// gate wrapped around it before it is safe to serve — this package answers
// "what happens once that gate has passed", never "who may pass it". The
// gateway's own CapGitRead/CapGitWrite split is exactly that gate, supplied
// by wrapping the http.HandlerFunc this package returns.
package routes
