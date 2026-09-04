// Package mwanachamagit provides Git-like versioned content for
// mwanachama-frontend-kazi: repositories, branches, commits, trees, blobs,
// merge requests, tags, and a keyword-tagging documentation layer, backed by
// GORM-mapped relational tables. Decided as a follow-up to the storage
// migration already done in mwanachama-backend-actor (2026-09-04): domain
// logic AND storage both live in this package, imported directly into the
// gateway process — no separate service, no gRPC, no proto.
//
// Layout:
//   - models/    — domain types (Agency, Repository, Branch, MergeRequest,
//     Tag, Commit, Tree, Blob, Keyword, ImportJob, FetchBranchJob); callers
//     use models.Repository etc. directly, no re-export in this package
//   - gormstore/ — GORM row structs, row<->domain conversion, migration,
//     and the raw-SQL helpers backing the graph-query methods
//   - doc.go (this file), tables.go — table-name/migrate wrappers
//   - git.go                — GitManager interface, gitManager struct, constructor
//   - git_impl_repo.go       — Repository lifecycle
//   - git_impl_branch.go     — Branch CRUD, merge, advanceBranchHead
//   - git_impl_tag.go        — Tag CRUD
//   - git_impl_mergerequests.go — MergeRequest CRUD/lifecycle
//   - git_impl_rollback.go   — workflow-run rollback
//   - git_impl_converters.go — row<->domain converters, allBlobsAtCommit
//   - git_impl_fileops.go    — WriteFile/ReadFile/DeleteFile/ListDirectory/Log/Diff
//   - git_impl_edgelifecycle.go — DR-010 branch-scoped edge lifecycle
//   - git_impl_keywords.go   — Keyword CRUD, branch-scoped edge CRUD
//   - git_impl_graph.go      — GetNeighborhood/SearchByKeywords/QueryGraph
//   - git_impl_import.go     — async repository import
//   - git_impl_fetchbranch.go — async on-demand branch fetch
//   - git_impl_blobcache.go  — lazy blob-content hydration
//   - git_impl_blobsearch.go, git_impl_push.go — thin wrapper / stub
//   - git_blobsearch_postgres.go — Postgres tsvector/ts_rank BlobSearcher
//   - errors.go, events.go, types.go, models.go — sentinel errors, event
//     topics/payloads, and the request/filter/graph DTOs (not persisted
//     entities, so they stay in this package rather than models/)
//   - routes/    — this package's own HTTP surface (decode/call GitManager/
//     encode), mirroring mwanachama-backend-api-gateway's existing
//     git_handlers*.go route table
//
// See this repo's CLAUDE.md for the entitygraph->GORM migration record.
package mwanachamagit
