# Fix flow: this repository is the custodian

Both `projects/hub-steward/code/wakala-api` and `mwanachama-wakala-api` import this module at a tagged version (hub issue ISS-105). The full page is `documentation/3. implementation/fix-flow-backend-imports.md` in the hub repository. This page is the part that happens here.

## Rules

- Every fix to the git module is made here, never in a consumer. No copy, no sync script and no `vendor/` folder exists anywhere. `hub-copy-diff.md` only reports the remaining hub copy until the switch lands, then it has nothing left to compare.
- The red test comes first and must fail for the reason of the bug.
- Shared before git. This module requires `mwanachama-backend-shared`, so a shared fix is tagged first, and `go.mod` here is bumped to that tag before this module is tagged. `go.mod` carries no `replace` and no `v0.0.0` in a tagged commit, because a consumer ignores a dependency's `replace`.

## Steps

1. Red test, fix, `go test ./...` here.
2. Run the tests again on a clean export of the commit (fresh clone into an empty folder, no sibling folders). Green.
3. The owner confirms the tag number. A tag pushed to a public repository is remembered by the Go proxy and checksum database and cannot be moved; a wrong one is superseded by a new version.
4. Tag and push. Wait until `go list -m github.com/aosanya/mwanachama-backend-git@vX.Y.Z` answers.
5. Bump the version in `hub-steward/code/wakala-api/go.mod` and in `mwanachama-wakala-api/go.mod`, run the git tests in both, commit both.

## Private siblings

The routing layer here takes the catalog through an interface and never imports `mwanachama-backend-catalog`, which is private until the owner makes it public. The other private backends (agency, permissions, ratings, workspace) follow the same flow once public; until then only `mwanachama-wakala-api` imports them, from sibling folders with `replace`.
