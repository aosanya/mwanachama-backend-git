# mwanachama-git — completed tasks

| Task | Title | Completed | Notes |
|------|-------|-----------|-------|
| G1 | Bootstrap repo: `git init`, `go.mod`, `.gitignore`, `Makefile`, four-phase `documentation/` skeleton, `todo.md`/`todo_done.md` | 2026-09-02 | New repo. Depends on [mwanachama-go-shared](../../../mwanachama-go-shared) for its entity-graph store (wired via `go.mod` local `replace`). |
| G2 | Port `git.go`'s `GitManager` interface + `models.go` domain types (Repository/Branch/Commit/Tree/Blob/MergeRequest/Tag/Keyword) unchanged | 2026-09-02 | Added `models.go` (domain + request/filter types), `types.go` (`FileEntry`/`CommitEntry`/`FileDiff`/`ErrMergeConflict`/`ImportRepoRequest`/`ImportJob` — these are referenced directly by `GitManager`'s method signatures, so folded into this task even though `types.go` wasn't itself named on the board), and `errors.go` (v2 sentinel errors only — dropped the v1-only `ErrRepoNotFound`). Package name `mwanachamagit`. Dropped the unused v1 `AuthorInfo` type (dead even in the source). `git.go` carries the `GitManager` interface plus the self-contained `BlobSearcher`/`RefLocker`/`mutexLocker` support types; doc comments were lightly reworded (`gRPC server handler` → `HTTP handler`, `CodeValdWork`→`mwanachama-taskmanager`) since those referenced dropped v1 infrastructure. **Deferred to G4**: the concrete `gitManager` struct and `NewGitManager` constructor — the original wires in `entitygraph.DataManager` (ready), `Backend` (v1, out of scope), and `eventbus.Publisher` (not yet ported — `mwanachama-go-shared`'s `Publisher`/S7 is still in flight in another session as of this writing), so building the struct now would mean inventing a placeholder for a seam another repo owns. `go build ./...`, `go vet ./...`, and `gofmt -l .` all clean. |
| G3 | Port `schema.go`'s `DefaultGitSchema()` onto `mwanachama-go-shared`'s type defs | 2026-09-02 | `mwanachama-go-shared`'s S2 landed (committed) by the time this started, so the dependency was live. Mechanical port of all ten `TypeDefinition`s onto `schema.TypeDefinition`/`PropertyDefinition`/`RelationshipDefinition` — dropped `PathSegment`/`EntityIDParam` on every type and relationship (go-shared's route-generation fields don't exist; mwanachama-api-gateway registers HTTP routes by hand). Dropped the `GitInternalState` type entirely and `Repository.head_ref` — both exist solely for the v1 go-git `storage.Storer` (grepped the original: neither is read/written by any `git_impl_*.go` file), which CLAUDE.md already scoped out. Kept `bare_clone_path`/`fetched_commit_shas`/`source_url` on Repository, `status`/`source_url` on Branch, and the `ImportJob`/`FetchBranchJob` types — these back the lazy-import v2 methods G2 already kept on `GitManager`; G6 will decide whether the fetch business logic actually gets implemented, but the schema needs to exist regardless since `types.go`'s `ImportJob`/`FetchBranchJob` already promise it. Added `schema_test.go` asserting `entitygraph.ValidateSchema(DefaultGitSchema())` passes (catches dangling `ToType`s, missing inverse relationships, bad `UniqueKey` fields) — not a G9 port-fidelity test, just a cheap authoring guard for this file. `go build`, `go vet`, `go test`, `gofmt -l` all clean. |

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
