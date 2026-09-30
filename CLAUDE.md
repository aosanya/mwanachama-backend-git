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
scope here** — the v1 `Backend` and `storage/arangodb/storer.go` (the
go-git `storage.Storer` built on entity CRUD) were never ported, and never
need to be; `openOrInitBareRepo`/`SmartHTTPHandler` below read/write a real
on-disk bare git repo directly, no entity-graph storer in between.

**Real git Smart HTTP wire-protocol `push` support landed — G6, see
`git_impl_push.go`/`git_smarthttp.go`.** This was flagged for a long time
as "not a default, a new scoped decision" (the note used to read almost
exactly that); the decision was made and it's built: `GitManager.
SmartHTTPHandler()` returns a real `http.Handler` a caller mounts anywhere,
serving ref advertisement, upload-pack (clone/fetch), and receive-pack
(push) against `openOrInitBareRepo`'s on-disk bare clone (auto-`git init
--bare` on first contact, same first-push-creates-the-remote behaviour a
real git host has) — ported from `CodeValdGit`'s own
`internal/server/githttp.go`, adapted single-tenant (this repo dropped the
Agency concept entirely) and read straight off go-git's own storer rather
than the dropped v1 `Backend.OpenStorer` abstraction. `IndexPushedBranch`
is implemented for real too, reusing git_impl_fetchbranch.go's own
commit/tree-walk *shape* but made idempotent per (repo, SHA) — unlike
FetchBranch's one-shot clean-slate walk, this runs on every push to a
branch that may already be fully indexed, so it checks for an existing row
by SHA before creating one rather than duplicating the whole history/tree
on every push. Verified end-to-end with real pushes (go-git client-side,
the actual wire protocol, not a mocked call) in
`git_smarthttp_test.go` — including a second push proving the idempotency
holds and a delete-only push (`git push --delete`, which carries no
packfile and needs its own path since go-git's `ReceivePack` fails on an
empty one).

**A commit walk that creates `Commit` rows is only half the job** — it must
also write the commit_parent join rows, or the history it just
indexed is unreachable. `Log` resolves history solely through
`CommitChainIDs`' recursive CTE over that table, so an unlinked
tip reports a one-commit history however many commits were actually
indexed. `git_impl_push.go`'s `linkCommitParents` does this for the push
path (added 2026-09-16, after review caught `walkNewCommits` omitting it);
`ParentIndex` carries git's own parent order, so a merge parent is never
mistaken for a first parent. **`git_impl_fetchbranch.go`'s `walkCommitsOnly`
still has this omission** — imported/fetched branches report a one-commit
history today, confirmed by test. That's board row G12, deliberately not
folded into G6's fix since it predates it and raises its own backfill
question for already-indexed repos.

**The push path's reuse-or-create check keys on `(sha, path)`, never SHA
alone** (`findRowIDBySHAAndPath`, fixed 2026-09-16). Path is not a property
of the content a SHA identifies — the same bytes legitimately sit at more
than one path (a repeated `LICENSE`, an empty `__init__.py`, a vendored
file) — so the SHA-only key G6 originally shipped filed every occurrence
under whichever path was indexed first and gave the rest no row at all,
leaving them unreadable through `ReadFile`. Confirmed by test before fixing,
and note this was a G6-only regression, not inherited: `FetchBranch`'s
`upsertBlobMetadataWithID` unconditionally `Create`s a row per path and was
always correct here. Adding the path to the key costs nothing G6 wanted —
re-pushing the same file at the same path still reuses its row, which is the
idempotency property `TestSmartHTTP_SecondPushIsIdempotentForUnchangedTree`
guards.

Worth keeping straight when touching any of this: **content-addressed reuse
across repositories is fine and deliberate.** Identical SHA means identical
bytes, the way forks share objects on a real git host. It was briefly
suspected as a bug and checked — there is no failure behind it, and
`Commit`/`Tree`/`Blob` rows are not repository-scoped by design. **Not done as part of G6**: mounting `SmartHTTPHandler()`
anywhere in `mwanachama-backend-api-gateway`'s own router — that's its own
follow-up, not implied by "the wire protocol works," the same way
`mwanachama-backend-accounting`'s ledger core and its gateway wiring
(W9) were two separate steps.

Also dropped from the original: `proto/`, `cmd/server`, `internal/server`
(gRPC `GitServiceServer`), `internal/registrar` (CodeValdCortex Cross
heartbeat) — `mwanachama-backend-api-gateway` runs as one service and imports this
package directly.

## Objects and routes are declared, not written

**The tables come from `git.blueprint.json`, not from Go structs.** Fifteen
objects, every field with its type and description, reached through
`Blueprint()`, `LoadSpec(path)` and `ParseSpec(raw)` — never through
`spec.Load`, or the roled objects arrive with no fields. A domain spec names
which object fills each role, what the domain calls it, which table it lands
in and its own indexes; it may not restate a type, a description or a rule.
`spec/examples/` ships two: `engineering` and `chambers`, a law firm
versioning contract drafts, so domain-neutrality is exercised rather than
asserted.

**The route table comes from `git.operations.json`.** Forty of the
forty-three addresses; `routes/routes.go` is the sentinel map, the
`dispatch.Table` and the builder ladder, and there are no handlers. The other
three are in `routes/undeclared.go` — see
[documentation/2. design/routes.md](documentation/2.%20design/routes.md) for
which and why.

**`Provision(db, *spec.Spec)` is the whole storage story.** `spec.Migrate`
emits the DDL; the blob full-text GIN index follows it, because `spec` has no
expression index. `cmd/ddl` prints both so a spec can be read as SQL before
it is trusted. There is no `AutoMigrate`.

**A column is found by field name, never by json tag.** `SubmittedBy` is
`submitted_by`. The tag is a presentation choice and gets this wrong where it
hurts. **Every declared column is written on every write**, because a map
missing a key means "leave it alone" to an update.

**The spec and the types are checked against each other when the manager is
built.** A declared column with no field, or a field with no column, fails in
`NewGitManager` rather than dropping a value on every write. The six fields
that are genuinely held elsewhere carry `spec:"-"` — see the decision record.

**A table is `<instance>_hashOf(<mount>)_hashOf(<module>_<object>)`.** Only
the instance stays readable. Assert on `RawNameFor` in tests, never on a
physical name, and exclude `%_spec_table_names` from anything counting
tables.

**IDs are minted explicitly.** The uuid used to come from a GORM
`BeforeCreate` hook on the row structs; nothing reaches such a hook now, so
`ensureID` runs at each create. Every create that skipped it wrote an empty
id and collided on the second row.

**Three things are held exactly as they were and must not be "fixed" in
passing**: `sha` carries no unique index, `branch.status` and
`blob_keyword_tag.signal` are strings rather than enums. The reasons are in
[documentation/1. requirements/declared-domain-decisions.md](documentation/1.%20requirements/declared-domain-decisions.md).

## What is superseded

| Was | Now | Board row |
| --- | --- | --- |
| `gormstore/` — 13 row structs, `*ToRow`/`*FromRow`, `Migrate`, `AutoMigrate` | `git.blueprint.json` + `specstore`; the row structs had become the domain types field for field | G17 |
| `tables.go` — `TableNames`, `DefaultTableNames`, `Migrate` re-exports | `Provision(db, *spec.Spec)`, physical names from the spec | G17 |
| `gormstore/queries.go` | `queries.go` in the root package, on the spec's names | G17 |
| 13 route files, 43 decode-call-encode handlers, `routes/wire.go`'s `gitStatusFor`/`writeGitErr`/`readJSON`/`writeJSON` | `git.operations.json` + `dispatch.Table`; `httpwire` for the rest | G17 |
| `validDocEdges` | the `blob_reference.name` enum, read through `checksField` | G17 |
| `fileEntryJSON`/`commitEntryJSON`/`fileDiffJSON` shaping helpers | json tags on `FileEntry`/`FileDiff`, and `CommitEntry.MarshalJSON` for the RFC3339 timestamp | G17 |
| per-field doc comments in `models/` | the blueprint's `description` fields | G17 |


## Porting notes

- `git.go`'s `GitManager` interface (method set unchanged) and the domain
  types in `mwanachama-backend-git/models` (`Repository`, `Branch`,
  `MergeRequest`, `Tag`, `Commit`, `Tree`, `Blob`, `Keyword`, `ImportJob`,
  `FetchBranchJob`) port field-for-field from the entitygraph era. Request/
  filter/graph DTOs (`CreateRepoRequest`, `MergeRequestFilter`, `GraphNode`/
  `GraphEdge`/`GraphResult`, `QueryGraphRequest`, etc.) are not persisted
  entities and stay in the root package's `models.go`, mirroring actor's
  split (only true domain types move to `models/`).
- Three domain fields are no longer stored columns, only derived at read
  time and written via a companion join-row builder at write time:
  `Commit.ParentIDs` (the `commit_parent` object, ordered by
  `parent_index`), `Tree.BlobIDs`/`Tree.SubtreeIDs` (`tree_blob`/
  `tree_subtree`), `Keyword.ChildIDs` (a query on `KeywordRow.ParentID`).
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
  from Go-side BFS/recursion to recursive CTEs (`BlobsAtCommit`,
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
  GIN-index mechanism carries over byte-for-byte. See `BlobFTSExpr`
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

## Code comments

Write code with no comments. Not one-liners above a function, not section
banners, not doc comments on exported symbols, not "why" notes next to a
tricky line. A name, a type, or a smaller function carries it instead.

Anything that genuinely needs explaining goes in this repo's `documentation/`
folder, under the phase it belongs to (`1. requirements`, `2. design`,
`3. implementation`, `4. qa`) — never inline.

**Why:** inline prose drifts out of sync with the code, duplicates what
`documentation/` already owns, and buries the explanation where nobody
looking for it will search.

**How to apply:**

- New code ships without comments. If a line seems to need one, rename or
  split until it doesn't.
- Touching code that already has comments: strip the ones in the code you are
  changing. Do not sweep untouched files unless asked.
- If the reasoning matters, add or update the matching `documentation/` page
  in the same change and leave nothing behind in the source.
- Machine-read directives are not comments and stay: build tags, `//go:embed`,
  `//go:generate`, linter pragmas (`//nolint`, `// eslint-disable-next-line`,
  `// ignore:`), license headers, codegen "do not edit" banners, and generated
  files as a whole.
- Commit messages, PR descriptions, and test names carry the narration that
  used to go in comments.

This rule is repeated verbatim in every mwanachama repo's `CLAUDE.md` so that
it reaches sessions that do not load this machine's user-level config —
scheduled cloud routines, other machines, and other agent harnesses.
