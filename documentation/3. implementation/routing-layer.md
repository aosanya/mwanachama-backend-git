# Routing layer (plan)

Hub issue G79, owner decision 2026-11-16. The code that sends each repository
request to its own SQLite file moves out of the two APIs into this module, so
both APIs wire it and neither copies it. This page is the plan; it is turned
into a description of what was built when the last part lands.

## Sources

| Source | Holds |
|--------|-------|
| `projects/hub-steward/code/wakala-api/internal/agencies`, the copy that is ahead | `gits_files.go`, `gits_files_routed.go`, `repository_file.go`, `repository_catalog.go`, `gits.go`, their tests, and `last_commit_at` (hub 0adab528, 6e31847e), the file-backed `SmartHTTPHandler` and `RebaseBranch` (hub 7069cc7b) |
| `mwanachama-wakala-api` `internal/agencies`, read only | commits 39b6248 (catalog adapter, file helpers), 1d2d969 (routed manager), 4655b1c (last commit test). Its routed manager has no `RebaseBranch`, so its package does not build today, and its `SmartHTTPHandler` answers 501 |

The two copies of the routed manager differ in three places only: the import
paths, `RebaseBranch`, and `SmartHTTPHandler` with `splitRepositoryPath`. The
port takes the hub copy.

## The catalog interface already exists

The interface the plan needs is `RepositoryCatalog` in
`repository_catalog.go` (see `repository-catalog.md`), added for hub issue
G59. It has `Register`, `Update`, `List`, `Lookup` and `Remove`, and its entry
carries the id, name, file name, size in bytes and last commit time. It
imports nothing private. `MemoryRepositoryCatalog` is its in-memory
implementation, so the routing tests run against it with no catalog module.

No second interface is defined. The four operations the routing layer calls
(list, get, put, delete) are `List`, `Lookup`, `Register` plus `Update`, and
`Remove`.

| Method | The routing layer uses it to |
|--------|------------------------------|
| `List(ctx)` | find a file by repository name, probe every file for an id, list repositories, answer the stray-file check. The order is the order of every listing, so an implementation returns name then id |
| `Lookup(ctx, id)` | find the file of a repository id. `ErrRepositoryNotCatalogued` becomes `ErrRepoNotInitialised` |
| `Register(ctx, entry)` | through `RepositoryFolder.Create` |
| `Update(ctx, id, sizeBytes, lastCommitAt)` | through `WithRepositoryCatalog` on each file opened for use, on every commit |
| `Remove(ctx, id)` | through `RepositoryFolder.Delete` |

## What moves

| From the API | To this module | Name here |
|--------------|----------------|-----------|
| `repositoryFolderGit`, `newRepositoryFolderGit` (`gits_files.go`) | `routed_git_manager.go` | `RoutedGitManager`, `NewRoutedGitManager(folder, maxOpen, spec, catalog)` |
| `fileOwners`, the scopes (`repo`, `named`, `first`, `eachFile`, `locate`, `owning` and the per-kind probes) | `routed_git_scope.go` | unexported |
| every `GitManager` method of `gits_files_routed.go` | `routed_git_methods.go` | methods of `RoutedGitManager`, `RebaseBranch` included |
| `SmartHTTPHandler`, `splitRepositoryPath` | `routed_git_smarthttp.go` | `RoutedGitManager.SmartHTTPHandler` |
| `ErrImportNeedsRepositoryFile` | `errors.go` | same name, exported |
| `RepositoryFileAt`, `RepositoryFileInFolder` (`repository_file.go`) | `repository_file_stat.go` | same names, exported |
| the 16 `TestGitFolder_*` tests that do not read the registry, the file helper tests, the last commit test, the Smart HTTP test | `routed_git_*_test.go`, `repository_file_stat_test.go` | rewritten on `MemoryRepositoryCatalog` and the engineering spec |

`RoutedGitManager` is `GitManager` (a compile-time assertion), plus `Close()`.
It opens each file for use with `WithRepositoryCatalog(catalog,
RepositoryFileInFolder(folder, catalog))` and opens the file `Create` writes
without a catalog, so `InitRepo` does not register the entry a second time.
It composes the existing `RepositoryFolder`, `RepoFiles` and
`RegistrationCheckFromCatalog`; nothing of those is copied.

## What stays in each API

| Stays | Why |
|-------|-----|
| The adapter from the catalog mount to `RepositoryCatalog` (`NewRepositoryCatalog`, `repository_catalog.go`, its paging and its tests) | It imports the private catalog module. Each API keeps its own, on its own mount |
| `RepositoriesMount` and `buildFolderGitManager`: look the catalog manager up, build the folder `<GIT_REPOSITORY_FOLDER>/<instance table slug>`, call `NewRoutedGitManager` | The Registry and the instance are the API's |
| `GIT_REPOSITORY_FOLDER` and `GIT_OPEN_FILES`, `gitFolderFromEnv`, `WithGitRepositoryFolder`, `closeGitManager` | Environment and cache eviction belong to the process |
| The spec lookup `gitSpecOf` and the `wakala.git` declaration | The declaration registry is the API's |
| The gate on the git module for the forms git routes and tools (`forms_git_routes.go`, `forms_git_gate_test.go`) | An HTTP and module-gating rule, not routing |
| The HTTP route that mounts `SmartHTTPHandler` | The API's router |
| The tests that build a `Registry`: the unset folder keeps the shared-database manager, the environment selects the folder, the registry reads the environment, an instance keeps its files in its own subfolder, and the catalog adapter tests | They need the Registry and the catalog module |

## Not in this move

The shared-database manager is unchanged. The APIs' own switch to this layer,
and the deletion of their copies, is part of the tag switch of hub issue
ISS-105 (TD-1509 for the hub, TD-1511 for `mwanachama-wakala-api`), after the
owner confirms the tags. Neither API and neither `go.mod` is touched here.

## Parts

Each part is one commit on `master`, builds with `go build ./...` and
`go vet ./...`, and names the commit it comes from. Each part writes its tests
first and sees them fail before the code.

| Part | Holds | From |
|------|-------|------|
| 1 | `RepositoryFileAt`, `RepositoryFileInFolder` and their tests | wakala-api 39b6248 |
| 2 | `RoutedGitManager`, its constructor, `Close`, the scopes, repository create, delete, purge, list, get by id and by name, the commit that updates the entry, the stray file, reopening, the open file bound, and the last commit time in summaries | wakala-api 1d2d969; hub 6e31847e and 0adab528 |
| 3 | every call routed by branch, merge request, tag, keyword, job or ref id, `RebaseBranch`, `ImportRepo` refused | wakala-api 1d2d969; hub 7069cc7b for `RebaseBranch` |
| 4 | `SmartHTTPHandler` over the named file, the `GitManager` assertion, the clone and push test | hub 7069cc7b |
| 5 | this page rewritten as a description of what was built; the "Not built" lines of `repository-folder.md` and `repository-catalog.md` updated | |

## Checks

`go build ./...`, `go vet ./...` and `go test -race -timeout 40m ./...` on a
clean `git archive` export with `../mwanachama-backend-shared` beside it, and
`go list -deps ./...` showing no private module.
