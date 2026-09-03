# CLAUDE.md

Guidance for Claude Code working in this repository.

## Project: mwanachama-git

Postgres port of `CodeValdGit` for [mwanachama-frontend-kazi](../mwanachama-frontend-kazi).
Module path `github.com/aosanya/mwanachama-git`.

`CodeValdGit` actually contains two generations of API: a legacy v1
(`Backend`/`RepoManager`/`Repo`, real `go-git` storage + a git Smart HTTP
server for wire-protocol `clone`/`fetch`/`push`) and the current v2
(`GitManager`, entitygraph-native — branches/commits/trees/blobs/merge
requests/tags/rollback/history modeled as entities and edges in the graph
store). **Only v2 is in scope here.** `mwanachama-frontend-kazi` needs versioned
content, not a real git server; the v1 `Backend`, `storage/arangodb/storer.go`
(the go-git `storage.Storer` built on entity CRUD), and `internal/server/
githttp.go` are deliberately not ported. If real git-protocol interop turns
out to be needed later, that's a new scoped decision, not a default.

Also dropped from the original: `proto/`, `cmd/server`, `internal/server`
(gRPC `GitServiceServer`), `internal/registrar` (CodeValdCortex Cross
heartbeat) — `mwanachama-api-gateway` runs as one service and imports this
package directly.

## Porting notes

- `git.go`'s `GitManager` interface and `models.go`'s domain types
  (`Repository`, `Branch`, `Commit`, `Tree`, `Blob`, `MergeRequest`, `Tag`,
  `Keyword`) port essentially unchanged.
- `schema.go`'s `DefaultGitSchema()` (10 type defs, one shared relationship
  edge collection, `Immutable`+`UniqueKey` on `Commit`/`Tree`/`Blob` keyed by
  SHA) ports onto `mwanachama-go-shared`'s type-definition shape.
- Most `git_impl_*.go` files (branch, converters, edgelifecycle, graph,
  keywords, mergerequests, repo, rollback, tag) only ever call
  `entitygraph.DataManager` — no direct AQL — so they port with minimal
  churn once `mwanachama-go-shared`'s `DataManager` exists.
- `blobcache.go`, `fileops.go`, `import.go` build real git objects via
  `go-git/plumbing/object` — keep the `go-git` object model for parsing, drop
  only the wire-protocol transport layer around it.
- The one real feature gap: `searcher.go`'s ArangoSearch/BM25 blob search
  needs a Postgres `tsvector`/`ts_rank` (GIN index) equivalent behind the
  existing `BlobSearcher` interface seam — not a mechanical swap.

## Conventions

- Task status lives on
  [documentation/3. implementation/todo.md](documentation/3.%20implementation/todo.md).
- Four-phase `documentation/` layout — see
  [documentation/README.md](documentation/README.md).
