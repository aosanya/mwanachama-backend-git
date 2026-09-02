# mwanachama-git (Go)

Open tasks only — 🚀 In Progress · 📋 Not Started · ⏸️ Blocked.
Everything else (completed rows, board context) is in [todo_done.md](todo_done.md).

| Task | Title | Status | Depends on |
|------|-------|--------|------------|
| G3 | Port `schema.go`'s `DefaultGitSchema()` onto `mwanachama-go-shared`'s type defs | 📋 | G2, mwanachama-go-shared#S2 |
| G4 | Port pure-entitygraph impl files as-is: branch, converters, edgelifecycle, graph, keywords, mergerequests, repo, rollback, tag | 📋 | G3, mwanachama-go-shared#S4 |
| G5 | Port go-git-touching impl files (blobcache, fileops, import) — build tree/blob objects via `go-git/plumbing/object`; no wire-protocol transport | 📋 | G4 |
| G6 | Decide scope of fetchbranch / push / index-sync (`syncGitGraph`) — these exist to ingest a real external git remote; default is to skip unless confirmed needed | 📋 | G4 |
| G7 | Blob search: Postgres FTS (`tsvector`/GIN) replacing ArangoSearch BM25, behind the existing `BlobSearcher` seam | 📋 | mwanachama-go-shared#S4 |
| G8 | Wire `events.go`'s topic list to `mwanachama-go-shared`'s `Publisher` interface | 📋 | G2, mwanachama-go-shared#S7 |
| G9 | Unit tests ported/adapted from `*_test.go`; integration tests against real Postgres | 📋 | G4, G5, G7 |
| G10 | Wire into `mwanachama-api-gateway`: new `internal/domain/…` consuming `GitManager` | 📋 | G9 |
