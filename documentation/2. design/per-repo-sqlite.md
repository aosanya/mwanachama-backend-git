# One SQLite file per repository

Decided in a question-and-answer session on 2026-11-13, for activating git in
the SWE hub (`projects/hub-steward-swe`). Status is below.

## Status

Hub issue G57 builds the open path: decisions 1, 2 and 11.

| Built by G57 | Not part of this change |
|--------------|-------------------------|
| `OpenRepoFile` opens one SQLite file and returns a `RepoFile` with `Path` and `Close`. | The cache of open files. |
| `Provision` creates the fifteen blueprint objects in the file. | |
| The file refuses a second repository row: `InitRepo` returns `ErrRepoAlreadyExists`, and soft-deleted rows count. | |
| The connection waits up to 10000 ms when the file is busy, uses one connection, and keeps the rollback journal (not WAL), so a copy of the lone `.db` file is whole. | |
| The module imports no Postgres driver, guarded by `TestGitPathHasNoPostgresDriver`. | The nightly backup. |
| | Mounting. |
| Hub issue G65 covers every `GitManager` operation on SQLite files; see "Feature coverage on SQLite (G65)" below. Some of its tests are red on purpose. | |

The Postgres blob searcher and the Postgres-only index in `Provision` still
exist but are not on this path.

Hub issue G62 measured many agents on one repository ([results](../4.%20qa/per-repo-sqlite-concurrency.md)) and found `MergeBranch` loses commits (bug G72).

## Catalog wins (G60)

Hub issue G60 builds decision 8.

- `ErrRepositoryNotRegistered` ("repository file has no catalog entry") is returned when a `.db` file has no catalog entry. The file is not opened or touched.
- `RegisteredFileOpener` (`repository_registration.go`) is the one door. `Open(ctx, fileName)` checks registration first. `ListRegistered` lists only the registered `.db` files in the folder and skips the others silently. File names must be plain base names ending `.db`; a path-like name is refused with a different error, so it is never mistaken for an unregistered file.
- Registration is a `RegistrationCheck`. `RegistrationCheckFromCatalog` says a file is registered when some catalog entry's `FileName` equals it. `NewRegisteredRepoFileOpener` opens `RepoFile`s through it, and `RepoFiles` (G58) opens through the same guard.
- The tests show a copied valid file refused, listed nowhere, opening after registration, and refused again once its entry is removed.

Not built: `routes/` does not map `ErrRepositoryNotRegistered` to an HTTP status.

## Create and delete (G61)

Hub issue G61 builds decisions 7 and 8 together: a repository is a file plus a catalog entry, and one call makes or removes both.

- `RepositoryFolder.Create` opens a new `.db` file, runs `InitRepo` in it, then registers the entry. A failure before the entry is written removes the file, so no file is left without an entry.
- `RepositoryFolder.Delete` (and `Purge`) removes the file first, then the entry. A failure keeps the entry, so the repository is still listed and a second `Delete` finishes the job.
- A stray file that a failed cleanup could not remove has no entry, so decision 8 ignores it.

Built; the order of steps, the failure table and the tests are in `3. implementation/repository-folder.md`.

## Catalog entry (G59)

Hub issue G59 builds decisions 7 and 12 on the module side: the entry (file
name, size, last commit time), registering it in `InitRepo`, updating it on
every commit, removing it in `DeleteRepo`, and listing repositories from the
catalog. This is built. What is left is a catalog-backed implementation in
wakala-api (G64) and the file name and size func (G57). See
`3. implementation/repository-catalog.md`.

## Bounded file cache (G58)

Hub issue G58 keeps the number of open repository files bounded, so hundreds of registered repositories do not hold hundreds of open files.

- `RepoFileCache` (`repofilecache.go`) is keyed by file path. `NewRepoFileCache(max, open, closeHandle)` takes the maximum number of open files from its caller and refuses a maximum below 1.
- `With(path, use)` opens the file on demand and keeps a recency list. When the cache already holds the maximum and a new file is wanted, it closes the least recently used idle file first and only then opens the new one, so the open files never exceed the maximum.
- A file in use is never closed. When every slot is in use, the caller waits until one is released.
- A closed file is reopened on demand the next time it is used, so eviction is invisible to callers apart from the reopen.
- `Open()` reports the number of open files. `Close()` waits for the files in use, closes all of them, and every later call returns `ErrRepoFileCacheClosed`.
- `RepoFiles` (`repofiles.go`) is this cache of `*RepoFile` behind the G60 registration guard. `NewRepoFiles(folder, max, registered, open)` is where the caller chooses the maximum, which must be at least 1. `With(ctx, fileName, use)`, `Open()` and `Close()` mirror the cache. The file name check and the catalog check run on every call, before the cache is asked, so an unregistered file never takes a slot.
- The tests (`repofilecache_test.go`, `repofiles_test.go`) register eight files with a maximum of three. They show the data intact after eviction and reopen, and the open files never above three.

## Why

A repository is one `.db` file that can be copied, backed up or handed to
someone else. This is the model Fossil uses.

## Decisions

| # | Decision |
|---|----------|
| 1 | Each repository is stored in its own SQLite file, in one folder per agency. |
| 2 | Repositories are queried with SQLite through the Go module. Postgres is not in the git path, so `sqlite_fdw` and a Postgres-protocol service are not needed. |
| 3 | One server process owns all the files of an agency. |
| 4 | Concurrent writers each work on their own branch and merge into the default branch. A merge that touches the same lines is refused with `ErrMergeConcurrencyConflict` and the agent resolves it. Branches, merge requests and the ref lock already exist. |
| 5 | A repository holds anything the agency produces: source, documents, reports. |
| 6 | There is no file size limit for now. |
| 7 | The list of repositories is catalog entries, one per repository, in a catalog instance of the agency. |
| 8 | Catalog wins: a `.db` file with no catalog entry is ignored until it is registered, and the module refuses to open it. |
| 9 | Nothing is migrated. No repositories exist yet; the first is created after the build, to test it. |
| 10 | The work is filed in its own backlog, with the same steps as the shared Backlog. |
| 11 | A file holds the same fifteen objects as `git.blueprint.json`, created by the same `Provision`, with exactly one repository row. Identical blobs are no longer shared between repositories: each file stores its own copy. |
| 12 | A repository's catalog entry carries the file name, the file size in bytes and the time of the last commit, and nothing else. |
| 13 | The folder of repository files is copied nightly to other storage. A file is copied only while no commit is running on it, so every copy is whole. Up to a day of recent work may be lost. |

## What this costs

- **Cross-repository queries are gone.** Keyword search and graph query run
  inside one repository. A question across repositories has to ask each file.
- **Open files.** The cache of open files (G58) bounds them. What is left is
  that a request can wait when every slot is in use, and that the first use
  after an eviction pays a reopen.
- **One writer per file.** SQLite allows a single writer per repository, so the
  busiest repository sets the write ceiling. Many agents on one repository
  should be measured before it is trusted.
  Measured by G62: about 450 commits plus merges per second without `-race` on
  the test machine, and no `SQLITE_BUSY` at 30 agents.
- **No size limit.** One very large file holds that repository's writer while
  it commits, and the file grows with every version of it. The limit is a
  deliberate later change.
- **Two-step delete.** Removing a repository means removing the file and its
  catalog entry. `RepositoryFolder.Delete` does both in one call (G61). The
  module refuses a file with no entry.
- **Persistent disk.** The files need a volume that outlives the pod, and the
  backup of that volume is ours (decision 13).
- **Copies of identical content.** Decision 11 gives up the cross-repository
  blob sharing that `CLAUDE.md` calls deliberate. `CLAUDE.md` was updated by
  hub issue G64.
- **One process owns the files.** Two replicas writing the same file over a
  shared volume corrupt it, so routing by repository is needed before a second
  replica.

## Not decided

Nothing open. A shorter backup interval is a later change if a day proves too long.

## Feature coverage on SQLite (G65)

Hub issue G65 exercises every `GitManager` operation through `OpenRepoFile`, on real SQLite files in `t.TempDir()`. No Postgres is involved. `repofile_features_inventory_test.go` enumerates the operations through reflection (51 of them), so a new operation fails the build of the suite until a test covers it.

| File | What it covers | Tests |
|------|----------------|-------|
| `repofile_features_helpers_test.go` | Shared helpers. | 0 |
| `repofile_features_inventory_test.go` | The reflection inventory of the 51 operations. | 1 |
| `repofile_features_repo_test.go` | Repository, branch, tag and merge request. | 34 |
| `repofile_features_content_test.go` | Files, log, diff, rollback, keywords, graph, edges and blob search. | 20 |
| `repofile_features_transport_test.go` | Import, fetch, `IndexPushedBranch` and `SmartHTTPHandler` with a real `git push` and `git clone`. | 19 |
| `repofile_features_copy_test.go` | Copy the file to a new folder: everything still reads. | 1 |
| `repofile_features_merge_test.go` | Merging branches. | 9 |
| `repofile_features_conflict_test.go` | Conflicting edits. | 5 |
| `repofile_features_history_test.go` | Log, diff order (entries sorted by path) and rebase. | 7 |

Run it with `go test -run 'RepoFileFeatures_' .`.

### Findings

Some tests fail on purpose because the code lacks the behaviour. They are expected red until the filed hub issues are fixed.

| Issue | Finding | Failing tests |
|-------|---------|---------------|
| G67 | `MergeBranch` overwrites the default head instead of keeping both sides. | `Merge_DivergedBranchesKeepBothSides`, `Merge_TwoWritersBothLand`, `Merge_RequestDivergedFlowKeepsBothSides`, `Conflict_ParallelMergesLoseNoUpdate` |
| G68 | Conflicts are never detected: `ErrMergeConflict` is never returned and `TopicMergeConflict` is never published. | `Conflict_SameLineEditReturnsMergeConflict`, `Conflict_NonOverlappingEditsMergeCleanly`, `Conflict_MergeRequestFailsAndPublishesFailure`, `Conflict_ResolvingOnTheSourceBranchLetsTheMergeSucceed` |
| G69 | There is no rebase operation. | `History_RebaseOperationExists` |
| G71 | `Log` with a path filter returns unrelated commits. | `History_LogByPathAfterMerges` |

Found without a failing test:

- `ErrMergeConcurrencyConflict` cannot be reached by a test, because `mergeBranch` re-reads the default head inside the lock. The compare-and-swap only guards writers outside the module.
- `DeleteFile` writes an empty blob rather than removing the path. This is the documented design.
- In single-repository mode a deleted repository still blocks `InitRepo`, because `ErrRepoAlreadyExists` counts archived rows.
- `PurgeRepo` is the same as `DeleteRepo`: a soft delete.
