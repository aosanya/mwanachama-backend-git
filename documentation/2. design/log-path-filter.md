# Log path filter

`Log` with `LogFilter{Path}` lists a commit only when the file at that path differs from every parent of the commit. The comparison is the blob SHA at the path, and absence from the commit's tree is its own state. This is git's rule for dropping commits that are the same as a parent on that path.

The rule lives in `commitTouchesPath` and `pathStateAt` in `git_impl_fileops.go`.

## What is listed

| Commit | Listed |
|--------|--------|
| Root commit that has the file | yes |
| Root commit without the file | no |
| Commit that adds the file | yes |
| Commit that changes the file | yes |
| Commit that deletes the file | yes |
| Commit that leaves the file as it was | no |
| Merge commit that differs from all of its parents at the path | yes |
| Merge commit that took the file unchanged from one side | no |

A path that never existed gives an empty list.

`DeleteFile` writes an empty blob rather than removing the path, so its commit is listed because the blob SHA changed, not because the path became absent.

## Limit and failures

- `LogFilter.Limit` counts commits walked from the head, before the path filter applies. A limit of 5 with a path returns at most 5 commits, and fewer when some of the five left the file alone.
- A failed blob lookup, or a failed read of the commit's parents, keeps the commit rather than hiding it.

## Tests

- `TestRepoFileFeatures_History_LogByPathAfterMerges` in `repofile_features_history_test.go`: after two feature merges, each file's log holds only its own adding commit, and a missing path gives an empty list.
- `TestWriteReadDeleteFile` in `git_impl_fileops_test.go`: the log for `README.md` holds the one commit that wrote it.
