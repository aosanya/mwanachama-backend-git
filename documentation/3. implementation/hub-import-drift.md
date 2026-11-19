# Hub import drift

What stands between the hub's in-tree git module (`projects/hub-steward/code/wakala-api/modules/git`) and this repository, measured on 2026-11-19 so the hub can drop its copy and import this module. Method: `scripts/hub-copy-diff.sh` (see [hub-copy-diff.md](hub-copy-diff.md)), then a read of the hub's wiring around the module.

## Summary

Nothing in the hub's module is missing here. After the import path is rewritten, every hub Go and JSON file is either identical to its counterpart here or is the older version of a file this repository has since changed. The hub has no file of its own in the module (no non-Go, non-JSON file either) and no commit after the point this repository already covers: the hub's last change to the module is 2026-11-16, and this repository's work of the same day sits on top of it.

Merging the hub into this repository therefore moves no code. The one hub-only item is a test that pins behaviour this repository replaced; it is not moved and goes to the owner as a question.

Pinned counts (`vocabulary_test.go`, the route and operation counts in `routes/`) are the same on both sides and are not changed.

## Only in the hub

| Item | Disposition |
|---|---|
| `TestDecidedGitPush_APushedTagIsRefusedWithThePlainMessageAndNoTagAppears` in `git_smarthttp_file_decided_test.go`: pushes a tag over git and expects the refusal "only branches can be pushed to the hub". | Not moved. It pins the older behaviour that a pushed tag is refused. This repository accepts a pushed tag (see "Differing beyond the import path"), so the test would fail here. Owner question 1. |

## Only in this repository

The hub's copy was taken before these existed. The hub receives all of them by importing this module; none is moved.

| Item | Files | What it is |
|---|---|---|
| Tag push and push size cap | `git_smarthttp_file_tag.go`, `git_smarthttp_file_tags_test.go`, `git_smarthttp_file_cap.go`, `git_smarthttp_file_cap_test.go`, `filesize.go` (`WithMaxPushSize`, `pushLimit`, `pushTooLargeMessage`), `git.go` (`maxPush` field), `errors.go` (`ErrImportNeedsRepositoryFile`) | A tag can be created over git push under tag rules; a push over 32 MiB (default, settable per manager) is refused with a plain message. Described in [4. qa/git-push-tags-and-cap.md](../4.%20qa/git-push-tags-and-cap.md). |
| Repository file stat | `repository_file_stat.go`, `repository_file_stat_test.go` | `RepositoryFileAt` and `RepositoryFileInFolder`, which the hub keeps in `internal/agencies/repository_file.go` (same code). |
| Routed manager | `routed_git_manager.go`, `routed_git_owners.go`, `routed_git_repository.go`, `routed_git_branches.go`, `routed_git_graph.go`, `routed_git_requests.go`, `routed_git_scope.go`, `routed_git_smarthttp.go` and their `routed_git_*_test.go` files | `RoutedGitManager`, the manager that sends each repository request to the file of its repository. The hub holds the same code as `repositoryFolderGit` in `internal/agencies/gits_files.go` and `gits_files_routed.go`. Compared after renaming, the two are the same line for line apart from `ErrImportNeedsRepositoryFile`, which the hub declares in `internal/agencies` and this repository declares in `errors.go`. |
| Integration tests behind the `integration` build tag | `git_realgit_integration_test.go`, `git_realgit_shallow_test.go`, `postgres_integration_test.go` | They need a real git binary or a Postgres database. |
| Other tests | `repository_summary_last_commit_test.go`, `git_typed_input_errors_test.go` | Last-commit time on a repository summary; typed 400 errors. |
| Commands | `cmd/ddl`, `cmd/repobackup` | Not part of the hub's copy. |

## Differing beyond the import path

| Operation | Hub | This repository | Disposition |
|---|---|---|---|
| `git push` of a tag to a repository file | Refused per ref with "only branches can be pushed to the hub". | Accepted when the tag rules allow it; refused with the rule's reason otherwise. | Kept as is here. Owner question 1. |
| `git push` larger than the cap | No cap in the module. | Refused with "this push is over the N MiB limit; push fewer files at a time". | Kept as is here; the hub gains it on import. |
| `ImportRepo` on a repository folder | The hub's `internal/agencies` returns its own `ErrImportNeedsRepositoryFile`. | The same error value text, declared by the module. | Kept as is here. When the hub imports, its declaration is removed and its test uses the module's value. |
| `repofile_test.go` forbidden-import list | Names `gorm.io/driver/postgres` and `lib/pq`. | Also names `jackc/pgx`. | Kept as is here: the stricter list. The hub's dependency graph contains pgx through other modules, so the hub's own copy of this test is dropped with its module folder. |
| `routes/repository_list_test.go`, `routes/undeclared_cover_test.go` | Load the hub's `internal/specs/wakala.git.json`. | Load `spec/examples/engineering.git.json`. | Kept as is here. The two spec files are identical except for `domain` (`forms` against `engineering`) and `instance` (`wakala` against `eng`). |
| `git_smarthttp_file_record.go` | A refused push is reported by its status only. | A push refusal that carries its own message reports that message. | Kept as is here; part of the tag and cap work above. |

## Hub wiring that is not part of the module

Listed, not moved. These stay in the hub and keep importing the module after the move.

| Hub file | What it does |
|---|---|
| `internal/specs/wakala.git.json` | The hub's copy of the git operations under the `forms` domain and `wakala` instance. |
| `internal/agencies/gits.go`, `gits_files.go`, `gits_files_routed.go` | Builds the git manager per agency, reads `GIT_REPOSITORY_FOLDER`, and holds `repositoryFolderGit`, which duplicates this repository's `RoutedGitManager`. After the import it is replaced by `NewRoutedGitManager`; its tests (`gits_files_test.go`, `gits_files_http_test.go`, `gits_files_last_changed_test.go`) are covered by the `routed_git_*_test.go` files here. |
| `internal/agencies/repository_catalog.go` | `catalogRepositories`, the adapter that keeps a repository's catalog entry in the hub's catalog module. It depends on the catalog module and stays in the hub. |
| `internal/agencies/repository_file.go` | `RepositoryFileAt`, `RepositoryFileInFolder`; the same code is in `repository_file_stat.go` here, so the hub's copy is removed. |
| `internal/api/http/forms_git_routes.go`, `git_route_auth.go`, `git_route_serve.go`, `mcp_tools_forms_git.go` and their tests | The HTTP layer, the sign-in check on git over HTTP, the mounting of the module's routes under an agency, and the tool definitions. |
| Permission catalogue and tool filters (`internal/api/http/mcp_tool_actions.go` and the published agency mounts test) | Which roles may call which git tool. |

## Owner questions

1. Should a tag pushed over git be accepted in the hub, as this repository does, or refused as the hub does today? The hub's test pins the refusal; this repository's tests pin the acceptance under tag rules and the 32 MiB cap. Importing this module changes the hub's behaviour to acceptance, and the hub's test goes with its module folder.
