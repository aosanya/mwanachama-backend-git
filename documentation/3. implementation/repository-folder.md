# Repository folder (create and delete)

Hub issue G61, decisions 7 and 8 of `2. design/per-repo-sqlite.md`. A
repository is a `.db` file plus a catalog entry. `RepositoryFolder`
(`repository_folder.go`) creates and deletes the two together, so a caller
never makes or removes one without the other.

## The folder

```go
NewRepositoryFolder(folder string, catalog RepositoryCatalog, open func(path string) (*RepoFile, error), opts ...RepositoryFolderOption) *RepositoryFolder
```

| Part | Is |
|------|----|
| `folder` | The agency's folder of `.db` files. `Create` makes it if it is missing. |
| `catalog` | The `RepositoryCatalog` (see `repository-catalog.md`) that holds the entries. |
| `open` | Opens one file as a `RepoFile`, for example through `OpenRepoFile`. |
| `withFileRemover` | Test-only option that replaces `os.Remove`, so a test can make file removal fail. |

It has three methods: `Create(ctx, CreateRepoRequest)`, `Delete(ctx, repoID)`
and `Purge(ctx, repoID)`. `Purge` is `Delete`.

One mutex serialises `Create` and `Delete` on a folder, so two calls never
interleave their file and catalog steps.

## Create

The steps run in this order, and the first one that fails stops the rest.

| Step | Does | On failure |
|------|------|------------|
| 1 | Lists the catalog and refuses a name that is already catalogued with `ErrRepoAlreadyExists`. | Nothing was written. |
| 2 | Makes the folder if needed, then picks a new id plus `.db` as the file name. | Nothing was written. |
| 3 | Opens the file with `open`. | No file, no entry. |
| 4 | Runs `InitRepo` inside the file. | The file is closed, then removed. |
| 5 | Closes the file. | The file is removed. |
| 6 | Reads the file's size with `os.Stat`. | The file is removed. |
| 7 | `Register`s the entry: repository id and name, the file name, the size. | The file is removed. |

The id and name in the entry are the ones `InitRepo` returned. The entry has no
last commit time yet.

## Delete

| Step | Does |
|------|------|
| 1 | Looks the entry up. No entry returns `ErrRepoNotInitialised` and touches nothing. |
| 2 | When `WithRepoFiles` was given, evicts the file's cached handle (waiting for users of it), so no open handle outlives the file. |
| 3 | Removes the `.db` file, then its `-journal` file. A file that is already missing counts as removed. |
| 4 | Removes the entry. An entry that is already gone counts as removed. |

The file goes first, so an entry never points at a file that was deleted
without anyone being told.

## What each failure leaves behind

| Failure | File | Entry | Returned |
|---------|------|-------|----------|
| Create: open or `InitRepo` fails | None | None | The error. |
| Create: the entry write fails | Removed | None | The register error. |
| Create: the entry write fails and the removal also fails | Stray file stays | None | Both errors joined. The stray file has no entry, so decision 8 ignores it. |
| Delete: file removal fails | Kept | Kept | The removal error. The repository is untouched and can be deleted again. |
| Delete: entry removal fails after the file is gone | Gone | Kept | The removal error. A second `Delete` finds the file missing and removes the entry. |

## Tests

`repository_folder_test.go` uses a temporary folder, `MemoryRepositoryCatalog`
wrapped so `Register` and `Remove` can fail, and the engineering spec.

| Test | Shows |
|------|-------|
| `TestRepositoryFolderCreate_MakesTheFileAndOneCatalogEntryForIt` | One `.db` file and one entry; the entry has the file name, the repository id and name, and the file's real size; the file lists the one repository. |
| `TestRepositoryFolderCreate_AFailedEntryWriteLeavesNoFileAndNoEntry` | A failed register returns its error and leaves no file and no entry. |
| `TestRepositoryFolderCreate_AFailedEntryWriteWhoseCleanupAlsoFailsReportsBothErrors` | When the removal also fails, the error matches both, and there is still no entry. |
| `TestRepositoryFolderCreate_ANameAlreadyCataloguedIsRefusedWithoutASecondFile` | A second create of a name returns `ErrRepoAlreadyExists`; one file and one entry remain. |
| `TestRepositoryFolderCreate_AFailedOpenLeavesNoFileAndNoEntry` | A failed open returns its error and leaves no file and no entry. |
| `TestRepositoryFolderDelete_RemovesTheFileAndTheEntry` | After `Delete` the file and the entry are gone. |
| `TestRepositoryFolderDelete_AFailedFileRemovalKeepsTheEntryAndTheFile` | A failed file removal keeps both. |
| `TestRepositoryFolderDelete_AFailedEntryRemovalKeepsTheEntryAndASecondDeleteCompletes` | The file is already gone, the entry stays, and a second `Delete` clears it. |
| `TestRepositoryFolderDelete_AnUnknownIDIsRefusedAndTouchesNothing` | An unknown id returns `ErrRepoNotInitialised`; the existing file and entry stay. |
| `TestRepositoryFolderPurge_RemovesTheFileAndTheEntryLikeDelete` | `Purge` does what `Delete` does. |
| `TestRepositoryFolderPurge_LeavesNothingOfTheRepositoryBehind` | After a write through the `RepoFiles` cache and a planted `-journal`, `Purge` leaves no `.db`, no `-journal`, no entry and no cached handle, and the same name can be created again (G74). |
| `TestRepositoryFolderDelete_LeavesTheOtherRepositorysFileAndEntryIntact` | Deleting one repository leaves the other's file, entry and data whole. |

## Not built

- A stray file left by a failed cleanup is not swept. It stays unregistered
  and ignored until someone removes it.

`RoutedGitManager` (`routing-layer.md`) calls `Create`, `Delete` and `Purge`
for `InitRepo`, `DeleteRepo` and `PurgeRepo`. Neither API calls it yet (hub
issue ISS-105).
