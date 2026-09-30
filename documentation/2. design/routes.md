# The route table

Forty-three addresses. Forty are declared in `git.operations.json` and built
by `dispatch.Table`; three are hand-written in `routes/undeclared.go` because
the operations format cannot state them.

`routes/routes.go` is the sentinel map, the table and the builder ladder.
There are no handlers: an operation names a `call`, and `dispatch` binds the
request to that method's parameters and renders what it returns.

## What a caller sees

| Method | Path | Call | Status |
| --- | --- | --- | --- |
| POST | `/repos` | InitRepo | 201 |
| GET | `/repos` | ListRepositories, or GetRepositoryByName with `?name=` | 200 |
| GET | `/repos/{repoID}` | GetRepository | 200 |
| DELETE | `/repos/{repoID}` | DeleteRepo | 204 |
| POST | `/repos/{repoID}/purge` | PurgeRepo | 204 |
| POST | `/repos/{repoID}/branches` | CreateBranch | 201 |
| GET | `/repos/{repoID}/branches` | ListBranches, GetBranchByName with `?name=`, ListBranchesFiltered with `?workflow_run_id=` | 200 |
| GET | `/branches/{branchID}` | GetBranch | 200 |
| DELETE | `/branches/{branchID}` | DeleteBranch | 204 |
| POST | `/branches/{branchID}/merge` | MergeBranch | 200 |
| POST | `/repos/{repoID}/tags` | CreateTag | 201 |
| GET | `/repos/{repoID}/tags` | ListTags | 200 |
| GET | `/tags/{tagID}` | GetTag | 200 |
| DELETE | `/tags/{tagID}` | DeleteTag | 204 |
| POST | `/merge-requests` | CreateMergeRequest | 201 |
| GET | `/merge-requests` | ListMergeRequests | 200 |
| GET | `/merge-requests/{mrID}` | GetMergeRequest | 200 |
| POST | `/merge-requests/{mrID}/complete` | CompleteMergeRequest | 200 |
| POST | `/merge-requests/{mrID}/close` | CloseMergeRequest | 200 |
| POST | `/workflow-runs/{workflowRunID}/rollback` | RollbackByWorkflowRun | 200 |
| POST | `/branches/{branchID}/files` | WriteFile | 201 |
| GET | `/branches/{branchID}/files` | ReadFile | 200 |
| DELETE | `/branches/{branchID}/files` | DeleteFile | 200 |
| GET | `/branches/{branchID}/directory` | ListDirectory | 200 |
| GET | `/branches/{branchID}/log` | Log | 200 |
| GET | `/diff` | Diff | 200 |
| POST | `/imports` | ImportRepo | 202 |
| GET | `/imports/{jobID}` | GetImportStatus | 200 |
| POST | `/imports/{jobID}/cancel` | CancelImport | 204 |
| POST | `/keywords` | CreateKeyword | 201 |
| GET | `/keywords` | ListKeywords | 200 |
| GET | `/keywords/tree` | GetKeywordTree | 200 |
| GET | `/keywords/{keywordID}` | GetKeyword | 200 |
| PUT | `/keywords/{keywordID}` | UpdateKeyword | 200 |
| DELETE | `/keywords/{keywordID}` | DeleteKeyword | 204 |
| POST | `/edges` | CreateEdge | 201 |
| DELETE | `/edges` | DeleteEdge | 204 |
| GET | `/branches/{branchID}/neighborhood/{entityID}` | GetNeighborhood | 200 |
| POST | `/search/keywords` | SearchByKeywords | 200 |
| POST | `/graph/query` | QueryGraph | 200 |
| POST | `/branches/{branchID}/fetch` | FetchBranch | 202 |
| GET | `/fetch-jobs/{jobID}` | GetFetchBranchStatus | 200 |
| POST | `/search/blobs` | SearchBlobs | 200 |

`routes_test.go` writes this set out rather than counting it. A conversion
that answers forty-three addresses with the wrong forty-three passes a count
and fails the list.

## The three that are not declared

An operation names exactly one `call`, and `dispatch` renders every error as
`{"error": "..."}`. Three addresses need more than that.

**`GET /repos`** and **`GET /repos/{repoID}/branches`** choose between two
and three manager methods on which query parameter is present. Giving each
its own path would have been a declaration, and a different API.

**`POST /branches/{branchID}/merge`** answers a conflict with `task_id` and
`conflicting_files` beside the message. `*ErrMergeConflict` is also a typed
error matched with `errors.As`, which the status table's `errors.Is` lookup
does not reach — so declaring it would have turned a 409 with a body a caller
can act on into a redacted 500. That is catalog's CAT7 in its worst form,
and it is why this one stayed in Go.

All three are filed as engine gaps on `mwanachama-backend-shared`'s board.

## The gate is data

`AnonymousActions` is empty, so every action is gated. It is an allowlist of
action ids: an operation added later and not named there arrives gated, so
the failure direction is a 401 rather than the whole table on the public
internet. `TestEveryAnonymousActionIsARealAction` refuses a name no operation
declares.

## Errors

`git.operations.json`'s `errors` maps all twenty-three exported sentinels to
the status `routes/wire.go`'s `gitStatusFor` gave them — ten to 409, ten to
404, three to 400. They were transcribed, not reconsidered. Anything that
looks wrong belongs on the board, not in the same pass.

An unmapped sentinel is redacted to a 500 `"internal error"`.
`TestEverySentinelTheSpecNeverMapsIsReported` holds the two sets together, so
that cannot happen quietly.
