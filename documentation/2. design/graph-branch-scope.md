# Graph queries are scoped to the branch they name

`QueryGraph` (`POST /graph/query`) and `SearchByKeywords`
(`POST /search/keywords`) both take a `branch_id`. Until G15 they used it only
to confirm the branch existed, then queried the deployment's whole Blob table
and blob-keyword-tag table, so any caller holding one branch of their own could
read every repository's file content.

Both now read the set of blobs in the tree of the branch's head commit
(`blobIDsOnBranch`, built on `allBlobsAtCommit`, the same walk the doc-edge
lifecycle uses) and restrict their results to it:

- `QueryGraph` adds `b.id IN <that set>` to its query before the file-type,
  folder, keyword and signal filters. A branch with no head commit answers an
  empty graph.
- `SearchByKeywords` intersects each keyword's tagged-blob set with the branch's
  set before applying the AND/OR match mode, so a cascade or an OR can never
  reach outside the branch.

"Reachable from the branch" means the head commit's tree, the same snapshot
`ReadFile` and `ListDirectory` read, not every blob in the branch's history.

Content-addressed reuse is unaffected: a blob row shared by two repositories
because both hold the same bytes at the same path is visible from either, since
the caller already holds that content.

Guarded by `routes/graph_crossrepo_test.go`'s
`TestG15_QueryGraphStaysInsideTheCallersBranch` and
`TestG15_SearchByKeywordsStaysInsideTheCallersBranch`.

## A neighbourhood named by path (G16)

`GetNeighborhood` (`GET /branches/{branchID}/neighborhood/{entityID}`) accepts a
file path in place of a row id. Until G16 that fallback (`resolveEntityID`)
looked the path up across the deployment's whole Blob table, so a caller
holding any branch could read another repository's file by guessing its name
(`secrets.txt`, `.env`), with no id at all.

The fallback now looks the path up only among the blobs at the named branch's
head commit, the same `allBlobsAtCommit` set as above. A path the branch does
not hold, or a branch with no head commit, answers `ErrEntityNotFound` (404),
exactly as a path nobody holds.

A raw row id is still resolved across every node table, as before; G16 is the
path fallback only.

Guarded by `routes/graph_neighborhood_crossrepo_test.go`'s
`TestG16_NeighborhoodByPathStaysInsideTheCallersBranch`, which also checks the
caller's own path still resolves.
