# Git refactored onto the actor template

**Date:** 2026-09-04 · **Surface:** git · **Status:** ✅ done

This repo stored Repository/Branch/Commit/Tree/Blob/Keyword/etc. through
`mwanachama-backend-shared/entitygraph.DataManager` — a generic,
runtime-versioned graph engine, one shared `entities` table and one shared
`relationships` table for everything. `actor` had already moved off that
same engine earlier the same day (its own CLAUDE.md, 2026-09-04): GORM
storage, a `models/`+`gormstore/` package split, and its own `routes/`
package. User asked for git to follow, fully — but this repo's domain is
much larger (10 entity types vs. actor's 3) and had a capability actor's
never needed: three methods (`GetNeighborhood`/`SearchByKeywords`/
`QueryGraph`) doing generic, any-type/any-direction graph exploration
against entitygraph's shared relationships table. Storage migration first,
then a `routes/` package added as a same-day follow-up ask.

## What was built

- `models/` — 11 domain types (`Agency`, `Repository`, `Branch`,
  `MergeRequest`, `Tag`, `Commit`, `Tree`, `Blob`, `Keyword`, `ImportJob`,
  `FetchBranchJob`) plus `time.go`.
- `gormstore/` — 16-table schema (`tables.go`: 9 node tables + 4 join
  tables), row structs + `*ToRow`/`*FromRow` converters per entity,
  `edges.go` (`BlobKeywordTagRow`/`BlobReferenceRow` — fixed-shape,
  both-ends-typed join tables replacing `tagged_with`/`references`), and
  `queries.go`'s recursive CTEs (`BlobsAtCommit`, `CommitChainIDs`,
  `KeywordDescendantIDs`) and typed edge catalogue (`NeighborhoodEdges`)
  replacing entitygraph's generic traversal.
- All 13 `git_impl_*.go` files rewritten onto `*gorm.DB`.
  `NewGitManager(dm, sm, pub, locker, searcher)` became
  `NewGitManager(db, tables, pub, locker, searcher)` — `GitManager`'s
  method set is unchanged.
- `git_blobsearch_postgres.go` repointed at real `blobs` table columns
  (`gormstore.BlobFTSExpr` shared between the search query and the
  migration-time GIN index, so they can't drift apart).
- Test suite: `testdb_test.go` (sqlite in-memory, `SetMaxOpenConns(1)` —
  the `TestGIT011_*` concurrency tests need one shared connection, not a
  pool handing goroutines separate anonymous in-memory databases).
  `GetNeighborhood`'s BFS fixtures rebuilt on real `Commit` rows linked by
  `git_commit_parents` instead of the old generic entities (a real
  many-to-many self-reference supports the same arbitrary fan-out/depth
  the old fixtures did). `schema.go`, `schema_test.go`,
  `fake_datamanager_test.go`, `integration_pg_test.go` deleted.
- `routes/` — 43 HTTP routes across 11 files, mirroring
  `mwanachama-backend-api-gateway`'s existing `git_handlers*.go` route
  table exactly (same paths, methods, status codes, sentinel-to-status
  mapping).
- `CLAUDE.md` rewritten with the full decision record.

## Key decisions

- **Flattened fully to explicit typed FK columns and join tables — no
  generic edges table survives.** User's call, explicit: "follow actor to
  the latter" over keeping a narrow generic edges table alongside the
  flattened domain tables for the sake of `GetNeighborhood`/`QueryGraph`.
  Cost, precisely: any edge label outside the enumerated 16 shapes (or
  outside `blob_references`'s constrained `Name` set) is unrepresentable
  without a schema migration going forward.
- **`Commit`/`Tree`/`Blob` `sha` gets no unique index**, even though
  entitygraph declared `UniqueKey: []string{"sha"}` — this repo only ever
  called `CreateEntity`, never `UpsertEntity` (the one call that actually
  applied it), so the declared uniqueness was already unenforced. Adding a
  real index now would break previously-succeeding writes, not port a
  guarantee that existed.
- **Scope stops at this repo**, mirroring actor's own precedent: the
  gateway's `cmd/server/stores.go` (which constructs `NewGitManager`
  inline — there is no `git_instances.go` isolation layer the way actor
  has `user_instances.go`) is left broken. The gateway was independently
  broken already, for unrelated reasons (`mwanachamacomm` package), before
  this session touched anything.
- **`routes/` skips actor's `ResourceNames` indirection.** None of git's
  path nouns (`repos`, `branches`, `merge-requests`, ...) are an
  org-configurable label the way actor's `Group`/`"chapters"` override is
  — paths are fixed literals, chosen to match the gateway's existing route
  table so it can act as a drop-in replacement later.

## Validation

`go build ./...`, `go vet ./...`, and `go test ./...` all pass (58+ tests,
sqlite in-memory backend), including `go build -tags=integration ./...`
(the Postgres-only integration file compiles). The integration suite
itself was not run against a live Postgres this session — no
`POSTGRES_URL` was set.

## Follow-ups

- **Gateway wiring is broken** — `cmd/server/stores.go`'s
  `memoryStores`/`postgresStores` construct the old
  `NewGitManager(dm, sm, pub, locker, searcher)` signature and need
  repointing at `(db, tables, pub, locker, searcher)`. Blocks the gateway
  build until done (on top of the pre-existing `mwanachamacomm` break).
- Gateway's `git_handlers*.go` are not yet repointed at the `routes/`
  package's 43 routes.
- `postgres_integration_test.go` (`-tags integration`) should get a real
  run against a live Postgres before trusting the blob-search
  tsvector/GIN-index rewrite end to end — `scripts/local-postgres.sh`
  stands one up.
