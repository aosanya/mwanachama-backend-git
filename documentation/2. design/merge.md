# How a merge works

`MergeBranch` merges a source branch into the repository's default branch and keeps both sides. Hub issue G67 replaced the old behaviour, which pointed the default head at the source head and dropped whatever the default branch had gained since the source was cut.

The code is `mergeBranch` and `mergeIntoDefault` in `git_impl_branch.go`, `mergeTrees` and `mergeBase` in `git_impl_merge3.go`, and `writeMergeCommit` in `git_impl_merge_commit.go`.

## Steps

Everything after the first line runs under the merge lock (the ref lock), so the default head is read once and cannot move inside it.

| Step | Case | Result |
|------|------|--------|
| 1 | The source branch has no commits. | The default branch is returned unchanged. |
| 2 | The default head equals the source head. | Nothing changes. |
| 3 | The merge base equals the source head (the source is already contained in the default branch). | Nothing changes. |
| 4 | The default branch has no commits, or the merge base equals the default head. | Fast-forward: the default head moves to the source head. No commit is written. |
| 5 | Otherwise (the branches diverged). | Three-way merge, then a merge commit. |

## Three-way merge

`mergeTrees` takes the file trees of the merge base, the default head and the source head, and decides each path on its own.

| Base, default, source | Merged tree |
|-----------------------|-------------|
| Only the source changed the path (edit, add or delete). | The source side. |
| Only the default branch changed the path. | The default side. |
| Both changed it to the same content. | That content. |
| Both changed it, differently. | An overlapping path. |

A path counts as changed when its blob SHA differs from the base, and absence is its own state, so a delete is a change.

The merge commit has two parents, `[default head, source head]`, the message `Merge branch '<source>' into <default>`, and the merged tree. The default head then moves to it with a compare-and-swap on the head that was read before the merge (`advanceBranchHead`). A fast-forward uses the same swap.

## A moved head

If the default head is no longer the one read before the merge, the swap fails and `ErrMergeConcurrencyConflict` is returned. Under the lock this needs a writer outside the module. The merge commit's rows are already written and stay unreferenced; no branch points at them.

## Merge requests

Completing a merge request calls the same `mergeBranch`. `merged_commit_sha` is the SHA of the default branch's new head: the merge commit after a diverged merge, the source head after a fast-forward. The source branch keeps its own head either way.

## Overlapping paths

For now the source side wins an overlapping path, and `mergeTrees` already returns the list of those paths.

- G68 turns that list into `ErrMergeConflict` and adds line-level merging inside a file. Until then `TestRepoFileFeatures_Conflict_NonOverlappingEditsMergeCleanly` stays red, because two edits to different lines of one file still count as overlapping.
- G69 is rebase, a separate operation.

## Merge base

`mergeBase` takes the first commit in the source's history, by depth, that is also an ancestor of the default head. It does not handle criss-cross merges, where two branches merged into each other leave more than one best common ancestor.

## Tests

| File | Covers |
|------|--------|
| `git_impl_merge3_test.go` | `TestMergeTrees` (every case above) and the `TestMergeBase_*` tests: linear history, one fork point, sibling branches, merge commits, no common ancestor. |
| `repofile_merge_commit_test.go` | `TestMergeCommit_*`: two parents and both histories, fast-forward writes no commit, already contained changes nothing, a stale expected head returns `ErrMergeConcurrencyConflict`, overlapping edits take the source side. |
| `repofile_merge_request_test.go` | `TestMergeRequest_*`: a diverged merge request completes with a two-parent commit, the completed event carries its SHA, the source head is kept, fast-forward adds no commit, sibling branches completed in turn keep every file. |

Tests that were red for G67 and pass now (all `TestRepoFileFeatures_`):

- `Merge_DivergedBranchesKeepBothSides`
- `Merge_RequestDivergedFlowKeepsBothSides`
- `Merge_TwoWritersBothLand`
- `Conflict_ParallelMergesLoseNoUpdate`

The G72 tests pass too: `TestRepoFileConcurrency_SecondMergeOfSiblingBranchKeepsFirstMergedCommit`, and the 10 and 30 agent run `TestRepoFileConcurrency_ManyAgentsMergeIntoOneRepositoryFile`, and both run in plain `go test`, no tag.
