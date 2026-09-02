# mwanachama-git (Go)

Open tasks only — 🚀 In Progress · 📋 Not Started · ⏸️ Blocked.
Everything else (completed rows, board context) is in [todo_done.md](todo_done.md).

| Task | Title | Status | Depends on |
|------|-------|--------|------------|
| G6 | Real git-push wire protocol for `IndexPushedBranch` (Smart HTTP receive-pack indexing) — currently a stub returning `ErrPushIndexingNotImplemented` so `*gitManager` compiles; default is to skip unless confirmed needed | 📋 | G4 |
| G9 | Unit tests ported/adapted from `*_test.go` (fidelity check against the original CodeValdGit suite) — `make test-pg`'s real-Postgres round-trip + FTS coverage already exists from G7 | 📋 | G4, G5, G7 |
| G10 | Wire into `mwanachama-api-gateway`: new `internal/domain/…` consuming `GitManager` | 📋 | G9 |
