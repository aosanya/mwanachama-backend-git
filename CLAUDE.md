# CLAUDE.md

Guidance for Claude Code working in this repository.

## Project: mwanachama-backend-git

Postgres port of `CodeValdGit` for [mwanachama-frontend-kazi](../mwanachama-frontend-kazi).
Module path `github.com/aosanya/mwanachama-backend-git`.

`CodeValdGit` actually contains two generations of API: a legacy v1
(`Backend`/`RepoManager`/`Repo`, real `go-git` storage + a git Smart HTTP
server for wire-protocol `clone`/`fetch`/`push`) and the current v2
(`GitManager`, now GORM-native — branches/commits/trees/blobs/merge
requests/tags/rollback/history modeled as relational rows). **Only v2 is in
scope here.** `mwanachama-frontend-kazi` needs versioned content, not a real git
server; the v1 `Backend`, `storage/arangodb/storer.go` (the go-git
`storage.Storer` built on entity CRUD), and `internal/server/githttp.go` are
deliberately not ported. If real git-protocol interop turns out to be
needed later, that's a new scoped decision, not a default.

Also dropped from the original: `proto/`, `cmd/server`, `internal/server`
(gRPC `GitServiceServer`), `internal/registrar` (CodeValdCortex Cross
heartbeat) — `mwanachama-backend-api-gateway` runs as one service and imports this
package directly.

**Storage moved from entitygraph to GORM, 2026-09-04.** This package
originally stored Repository/Branch/MergeRequest/Tag/Commit/Tree/Blob/Keyword
through `mwanachama-backend-shared/entitygraph.DataManager` — a generic,
runtime-versioned entity/relationship graph engine, with every entity
sharing one Postgres `entities` table and every edge sharing one
`relationships` table. Follows the same storage-layer migration
`mwanachama-backend-actor` did on 2026-09-04, applied here to a much larger
domain (10 entity types instead of 3) and — the one thing actor's migration
never had to solve — three methods (`GetNeighborhood`, `SearchByKeywords`,
`QueryGraph`) that did generic, any-type/any-direction graph exploration
against entitygraph's shared relationships table. This was a storage-layer
swap only: `GitManager`'s method set is unchanged; only `NewGitManager`'s
constructor and everything behind it changed shape. See the `gormstore/`
package.

**Flattened fully to explicit typed FK columns and join tables — no
generic edges table survives, 2026-09-04.** Decided explicitly (not
inherited from actor by default): actor's migration had no generic-graph
feature to preserve in the first place, so "follow actor's precedent" here
meant flattening completely, the same way actor's one relationship
(`ActorGroupAssignment`) became a real join table rather than a
polymorphic edge row. Sixteen physical tables replace entitygraph's two:
nine node tables (one per entity type) plus four join tables —
`git_commit_parents` (many-to-many, `parent_index` preserving first-parent
vs. merge-parent order), `git_tree_blobs`, `git_tree_subtrees`, and two
fixed-shape Blob&harr;Blob/Blob&harr;Keyword tables
(`git_blob_keyword_tags` for `tagged_with`, `git_blob_references` for
`references`/`referenced_by`/four other constrained labels — see
`git_impl_keywords.go`'s `validDocEdges`). See `gormstore/tables.go` for the
full column-by-column schema and `gormstore/queries.go` for the recursive
CTEs (`BlobsAtCommit`, `CommitChainIDs`, `KeywordDescendantIDs`) and the
sixteen-shape edge catalogue (`NeighborhoodEdges`) that replace entitygraph's
generic traversal.

**What this costs, precisely (`git_impl_graph.go`):**
- `GetNeighborhood`'s `GraphEdge.ID` is no longer a real storage key — FK-
  and join-derived edges have no row ID, so it's now a synthetic
  `"<name>:<fromID>:<toID>"` used only for BFS dedup. No caller round-tripped
  an edge ID before, so nothing breaks, but the value's meaning changed.
- Any edge label outside the enumerated sixteen shapes (or outside
  `blob_references`'s constrained `Name` set) is unrepresentable without a
  schema migration — the direct consequence of flattening, not an oversight.
- `SearchByKeywords`'s result set is now formally all-Blob (it always was in
  practice — `tagged_with` only ever pointed at Blobs — just was type-open
  before).
- `QueryGraph` and `SearchByKeywords` still don't filter by `req.BranchID`
  on the `tagged_with`/`references` rows themselves (only a branch
  *existence* check runs first) — a real, pre-existing gap carried over
  unchanged from the entitygraph version, not fixed here. Each is a one-line
  `AND branch_id = ?` addition later if ever wanted.

**Behavior fixed along the way (not mechanical, called out explicitly):**
- `Keyword.ChildIDs` is now actually populated by `GetKeyword` (the
  entitygraph-era `entityToKeyword` never filled it).
- `ListKeywords`/`listKeywordsByParent("")` now correctly returns only root
  keywords — the entitygraph version returned *every* keyword when
  `parentID == ""` and left filtering to the caller (its own code comment
  apologized for this).
- `DeleteKeyword` now cascades a delete on `git_blob_keyword_tags` for the
  removed keyword — the entitygraph version left `tagged_with` rows
  dangling against a keyword that no longer existed.
- `advanceBranchHead`'s CAS guard is a single conditional `UPDATE ... WHERE
  id = ? AND head_commit_id = ?` (checking `RowsAffected`) instead of a
  separate read-then-write — closes the race window the old
  read-then-CreateRelationship/UpdateEntity pair had between the check and
  the write.

**Behavior deliberately NOT fixed (preserved for parity, do not "fix" without
a scoped decision):**
- `Commit`/`Tree`/`Blob` `sha` has **no unique index**. Entitygraph declared
  `UniqueKey: []string{"sha"}` for all three but this repo only ever called
  `CreateEntity`, never `UpsertEntity` — the one call that actually applied
  a partial unique index — so the declared uniqueness was never enforced.
  Adding a real unique index now would break previously-succeeding writes
  (e.g. `WriteFile`/`FetchBranch` re-materialising the same content). The
  `errors.Is(err, ErrEntityAlreadyExists)`-guarded fallback lookups in the
  old `git_impl_fetchbranch.go` were dead code for the same reason and were
  deleted, not ported.
- `WriteFile` still wires every blob directly onto the root tree
  (`git_tree_blobs`), not through the actual nested subtree structure — the
  non-root `Tree` rows `buildNestedTrees` produces are created but never
  linked by any join row. `FetchBranch`'s `upsertTreeMetadataWithEdges` *does*
  wire proper nesting (`git_tree_subtrees`); this asymmetry between the two
  write paths is inherited, not introduced.
- Repositories created via `ImportRepo`/`runImport` are not linked to the
  singleton `Agency` row (`AgencyID` stays `NULL`) — only `InitRepo` links
  one. Pre-existing gap, carried over unchanged.

**Scope: this repo only, 2026-09-04.** `mwanachama-backend-api-gateway`'s
wiring (`cmd/server/stores.go`'s `memoryStores`/`postgresStores`, both of
which construct `mwanachamagit.NewGitManager` inline — there is no
`git_instances.go` isolation layer the way actor's migration had
`user_instances.go`) now calls a `NewGitManager` signature that no longer
exists and will not compile until a follow-up change updates it — not done
here, by explicit scope decision, mirroring actor's own precedent. The
gateway repo was independently broken for unrelated reasons
(`mwanachamacomm` package) at the time of this migration, so this is not a
newly-introduced regression to a previously-green build.

## Porting notes

- `git.go`'s `GitManager` interface (method set unchanged) and the domain
  types in `mwanachama-backend-git/models` (`Agency`, `Repository`, `Branch`,
  `MergeRequest`, `Tag`, `Commit`, `Tree`, `Blob`, `Keyword`, `ImportJob`,
  `FetchBranchJob`) port field-for-field from the entitygraph era. Request/
  filter/graph DTOs (`CreateRepoRequest`, `MergeRequestFilter`, `GraphNode`/
  `GraphEdge`/`GraphResult`, `QueryGraphRequest`, etc.) are not persisted
  entities and stay in the root package's `models.go`, mirroring actor's
  split (only true domain types move to `models/`).
- Three domain fields are no longer stored columns, only derived at read
  time and written via a companion join-row builder at write time:
  `Commit.ParentIDs` (`gormstore.CommitParentRow`, ordered by
  `ParentIndex`), `Tree.BlobIDs`/`Tree.SubtreeIDs` (`gormstore.TreeBlobRow`/
  `TreeSubtreeRow`), `Keyword.ChildIDs` (a query on `KeywordRow.ParentID`).
- `Blob.TreeID` stays on the domain type for JSON-contract compatibility but
  is never populated — content-addressed blobs are reachable from more than
  one tree (`git_tree_blobs` is genuinely many-to-many), so there is no
  single owning tree to report. Unchanged from the entitygraph era, where
  the same field was declared and likewise never set.
- Most `git_impl_*.go` files port with real changes, not mechanical ones:
  `git_impl_branch.go`'s dual-direction self-healing edge lookups
  (`GetBranch`, `listBranchesByRepo`) delete outright — they existed only
  because a `has_branch`/`belongs_to_repository` edge pair could disagree,
  and one `repository_id` column cannot disagree with itself.
  `git_impl_mergerequests.go`'s `listAllMergeRequests` (list every
  repository, then loop) collapses into one filtered query.
  `git_impl_converters.go`'s `allBlobsAtCommit` and `git_impl_fileops.go`'s
  `walkCommitChain`/`git_impl_keywords.go`'s keyword-tree build all move
  from Go-side BFS/recursion to recursive CTEs (`gormstore.BlobsAtCommit`,
  `CommitChainIDs`, `KeywordDescendantIDs`).
- `blobcache.go`, `fileops.go`, `import.go`, `fetchbranch.go` build real git
  objects via `go-git/plumbing/object` — kept the `go-git` object model for
  parsing unchanged; only the persistence calls around it moved from
  `entitygraph.DataManager` to GORM row inserts/queries.
- `git_blobsearch_postgres.go`'s Postgres `tsvector`/`ts_rank` full-text
  search was already raw SQL against the shared `entities` JSONB table, not
  entitygraph — the smallest-touch file in the port. Only the column
  addressing changed (`properties->>'name'` → `name`, no more `type_id =
  'Blob'` predicate since the table *is* blobs now); the tsvector/ts_rank/
  GIN-index mechanism carries over byte-for-byte. See `gormstore.BlobFTSExpr`
  for the one shared expression the query and the migration-time index must
  stay textually identical to.
- Test infrastructure: `testdb_test.go`'s sqlite-in-memory `newTestManager`
  replaces the old hand-maintained `fakeDataManager` — exercising real
  GORM/SQL behavior catches more than a Go map fake ever could. Stays
  internal to package `mwanachamagit` (not `_test`), unlike actor's fully
  external test package, since this repo's tests reach unexported fields
  (`m.db`, `m.tables`) and helpers directly. `schema_test.go` (validated
  `DefaultGitSchema()` via `entitygraph.ValidateSchema`) has no GORM
  equivalent and was deleted outright. `GetNeighborhood`'s BFS tests, which
  used to build fixtures from arbitrary `"Node"`/`"next"` entities (exactly
  the generic-graph capability that no longer exists — see above), now
  build the same shapes from real `Commit` rows linked by
  `git_commit_parents`, a genuinely many-to-many self-referencing relation
  that supports the same arbitrary fan-out/depth.
- A pooled sqlite `:memory:` connection hands concurrent goroutines
  different anonymous in-memory databases unless pinned to one connection
  (`SetMaxOpenConns(1)` in `testdb_test.go`) — actor's test harness never hit
  this since its tests are single-goroutine; this repo's `TestGIT011_*`
  concurrency tests are not.

**`routes/` package added, 2026-09-04 (same day, follow-up).** Mirrors
`mwanachama-backend-api-gateway`'s existing `internal/api/http/
git_handlers*.go` route table exactly — same 43 paths/methods/status codes,
same sentinel-to-status mapping (`routes/wire.go`'s `gitStatusFor`/
`writeGitErr`) — so a mounting process can swap its own handler wiring for
[routes.Routes] without changing its API surface. Unlike
`mwanachama-backend-actor/routes`, there is no `ResourceNames` indirection:
none of git's path nouns (`repos`, `branches`, `merge-requests`, ...) are an
org-configurable label the way actor's `Group`/`"chapters"` is, so the paths
are fixed rather than parameterized. Wiring this into the gateway itself is
a separate, not-yet-done change — see the scope note above.

## Conventions

- Task status lives on
  [documentation/3. implementation/todo.md](documentation/3.%20implementation/todo.md).
- Four-phase `documentation/` layout — see
  [documentation/README.md](documentation/README.md).
