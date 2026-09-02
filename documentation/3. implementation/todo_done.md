# mwanachama-git — completed tasks

| Task | Title | Completed | Notes |
|------|-------|-----------|-------|
| G1 | Bootstrap repo: `git init`, `go.mod`, `.gitignore`, `Makefile`, four-phase `documentation/` skeleton, `todo.md`/`todo_done.md` | 2026-09-02 | New repo. Depends on [mwanachama-go-shared](../../../mwanachama-go-shared) for its entity-graph store (wired via `go.mod` local `replace`). |
| G2 | Port `git.go`'s `GitManager` interface + `models.go` domain types (Repository/Branch/Commit/Tree/Blob/MergeRequest/Tag/Keyword) unchanged | 2026-09-02 | Added `models.go` (domain + request/filter types), `types.go` (`FileEntry`/`CommitEntry`/`FileDiff`/`ErrMergeConflict`/`ImportRepoRequest`/`ImportJob` — these are referenced directly by `GitManager`'s method signatures, so folded into this task even though `types.go` wasn't itself named on the board), and `errors.go` (v2 sentinel errors only — dropped the v1-only `ErrRepoNotFound`). Package name `mwanachamagit`. Dropped the unused v1 `AuthorInfo` type (dead even in the source). `git.go` carries the `GitManager` interface plus the self-contained `BlobSearcher`/`RefLocker`/`mutexLocker` support types; doc comments were lightly reworded (`gRPC server handler` → `HTTP handler`, `CodeValdWork`→`mwanachama-taskmanager`) since those referenced dropped v1 infrastructure. **Deferred to G4**: the concrete `gitManager` struct and `NewGitManager` constructor — the original wires in `entitygraph.DataManager` (ready), `Backend` (v1, out of scope), and `eventbus.Publisher` (not yet ported — `mwanachama-go-shared`'s `Publisher`/S7 is still in flight in another session as of this writing), so building the struct now would mean inventing a placeholder for a seam another repo owns. `go build ./...`, `go vet ./...`, and `gofmt -l .` all clean. |

## Archived board context

`mwanachama-git` — Go library, module `github.com/aosanya/mwanachama-git`.
Consumed by [mwanachama-api-gateway](../../../mwanachama-api-gateway) for
[mwanachama-kazi](../../../mwanachama-kazi).

### Why this repo exists

`mwanachama-kazi` needs git-like versioned content. `CodeValdGit` already
built this — as an ArangoDB-backed library for CodeValdCortex agencies, with
a gRPC service and (in its v1 API) real git wire-protocol serving. Neither
of those fit `mwanachama-api-gateway`, which is Postgres-only and runs as one
service with no sub-services.

**Decision: full port, same public API surface, v2 (`GitManager`) only.**
Exploration found `CodeValdGit`'s business logic (`git_impl_*.go`) is almost
entirely storage-agnostic — it calls `entitygraph.DataManager`, never AQL
directly, apart from one seam (`searcher.go`'s blob full-text search). So the
port is: reuse `git_impl_*.go`/`models.go`/`git.go` close to verbatim, retarget
storage onto [mwanachama-go-shared](../../../mwanachama-go-shared)'s Postgres
entity-graph engine, drop gRPC/proto/cmd/registrar, and drop the go-git
wire-protocol layer (v1 `Backend`, `storer.go`, `githttp.go`) since
`mwanachama-kazi` doesn't need real `git clone`/`push` interop.

Full task breakdown and rationale: originally scoped in
`/Users/tony/.claude/plans/kind-snacking-rose.md`.
