# Git over HTTP from a per-repository file: tag pushes and the push cap, 16 November 2026

Hub issue ISS-88, todo TD-1464. Each test starts `RepoFile.GitHTTPHandler` on a temp per-repository `.db` file and drives it with a real `git` binary, or posts a receive-pack body straight to the handler for the byte-exact cases. Run with `go test -race -count=1 -run 'TestGitPushTags|TestGitPushCap' .` from the repository root.

The rule each row pins is on [Push indexing](../3.%20implementation/push.md), section "Tags and the push cap in the file handler".

## Tag pushes (`git_smarthttp_file_tags_test.go`)

| Case | Result |
|---|---|
| A lightweight tag on a new commit | Accepted; the Tag row holds the commit; a fresh clone lists the tag, resolves it to that commit and reads the tagged file |
| A lightweight tag on the default branch's tip | Accepted; row filed |
| An annotated tag | Accepted; the row keeps the message and tagger; a fresh clone sees the tag at the commit |
| A tag and a new branch in one push | Both accepted |
| A forced push that moves an existing tag | Refused with "tags are made once; delete one with the file tools"; the row keeps its commit |
| A push that re-creates a name made with the file tools | Refused with the same message; the row is unchanged |
| A push that deletes a tag | Refused with the same message; the row stays |
| A tag on a tree | Refused with "a tag must point at a commit"; no row |
| A push to `refs/notes/review` | Refused with "only branches can be pushed to the hub" (also pinned in `git_smarthttp_file_decided_test.go`) |

## Push cap (`git_smarthttp_file_cap_test.go`)

Run on a clean export of 025a42b with only the test files and the `maxPush` field in `git.go` added, the four tests marked red fail and the two guards pass; on the fix all six pass.

| Test | Case | Kind | Result |
|---|---|---|---|
| `TestGitPushCap_APushJustOverThirtyTwoMiBIsRefused...` | A real `git push` of a random file 64 KiB over 32 MiB | Red proof | Refused; git prints "this push is over the 32 MiB limit; push fewer files at a time"; no branch row; the file is not in a new clone |
| `TestGitPushCap_APushWhoseBodyIsExactlyTheCap...` | A body of exactly the cap, and the same body with the cap one byte lower | Red proof | Accepted at the cap; refused one byte under it, with the limit message on the ref |
| `TestGitPushCap_ABodyThatDeclaresMoreThanTheCap...` | A body that declares more than the cap in `Content-Length` | Red proof | Refused; zero bytes of the body were read |
| `TestGitPushCap_AChunkedBodyIsCutOffAtTheCap...` | A chunked body four times the cap | Red proof | Refused; the server read no more than the cap plus 1 MiB |
| `TestGitPushCapGuard_APushJustUnderThirtyTwoMiB...` | A real `git push` of a random file 64 KiB under 32 MiB | Guard | Accepted; a clone of the branch is at the pushed commit |
| `TestGitPushCapGuard_APushOfTwentyMiB...` | A real `git push` of a 20 MiB random file | Guard | Accepted; a clone of the branch is at the pushed commit |

A guard passes with or without the cap, because the old handler accepted every size. It keeps the cap from refusing pushes it should take. Only a red proof can show the cap exists: the just-under push and the just-over push together show where the limit sits, and only the second is red.

With `-race` the just-over push takes about 3 seconds (it is refused early), the 20 MiB guard about 8 and the just-under guard about 12. Against the earlier tests, the just-under push is the only addition: about 12 seconds.

## Not covered

A per-repository size cap, which is a separate decision. The hub's read deadline for the `/git/` route. Pushing the same annotated tag twice from a client that verifies the tag object's id.
