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

## Tags and the push cap in the file handler

`git_smarthttp_file.go` is the handler for a repository kept in its own `.db` file. Two rules were added to its receive-pack path on 2026-11-16 (hub issue ISS-88, TD-1464, owner decisions on questions 3 and 6 of the hub's `git-remote.md`).

### Tag pushes

`pushRefusals` takes a `tagRules` value (`git_smarthttp_file_tag.go`) built from the repository's live Tag rows and its tag patterns, and decides every `refs/tags/` command before any pack is read.

| Command | Result |
|---|---|
| Create a name that has no Tag row, new tip is a commit or an annotated tag of a commit | Accepted; the commit's objects are stored and one Tag row is filed |
| Create a name that already has a Tag row (made with the tools or by an earlier push) | Refused: "tags are made once; delete one with the file tools" |
| Move a tag (old id is not zero) or delete one (new id is zero) | Refused with the same message |
| A name git rejects, or one the repository's tag patterns do not allow | Refused: "not a valid tag name" or "this repository does not allow that tag name" |
| New tip is a tree, a blob, or an annotated tag of one of those | Refused after the pack is unpacked: "a tag must point at a commit" |
| Any ref outside `refs/heads/` and `refs/tags/` | Refused: "only branches can be pushed to the hub" |

One refused ref refuses the whole push, as for branches ("not applied because another ref in this push was refused"). A tag and a branch in one push are both accepted when both are allowed.

An annotated tag is stored as the row the tools already make: `Message` and `TaggerName` come from the tag object, `SHA` is the commit it points at. The file stores commits, trees and blobs only, so the tag object itself is not kept and a clone sees a lightweight tag at the commit. The cost is that a client that verifies the tag object's id will not find it on the remote, and that pushing the same annotated tag again is refused as already made.

The tag row and the tagged commit's objects are written in one transaction behind the same write lock a branch push takes, so a failed push leaves no row. `git_impl_push_tag.go` (`indexPushedTag`) is the older path for an on-disk bare clone and is not used by the file handler.

### The 32 MiB cap

`WithMaxPushSize` and `pushLimit` live in `filesize.go` beside `WithMaxFileSize`; the default is 32 MiB and the cap is on the request body (the command lines plus the pack), not on the stored size.

- A request that declares a `Content-Length` over the cap is answered before any of its body is read.
- Any other body is wrapped in `cappedBody`, which hands the pack reader exactly the cap and then fails. A body of exactly the cap is accepted; one byte more is refused. The pack is therefore never held in memory beyond the cap, where an uncapped push costs 12 to 15 times its size.
- The refusal is a 200 with a receive-pack report whose unpack status and every ref status read "this push is over the 32 MiB limit; push fewer files at a time", so `git push` prints it as `remote unpack failed`. It is not an HTTP error status, because git hides the body of an HTTP error and shows only "HTTP 500". The client still prints its own `RPC failed` line after it.
- Nothing is stored for a refused push: the check runs before `indexAccepted`.
- The probe a client sends for a push over its 1 MiB `http.postBuffer` (the four bytes `0000`) is answered before the cap and does not count against it.

The hub's own router limit on the whole `/git/` route and its read deadline are outside this repository.

### Which cap tests prove the cap

Four tests in `git_smarthttp_file_cap_test.go` are red proofs: they fail on the code before the cap and pass with it. They are the real-git push just over 32 MiB, the body of exactly the cap against one byte over, the body that declares more than the cap, and the chunked body cut off at the cap. Two tests are guards that pass either way and keep the cap from refusing what it should take: the real-git push just under 32 MiB and the real-git push of 20 MiB (their names carry `Guard`). The table with the run results is on the QA page [Git over HTTP from a per-repository file](../4.%20qa/git-push-tags-and-cap.md).
