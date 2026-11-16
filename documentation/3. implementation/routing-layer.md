# Routing layer

Hub issue G79, owner decision 2026-11-16. The code that sends each repository
request to its own SQLite file lives in this module, so both APIs wire it and
neither copies it. `RoutedGitManager` is a `GitManager` over a folder of
repository files and a `RepositoryCatalog`.

## Where it came from

| Source | Held |
|--------|------|
| `projects/hub-steward/code/wakala-api/internal/agencies`, the copy that was ahead | `gits_files.go`, `gits_files_routed.go`, `repository_file.go`, hub 787ea282, 0adab528 (`last_commit_at`), 7069cc7b (`RebaseBranch`, file-backed `SmartHTTPHandler`) |
| `mwanachama-wakala-api` `internal/agencies`, read only | 39b6248 (file helpers), 1d2d969 (routed manager), 4655b1c (last commit test). Its routed manager has no `RebaseBranch`, so its package does not build, and its `SmartHTTPHandler` answers 501 |

The port took the hub copy. The hub commit 6e31847e named in the issue is in
no local hub repository; the `last_commit_at` change is 0adab528.

## The catalog interface

No new interface was needed. `RepositoryCatalog` (`repository_catalog.go`, see
`repository-catalog.md`) already had `Register`, `Update`, `List`, `Lookup`
and `Remove` over an entry of id, name, file name, size and last commit time,
and imports nothing private. `MemoryRepositoryCatalog` implements it for the
tests. Each API implements it with its own catalog mount.

`List` must return entries by name, then id: every listing and the choice of
the repository for a root keyword follow that order.

## Use

```go
NewRoutedGitManager(folder string, maxOpen int, s *spec.Spec, catalog RepositoryCatalog) (*RoutedGitManager, error)
```

| Part | Is |
|------|----|
| `folder` | The folder of `.db` files, one per agency |
| `maxOpen` | Files kept open at once (`RepoFiles`); at least 1 |
| `s` | The git spec each file is provisioned from |
| `catalog` | Where the entries live |

`Close()` closes every open file. Files opened for use carry
`WithRepositoryCatalog(catalog, RepositoryFileInFolder(folder, catalog))`, so
a commit updates the entry's size and last commit time. The open used by
`RepositoryFolder.Create` carries no catalog, otherwise `InitRepo` would
register the entry and `Create` would then fail on the second `Register`.

## Files

| File | Holds |
|------|-------|
| `routed_git_manager.go` | the type, the constructor, `Close`, `openFiles` |
| `routed_git_owners.go` | the remembered owner of an id: a mutex-guarded map, emptied past 4096 entries and on every repository delete |
| `routed_git_scope.go` | the scopes that find a file: by repository id, by name, the first, every file, and by owner probe for a branch, merge request, tag, keyword, import job or fetch job |
| `routed_git_repository.go` | create, delete, purge, list and summaries, get by id and name |
| `routed_git_branches.go` | branch calls and file calls (write, read, delete, list, log), rebase |
| `routed_git_requests.go` | merge requests, tags, rollback by workflow run |
| `routed_git_graph.go` | diff, import (refused), keywords, edges, graph, fetch, pushed-branch indexing, blob search |
| `routed_git_smarthttp.go` | `SmartHTTPHandler` and the `GitManager` assertion |
| `repository_file_stat.go` | `RepositoryFileAt`, `RepositoryFileInFolder` |

## How each call finds its file

| Calls | File found by |
|-------|---------------|
| `InitRepo` | `RepositoryFolder.Create` |
| `DeleteRepo`, `PurgeRepo` | `RepositoryFolder.Delete`; the remembered owners are cleared |
| `ListRepositories`, `ListRepositorySummaries` | every catalog entry in catalog order; each file is asked for its own row. `UpdatedAt` and the summary's `LastCommitAt` come from the entry |
| `GetRepository`, `GetBranchByName`, `CreateBranch`, `ListBranches`, `ListBranchesFiltered`, `CreateMergeRequest`, `CreateTag`, `ListTags`, `FetchBranch` | repository id, through the catalog entry. An id with no entry is `ErrRepoNotInitialised` |
| `GetRepositoryByName`, `SearchBlobs`, `IndexPushedBranch` | repository name, through the catalog |
| `GetBranch`, `DeleteBranch`, `MergeBranch`, `RebaseBranch`, `WriteFile`, `ReadFile`, `DeleteFile`, `ListDirectory`, `Log`, `CreateEdge`, `DeleteEdge`, `GetNeighborhood`, `SearchByKeywords`, `QueryGraph` | branch id: each file is probed with `GetBranch`, the owner is remembered |
| `GetMergeRequest`, `CompleteMergeRequest`, `CloseMergeRequest` | merge request id, probed with `GetMergeRequest` |
| `GetTag`, `DeleteTag` | tag id, probed with `GetTag` |
| `GetKeyword`, `GetKeywordTree`, `UpdateKeyword`, `DeleteKeyword` | keyword id, probed with `GetKeyword` |
| `GetImportStatus`, `CancelImport`, `GetFetchBranchStatus` | job id, probed with the status call |
| `Diff` | each file is tried; `ErrRefNotFound` means not here |
| `ListMergeRequests` | the repository id when the filter has one, otherwise every file, merged and cut to the limit |
| `RollbackByWorkflowRun` | every file; the counts are summed |
| `CreateKeyword` | the file of the parent keyword; a root keyword goes to the first repository in the catalog, and with none it is `ErrRepoNotInitialised` |
| `ListKeywords` | the file of the parent keyword; with no parent, the roots of every file |
| `ImportRepo` | refused with `ErrImportNeedsRepositoryFile` |
| `SmartHTTPHandler` | `/<name>.git/...` is served from the file of the repository with that name: 404 for an unknown name, 400 for a path with no name |

A remembered owner whose file has since lost its entry is forgotten and the
files are probed again. A file in the folder with no catalog entry is never
opened (`ErrRepositoryNotRegistered`, decision 8).

## What stays in each API

| Stays | Why |
|-------|-----|
| The adapter from the catalog mount to `RepositoryCatalog` (`NewRepositoryCatalog`, its paging, its tests) | It imports the private catalog module |
| `RepositoriesMount` and the build of the manager: look the catalog manager up, call `NewRoutedGitManager(<GIT_REPOSITORY_FOLDER>/<instance table slug>, ...)` | The Registry and the instance are the API's |
| `GIT_REPOSITORY_FOLDER`, `GIT_OPEN_FILES`, `gitFolderFromEnv`, `WithGitRepositoryFolder`, `closeGitManager` | Environment and cache eviction belong to the process |
| `gitSpecOf` and the `wakala.git` declaration | The declaration registry is the API's |
| The gate on the git module for the forms git routes and tools (`forms_git_gate_test.go`) | An HTTP and module-gating rule |
| The HTTP route that mounts `SmartHTTPHandler` | The API's router |
| The tests that build a `Registry`: an unset folder keeps the shared-database manager, the environment selects the folder, the registry reads the environment, an instance keeps its files in its own subfolder | They need the Registry |

## Tests

`routed_git_*_test.go` run on `MemoryRepositoryCatalog`, the engineering spec
and a temporary folder (`routed_git_fixture_test.go`). They are the 15 routing
tests of 1d2d969 that do not read the Registry, the last commit test of
4655b1c, a rebase test (the wakala-api copy has none), the Smart HTTP clone
and push test of hub 7069cc7b, and a test for a path with no repository name.
`repository_file_stat_test.go` holds the three file helper tests.

## Not done here

- Neither API uses it yet and neither deletes its own copy. That is part of
  the tag switch of hub issue ISS-105 (TD-1509 for the hub, TD-1511 for
  `mwanachama-wakala-api`), after the owner confirms the tags.
- The routed manager passes no event publisher, ref locker or blob searcher to
  `OpenRepoFile`, as the APIs did not.
- Every call checks registration by listing the catalog, one query per entry.
- Keywords carry no repository id, so a keyword belongs to one file and an edge
  to a keyword of another repository is not supported.
- `ImportRepo` is refused: it writes its repository row from a goroutine and
  never registers the catalog entry.
