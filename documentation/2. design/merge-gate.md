# Who may merge (G26)

A merge request used to carry its author as a free-text name, and
`MergeBranch` asked nobody's permission. A host that wants "only the role
the RACI matrix names Accountable may merge" now has a seam for it.

## The seam

`WithMergeGate(gate)` is an `Option` on `NewGitManager`. A `MergeGate` is
`func(ctx, caller, repositoryID, targetBranchID string) (bool, error)`:
the host answers whether that caller may merge into that branch. This
module never learns what a role is; the RACI lookup is the host's.

- `CompleteMergeRequest(ctx, mrID, mergedBy)` asks the gate before
  merging. `mergedBy` is bound `{"from": "caller"}` in
  `git.operations.json`, so no request body can name who merges.
- `MergeBranch` asks the gate too, with an empty caller, since it has no
  caller to bind; a gate that refuses an empty caller closes that path.
- A refusal is `ErrMergeNotPermitted`, mapped to **403**. The merge request
  stays `open`: a refusal is not a failed merge, so it is neither marked
  `failed` nor announced on `TopicMergeFailed`.
- A gate's own error is returned wrapped, and is a 500 like any other
  unmapped error.
- With no gate set, every merge proceeds exactly as before.

## Who merged

`merge_request.merged_by` (blueprint and `models.MergeRequest.MergedBy`)
records the caller once a merge succeeds. It stays empty for a merge no
caller made.

## Left for the host

`mwanachama-wakala-api` has to supply the gate: resolve the repository and
branch to the agency Objective whose Accountable role may merge, and the
caller to that role.
