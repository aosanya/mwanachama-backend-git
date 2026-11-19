# Hub copy diff

`mwanachama-backend-git` is the custodian of the git module. The hub (`projects/hub-steward/code/wakala-api/modules/git`) holds a copy made on 2026-11-04. Every change made in that copy must exist here, so the two differ by the import path only (hub issue G78).

## Command

```
scripts/hub-copy-diff.sh [path-to-hub-modules-git]
```

The default path is `../projects/hub-steward/code/wakala-api/modules/git`, relative to this repository's parent folder. The exit code is 1 when the trees differ, 0 when they do not.

## What it does

1. Copies the `*.go` and `*.json` files of this repository (without `cmd/`, `documentation/`, `scripts/`) and of the hub folder into two temporary folders.
2. Rewrites the hub's imports: `wakalaserver/modules/shared/` becomes `github.com/aosanya/mwanachama-backend-shared/` and `wakalaserver/modules/git` becomes `github.com/aosanya/mwanachama-backend-git`.
3. Runs every Go file on both sides through `scripts/hubdiff`, which parses it, drops all comments and prints it with `gofmt`, so comments and layout do not show as differences.
4. Runs `diff -r backend hub`.

## Result of 2026-11-16

Every non-test Go file and every JSON file is identical after the rewrite. What remains is tests, each with its reason:

| Difference | Reason |
|---|---|
| Only here: `git_realgit_integration_test.go`, `git_realgit_shallow_test.go`, `postgres_integration_test.go` | Build tag `integration`: they need a real git binary or a Postgres database, which the hub does not run. |
| Only here: `repository_summary_last_commit_test.go` | G76 test for `last_commit_at`; the hub covers the same through its list tests. |
| Only here: `git_typed_input_errors_test.go` | The red test written for the typed 400 errors (hub 84306abb had none). |
| `repofile_test.go`: the forbidden-import list here also names `jackc/pgx` | The hub module's dependency graph contains pgx through other modules, so the hub drops it from the list. |
| `routes/repository_list_test.go`, `routes/undeclared_cover_test.go`: the spec path | The hub loads `internal/specs/wakala.git.json`; here the tests load `spec/examples/engineering.git.json`. |

## Result of 2026-11-19

The hub has nothing of its own in the module except one test that pins the older refusal of a pushed tag. Everything else the script reports is work present only here. The full table, the hub wiring around the module and the owner question are in [hub-import-drift.md](hub-import-drift.md).
