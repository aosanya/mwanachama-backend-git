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

`GetNeighborhood`'s path fallback (`resolveEntityID`) still resolves a path
against the whole Blob table; that is G16 and is not changed here.
