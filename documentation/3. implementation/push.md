# Push indexing

`git_impl_push.go` implements `GitManager.IndexPushedBranch` (G6, the git push wire protocol) and the repository-storer resolution the Smart HTTP handler (`git_smarthttp.go`) needs to accept an inbound push.

## Origin and why the walks differ from FetchBranch

The code is ported from the original CodeValdGit `git_impl_push.go` onto this repository's own GORM tables. It reuses the pattern of `walkCommitsOnly` and `upsertTreeMetadataWithEdges` in `git_impl_fetchbranch.go` rather than calling them: both are written for FetchBranch's one-time walk over a stub branch with no Commit, Tree or Blob rows, and they create every row they visit. `IndexPushedBranch` runs on every push to a branch that may already be fully indexed, so its walks look up an existing row by SHA first and stop or skip instead of creating again. Without that, every later push would duplicate the branch's whole history and file tree.

## IndexPushedBranch

- `oldSHA` is the previous branch tip (all zeros for a new branch) and `newSHA` the new tip. Both are the values `git-receive-pack` reported for the ref, so the method never guesses them.
- Idempotent per (repository, SHA): a commit, tree or blob whose SHA already has a row is left alone.
- `refs/tags/*` is indexed as a Tag through `indexPushedTag`.
- Only `refs/heads/*` is a branch. Any other namespace is stored in the bare repository and not indexed; without the check it would be filed as a Branch row named after the whole ref, because the name is derived by trimming a `refs/heads/` prefix that is not there.

## walkNewCommits and linkCommitParents

`walkNewCommits` walks backward from the tip, breadth first, inserting a Commit row for every commit that has none, and stops a path at `oldSHA` or at a commit an earlier push or fetch already indexed (its parents were walked when it was first indexed). It returns the number of rows created.

`linkCommitParents` writes the `git_commit_parents` rows for the batch. Without them the pushed history is unreachable: the recursive CTE in `CommitChainIDs`, the only way `Log` walks back from a tip, follows that table and nothing else, so an unlinked tip reports one commit however many the push carried. A parent's row id comes from the same batch when the parent was new too, and from one SHA lookup when it was already indexed. `ParentIndex` keeps git's own parent order even if a link is skipped, so a merge parent is never taken for a first parent.

## Tree and blob reuse keys on (sha, path)

`upsertTreeIdempotent` and `upsertBlobIdempotent` are `upsertTreeMetadataWithEdges` and `upsertBlobMetadataWithID` made safe to call on every push: a Tree or Blob already carrying a row for the same (SHA, path) is reused, so an unchanged subtree or file pushed again costs one SELECT.

`findRowIDBySHAAndPath` keys on both because path is not a property of the content a SHA identifies. The same bytes legitimately sit at more than one path (a repeated LICENSE, an empty `__init__.py`, a vendored file). Keying on SHA alone filed every occurrence under the path indexed first and left the rest with no row, so `ReadFile` could not find them.

## Branch rows and the clone folder

`findOrCreatePushedBranch` resolves a branch ref to a Branch row id and creates the row when the branch was pushed first and never created through `CreateBranch`.

`pushClonesRoot` is the persistent directory that push-target bare clones live under. It is distinct from the scratch directories of `cloneRootDir` (`git_impl_import.go`), which are recreated empty on every call and are wrong for a repository that must still be there at the next push.

`openOrInitBareRepo` resolves a repository name to its on-disk bare clone and runs `git init --bare` the first time a repository is pushed to. An InitRepo-created repository has a row but no clone yet, and neither has a repository that `git_smarthttp.go` creates on first contact from a client that never called InitRepo.
