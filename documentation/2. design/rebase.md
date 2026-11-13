# How a rebase works

`RebaseBranch` replays a branch's own commits on top of another branch and moves the branch head to the result. Hub issue G69 added it. The operation was missing even though the `MergeBranch` doc in `git.go` already spoke of rebase conflicts.

The request is `RebaseBranchRequest{BranchID, OntoBranchID}` and the result is the updated `models.Branch`. `OntoBranchID` defaults to the repository's default branch. It is declared as the operation `rebase_branch`, `POST /branches/{branchID}/rebase`, with the body `{onto_branch_id}`.

The code is in `git_impl_rebase.go`. It reuses `mergeTrees` and `mergeBase` from `git_impl_merge3.go`, `advanceBranchHead` and `WithMergeLock` from the merge path described in [merge.md](merge.md), and `ErrMergeConflict` from `types.go`.

## Steps

Everything runs under the merge lock (`WithMergeLock`). The branch and the target heads are read once, so neither can move inside the lock.

| Step | Case | Result |
|------|------|--------|
| 1 | The target has no commits, or the branch head equals the target head. | The branch is returned unchanged. |
| 2 | The merge base equals the target head (the branch is already based on the target). | No-op: the branch is returned unchanged. |
| 3 | The merge base equals the branch head (the branch is an ancestor of the target and has no commits of its own). | Fast-forward: the branch head moves to the target head. No commit is written. |
| 4 | Otherwise. | Replay the branch's own commits onto the target head. |

Step 2 is what makes a repeated rebase harmless: after a rebase the merge base is the target head, so the second call changes nothing.

## Replay

The own commits are the first-parent chain from the branch head down to the merge base, oldest first. A merge commit among them is refused with an error; rebasing merge commits is not supported.

For each commit `c` with parent `p`, `mergeTrees(tree(p), tip tree so far, tree(c))` decides each path, exactly as in a merge but with the commit's own parent as the base. The merged tree is the next tip tree. The first tip tree is the target head's tree.

All trees are computed in memory before any row is written, so a conflict in a later commit leaves nothing behind.

## New commits

One new commit is written for each original commit.

| Field | Value |
|-------|-------|
| Message | The original message. |
| Author | The original author name, email and author time. |
| Committer | `mwanachama-bot`, at the time of the rebase. |
| Tree | The tip tree computed for that commit. |
| Parent | The previous new commit. The first one's parent is the target head. |

The original commits stay in the database and are no longer reachable from the branch.

## Conflict

Any overlapping path in any replayed commit returns `*ErrMergeConflict`. `TaskID` is the branch name and `ConflictingFiles` is the sorted union of the overlapping paths of the first conflicting commit. The branch head is unchanged and no rows were written.

Line-level merging inside a file belongs to G68 and applies here without further work, because the rebase calls `mergeTrees`. Until G68 lands, two edits to one file count as overlapping.

## The swap

The branch head moves to the last new commit with the compare-and-swap in `advanceBranchHead`, expecting the head read at the start. If the head has moved, `ErrMergeConcurrencyConflict` is returned and the new commit rows stay unreferenced, the same as for a merge. A fast-forward uses the same swap.

## Not done

- The target branch is never changed.
- Doc edges are not replicated onto the new commits.
- No `TopicBranchMerged` event is published, because nothing is merged.

## Tests

| File | Covers |
|------|--------|
| `git_impl_rebase_test.go` | `TestRebaseBranch_*`: a clean rebase, a conflict leaves the head unchanged, a no-op and a repeated rebase, fast-forward, rebase onto a named branch, an unknown branch. |
| `repofile_features_history_test.go` | `TestRepoFileFeatures_History_RebaseOperationExists` was red before G69 and passes now. |
