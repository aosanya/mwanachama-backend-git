# mwanachama-git (Go)

Open tasks only — 🚀 In Progress · 📋 Not Started · ⏸️ Blocked.
Everything else (completed rows, board context) is in [todo_done.md](todo_done.md).

| Task | Title | Status | Depends on |
|------|-------|--------|------------|
| G6 | `IndexPushedBranch` (Smart HTTP receive-pack indexing) — real git-push wire protocol; default is to skip unless confirmed needed | 📋 | G4 |
| G7 | Blob search: Postgres FTS (`tsvector`/GIN) replacing ArangoSearch BM25, behind the existing `BlobSearcher` seam | 📋 | mwanachama-go-shared#S4 |
| G9 | Unit tests ported/adapted from `*_test.go`; integration tests against real Postgres | 📋 | G4, G5, G7 |
| G10 | Wire into `mwanachama-api-gateway`: new `internal/domain/…` consuming `GitManager` | 📋 | G9 |
