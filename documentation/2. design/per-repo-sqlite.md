# One SQLite file per repository

Decided in a question-and-answer session on 2026-11-13, for activating git in
the SWE hub (`projects/hub-steward-swe`). Nothing here is built yet.

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
- **Open files.** Hundreds of repositories need a bounded cache of open files.
- **One writer per file.** SQLite allows a single writer per repository, so the
  busiest repository sets the write ceiling. Many agents on one repository
  should be measured before it is trusted.
- **No size limit.** One very large file holds that repository's writer while
  it commits, and the file grows with every version of it. The limit is a
  deliberate later change.
- **Two-step delete.** Removing a repository means removing the file and its
  catalog entry. The module refuses a file with no entry.
- **Persistent disk.** The files need a volume that outlives the pod, and the
  backup of that volume is ours (decision 13).
- **Copies of identical content.** Decision 11 gives up the cross-repository
  blob sharing that `CLAUDE.md` calls deliberate, so that note needs updating
  when this is built.
- **One process owns the files.** Two replicas writing the same file over a
  shared volume corrupt it, so routing by repository is needed before a second
  replica.

## Not decided

Nothing open. A shorter backup interval is a later change if a day proves too long.
