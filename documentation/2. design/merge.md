# How a merge works

`MergeBranch` merges a source branch into the repository's default branch and keeps both sides. Hub issue G67 replaced the old behaviour, which pointed the default head at the source head and dropped whatever the default branch had gained since the source was cut.

The code is `mergeBranch` and `mergeIntoDefault` in `git_impl_branch.go`, `mergeTrees` and `mergeBase` in `git_impl_merge3.go`, `mergeLines` in `git_impl_merge_lines.go`, `resolveOverlaps` and `storeMergedBlobs` in `git_impl_merge_content.go`, and `writeMergeCommit` in `git_impl_merge_commit.go`.

## Steps

Everything after the first line runs under the merge lock (the ref lock), so the default head is read once and cannot move inside it.

| Step | Case | Result |
|------|------|--------|
| 1 | The source branch has no commits. | The default branch is returned unchanged. |
| 2 | The default head equals the source head. | Nothing changes. |
| 3 | The merge base equals the source head (the source is already contained in the default branch). | Nothing changes. |
| 4 | The default branch has no commits, or the merge base equals the default head. | Fast-forward: the default head moves to the source head. No commit is written. |
| 5 | Otherwise (the branches diverged). | Three-way merge, then a merge commit. If any file conflicts, `ErrMergeConflict` instead, see [Conflicts](#conflicts). |

## Three-way merge

`mergeTrees` takes the file trees of the merge base, the default head and the source head, and decides each path on its own.

| Base, default, source | Merged tree |
|-----------------------|-------------|
| Only the source changed the path (edit, add or delete). | The source side. |
| Only the default branch changed the path. | The default side. |
| Both changed it to the same content. | That content. |
| Both changed it, differently. | An overlapping path, merged line by line, see [Line-level merge](#line-level-merge). |

A path counts as changed when its blob SHA differs from the base, and absence is its own state, so a delete is a change.

The merge commit has two parents, `[default head, source head]`, the message `Merge branch '<source>' into <default>`, and the merged tree. The default head then moves to it with a compare-and-swap on the head that was read before the merge (`advanceBranchHead`). A fast-forward uses the same swap.

## A moved head

If the default head is no longer the one read before the merge, the swap fails and `ErrMergeConcurrencyConflict` is returned. Under the lock this needs a writer outside the module. The merge commit's rows are already written and stay unreferenced; no branch points at them.

## Merge requests

Completing a merge request calls the same `mergeBranch`. `merged_commit_sha` is the SHA of the default branch's new head: the merge commit after a diverged merge, the source head after a fast-forward. The source branch keeps its own head either way.

## Line-level merge

`mergeTrees` returns the overlapping paths and `resolveOverlaps` merges each one with `mergeLines(base, ours, theirs)`, where ours is the default branch and theirs is the source branch (hub issue G68). The source no longer wins an overlapping path.

| Case | Result |
|------|--------|
| Ours equals theirs. | That content. |
| One side equals the base. | The other side. |
| A NUL byte anywhere in the three texts. | Conflict. |
| Otherwise | Each side is diffed against the base with a longest common subsequence into hunks (a range of base lines and the lines that replace it), then the hunks of the two sides are combined. |

When the hunks are combined:

- Hunks that do not overlap or touch are both applied, so edits to different parts of a file merge.
- Hunks that overlap or touch conflict, unless they are identical, which applies once. An edit to the line next to another side's edit therefore conflicts; one untouched line between them does not.
- Two different insertions at the same point conflict; the same insertion on both sides is clean.
- A delete on one side and an edit of the same line on the other conflict.

Trailing newline: if only one side changed whether the text ends in a newline, that side's choice is kept; if both changed it, ours is kept.

## Conflicts

The decision is made for every overlapping path before anything is written, so a refused merge leaves no blob, tree or commit row behind. A path is a conflict when:

- `mergeLines` reports one;
- one side deleted it and the other edited it (modify/delete);
- both added it with different content (add/add), because there is no base;
- any of the three blobs is not utf-8 text, has no stored content, or fails its content check, so it cannot be merged by line.

Only when no path conflicts does `storeMergedBlobs` write one new blob for each merged path (utf-8, the SHA of the merged text), after which the merge commit is written as above.

On a conflict `mergeIntoDefault` returns `*ErrMergeConflict` with `TaskID` set to the source branch name and `ConflictingFiles` sorted. The default head does not move. The error matches the `ErrMergeConflicted` sentinel with `errors.Is`.

## Events

| Outcome | Published |
|---------|-----------|
| Merged, fast-forward or merge commit. | `TopicBranchMerged`. |
| Conflict. | `TopicMergeConflict` once, with `MergeConflictPayload` (`branch_id`, `conflicting_files`, `workflow_run_id`), and no `TopicBranchMerged`. |

## The failed merge request

`CompleteMergeRequest` calls `mergeBranch`, so a conflict ends the request in status `failed`. `error_message` keeps the conflict text, which names the files, and `TopicMergeFailed` is published once, next to the `TopicMergeConflict` from the merge. `TopicMergeCompleted` is not published and the default head stays where it was. A failed request is not open, so completing it again returns `ErrMergeRequestNotOpen`; open a new request after the branch is fixed.

## Over HTTP

`POST /merge-requests/{mrID}/complete` and `POST /branches/{branchID}/merge` answer 409 on a conflict. The body has `error`, `task_id` (the source branch name) and `conflicting_files`. The failed merge request is read with `GET /merge-requests/{mrID}`.

## Resolving a conflict

The default branch is never edited to resolve it. Edit the conflicting files on the source branch, so that they contain the result wanted, and merge again. The merge then succeeds when the edit makes the two sides equal, or when the base has moved, for example after the source was merged once and the default branch was changed since. With the same-line example, writing the default branch's line plus the new line to the source branch makes ours equal theirs on that file and the merge goes through.

## Rebase

G69 is rebase, a separate operation (`git_impl_rebase.go`); it also returns `ErrMergeConflict` when the replay overlaps.

## Merge base

`mergeBase` takes the first commit in the source's history, by depth, that is also an ancestor of the default head. It does not handle criss-cross merges, where two branches merged into each other leave more than one best common ancestor.

## Tests

| File | Covers |
|------|--------|
| `git_impl_merge3_test.go` | `TestMergeTrees` (every case above) and the `TestMergeBase_*` tests: linear history, one fork point, sibling branches, merge commits, no common ancestor. |
| `repofile_merge_commit_test.go` | `TestMergeCommit_*`: two parents and both histories, fast-forward writes no commit, already contained changes nothing, a stale expected head returns `ErrMergeConcurrencyConflict`, overlapping edits return `ErrMergeConflict` and leave the default untouched. |
| `repofile_merge_request_test.go` | `TestMergeRequest_*`: a diverged merge request completes with a two-parent commit, the completed event carries its SHA, the source head is kept, fast-forward adds no commit, sibling branches completed in turn keep every file. |
| `git_impl_merge_lines_test.go` | `TestMergeLines_*`: one-sided edits, top and bottom edits, same line, identical edits, adjacent and separated edits, inserts, deletes against edits, trailing newline, empty base, NUL bytes. |
| `repofile_features_conflict_test.go` | `TestRepoFileFeatures_Conflict_*`: a same-line edit returns the conflict, edits to different lines merge, a merge request fails and publishes `TopicMergeFailed` and `TopicMergeConflict`, resolving on the source branch lets the merge succeed, parallel merges lose no update. |
| `routes/merge_request_conflict_wire_test.go` | Completing a conflicting merge request answers 409 naming the file, and the request reads back `failed` with the text in `error_message`. |

Tests that were red for G67 and pass now (all `TestRepoFileFeatures_`):

- `Merge_DivergedBranchesKeepBothSides`
- `Merge_RequestDivergedFlowKeepsBothSides`
- `Merge_TwoWritersBothLand`
- `Conflict_ParallelMergesLoseNoUpdate`

The G72 tests pass too: `TestRepoFileConcurrency_SecondMergeOfSiblingBranchKeepsFirstMergedCommit`, and the 10 and 30 agent run `TestRepoFileConcurrency_ManyAgentsMergeIntoOneRepositoryFile`, and both run in plain `go test`, no tag.
