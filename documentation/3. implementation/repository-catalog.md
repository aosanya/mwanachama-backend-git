# Repository catalog (module side)

Hub issue G59, decisions 7 and 12 of `2. design/per-repo-sqlite.md`. The
module keeps the catalog entry of each repository and lists repositories from
it. The catalog itself is not in this module.

## The entry

`RepositoryCatalogEntry` (`repository_catalog.go`) has five fields: `ID` and
`Name` (the repository's), `FileName`, `SizeBytes` and `LastCommitAt`
(RFC 3339 text).

The stored document has exactly three keys and nothing else:

```json
{"file_name": "widgets.db", "size_bytes": 12288, "last_commit_at": "2026-11-13T10:00:00Z"}
```

- `Document()` returns that JSON.
- `ParseRepositoryCatalogDocument(id, name, document)` reads it back and
  refuses any other key. The id and name come from the catalog row, not the
  document.

## The interface

`RepositoryCatalog` has five methods:

| Method | Does |
|--------|------|
| `Register(ctx, entry)` | Adds an entry. `ErrRepositoryAlreadyCatalogued` if the id is taken. |
| `Update(ctx, id, sizeBytes, lastCommitAt)` | Sets size and last commit time. `ErrRepositoryNotCatalogued` if there is no entry. |
| `List(ctx)` | Returns every entry. |
| `Lookup(ctx, id)` | Returns one entry. `ErrRepositoryNotCatalogued` if there is none. |
| `Remove(ctx, id)` | Deletes the entry. `ErrRepositoryNotCatalogued` if there is none. |

`MemoryRepositoryCatalog` (`repository_catalog_memory.go`,
`NewMemoryRepositoryCatalog()`) is the in-memory fake the tests use. It lists
entries sorted by name, then id.

## Turning it on

```go
WithRepositoryCatalog(catalog RepositoryCatalog, file RepositoryFile) Option
```

`RepositoryFile` is `func(repoID string) (fileName string, sizeBytes int64, err error)`.
It tells the module which file holds a repository and how big it is now. Both
are set on the manager together (`repository_catalog_option.go`). With no
catalog, nothing below happens and the module behaves as before.

## When the entry is written

- `InitRepo` registers the entry after the repository row and its default
  branch exist: file name and size from the `RepositoryFile`, no last commit
  time yet. If registering fails, `InitRepo` returns the error.
- Every commit path goes through `advanceBranchHead`, which updates the entry
  with the file's current size and the commit's time (`CommittedAt`, else
  `CreatedAt`). A failed update, including a missing entry or a failing
  `RepositoryFile`, never fails the commit.
- `DeleteRepo` (and `PurgeRepo`, its alias) removes the entry after the soft
  delete. A repository whose entry is already gone is deleted without error.
- Register and update are skipped when no `RepositoryFile` was given. Remove is
  skipped only when there is no catalog.

## Listing

When a catalog is set, `ListRepositories` and `ListRepositorySummaries` read
it (`git_impl_repo_list_catalog.go`):

- One repository per catalog entry, in the catalog's order.
- Id and name come from the entry. `UpdatedAt` is the entry's last commit time
  when it has one, otherwise the row's own.
- The other fields (description, default branch, created time) come from the
  repository row.
- A row without an entry is not listed. A deleted repository is not listed
  (its entry is removed, and rows marked deleted are ignored).
- An entry with no row is still listed, with only the id, name and time. In
  summaries it has an empty default branch and default branch id.
- With no catalog the old query over the repository rows is used.

The tests are `git_impl_repo_catalog_test.go` and
`git_impl_repo_list_catalog_test.go`.

## What other work must supply

- Each API: a `RepositoryCatalog` backed by the agency's catalog mount,
  storing the three-key document through `Document()` and
  `ParseRepositoryCatalogDocument`. `RoutedGitManager` (`routing-layer.md`)
  takes it, builds the `RepositoryFile` func with `RepositoryFileInFolder` and
  turns on `WithRepositoryCatalog` for every file it opens for use.

## Not wired yet

- No catalog-backed `RepositoryCatalog` in this module, only the fake. It
  would import the private catalog module.

## The repository list's last commit time

Hub issue G76. `models.RepositorySummary`, what `ListRepositorySummaries`
returns and `forms_git_repository_list` shows, has a `last_commit_at` field
(RFC 3339 text, left out of the JSON when empty). The field is the type's;
what fills it is the manager.

The shared-database manager (`gitManager`) leaves it empty on purpose, with or
without a catalog turned on. Its summaries come from the repository and branch
rows, which hold no commit time, and the catalog entry's `LastCommitAt` feeds
only `UpdatedAt` in `ListRepositories`. A caller must read an absent
`last_commit_at` as "not reported", not as "no commit".
`TestListRepositorySummariesFromTheSharedDatabaseLeavesLastCommitAtEmpty` holds
this.

A manager that lists a folder of per-repository files fills the field from the
catalog entry's `LastCommitAt`, which each merge keeps current. That manager is
`RoutedGitManager` (`routing-layer.md`).
