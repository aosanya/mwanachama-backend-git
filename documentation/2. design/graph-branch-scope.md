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

G16 was the path fallback only; a raw row id is G22, below.

Guarded by `routes/graph_neighborhood_crossrepo_test.go`'s
`TestG16_NeighborhoodByPathStaysInsideTheCallersBranch`, which also checks the
caller's own path still resolves.

## A neighbourhood named by raw id (G22)

Until G22 a raw row id was resolved across every node table and the walk
followed every edge, so a caller holding one branch and another repository's
blob, commit or tree id read that neighbourhood, file content included.
Member app decision 251 (2026-10-22) settled the rule: a read serves only the
repository's own content at the named branch's current head.

`nodesAtBranchHead` builds the set a neighbourhood may contain:

- the commits of the branch's repository (`repository_id`), and its head commit;
- the head commit's root tree and every subtree beneath it;
- the blobs at the head commit (`allBlobsAtCommit`, the same set as above);
- the keyword taxonomy, which is not repository data.

`GetNeighborhood` answers `ErrEntityNotFound` (404) when the resolved start is
not in that set — another repository's id, or a blob or tree that an edit has
since replaced in the head's tree — exactly as an id nobody holds. The walk
admits only neighbours in the set, so it can never step from a shared keyword
or a content-addressed tree into another repository. Repository, branch, tag
and merge-request rows are outside the set, so a neighbourhood no longer
returns them; an edge to one is dropped with its endpoint. A branch with no
head commit holds nothing.

Guarded by `routes/graph_neighborhood_rawid_test.go`'s
`TestG22_NeighborhoodByRawIDStaysInsideTheCallersBranchHead`, which also checks
the caller's own raw blob id at head still resolves.
