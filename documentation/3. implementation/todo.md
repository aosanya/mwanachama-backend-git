# mwanachama-backend-git (Go)

Open tasks only — 🚀 In Progress · 📋 Not Started · ⏸️ Blocked.
Everything else (completed rows, board context) is in [todo_done.md](todo_done.md).

| Task | Title | Status | Depends on |
|------|-------|--------|------------|
| G12 | `FetchBranch`/`ImportRepo` history is unreachable: `walkCommitsOnly` (`git_impl_fetchbranch.go`) materialises `Commit` rows but never writes the `git_commit_parents` join rows `Log` walks, so an imported branch reports a one-commit history. Confirmed by test — a three-commit import gave 3 Commit rows, 0 CommitParent rows, `Log` = 1 entry. Same defect G6's follow-up fixed on the push side (`linkCommitParents`, `git_impl_push.go`), which is the fix to reuse. Predates G6; open question is whether already-indexed repos need a backfill or only new fetches are fixed | 📋 | — |
