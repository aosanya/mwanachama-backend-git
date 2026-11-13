# Many agents on one repository file (G62)

Date: 2026-11-13. Hub issues G62 and G72.

## Machine

Apple M3 Pro, 11 logical CPUs, 18 GiB, macOS 15.7.3. go1.27.3 darwin/arm64.

## The test

`repofile_concurrency_test.go`, no build tag. Run with:

```
go test -race -run TestRepoFileConcurrency ./...
```

- N agents (10 and 30) start together on a barrier, on one `OpenRepoFile`: a real SQLite file, one connection, busy timeout 10 s, rollback journal.
- Each agent cuts its own branch from the default branch, makes two `WriteFile` commits to its own file, and runs `MergeBranch` into the default branch.
- A refused merge (`ErrMergeConcurrencyConflict`) is retried on a fresh branch.
- Two protocols:
  - `mergeWithoutRecheck` (MWR): merge as soon as the commits are written.
  - `recheckDefaultHeadBeforeMerge` (RDH): the agent checks that the default head did not move before merging, and re-cuts the branch if it did. A re-cut is counted as stale.
- `TestRepoFileConcurrency_SecondMergeOfSiblingBranchKeepsFirstMergedCommit` is a deterministic two-branch test of the case the many-agent run exposed.

The earlier measurement, before the true merge of G67, lost 8 to 9 of 10 files and 28 to 29 of 30, with 0 refused merges. That is why G72 was filed.

Results below are three runs with `-race` and three without, each running both protocols at 10 and 30 agents: 12 many-agent subtest runs per build, plus the sibling-branch test each time.

## Results

Commits per second counts commits written plus merges. Each cell covers the six runs for that agent count (MWR and RDH, three runs each).

| Build | Agents | Min | Median | Max |
|-------|--------|-----|--------|-----|
| `-race` | 10 | 66.8 | 78.1 | 79.6 |
| `-race` | 30 | 59.4 | 68.1 | 69.5 |
| no `-race` | 10 | 245.1 | 347.5 | 400.5 |
| no `-race` | 30 | 179.6 | 247.1 | 295.4 |

## What they mean

- **No commit lost, no file lost.** Every run kept every file and every commit the agents wrote, at 10 and 30 agents, under both protocols, with and without `-race`. The sibling-branch test passed 3 of 3 under `-race` (0.24 to 0.34 s) and 3 of 3 without it (0.05 to 0.06 s).
- **Refused merges: 0 in every run.** The true merge combines the changes of agents that wrote different files, so no merge needs refusing. `ErrMergeConcurrencyConflict` appears only when the default head moves between reading and writing, which the module's lock prevents; see [How a merge works](../2.%20design/merge.md).
- **Stale re-cuts.** RDH re-cuts a branch when the default head moved. Stale counts ran from 0 to 16 per run; commits written rise by two for every re-cut.
- **Locking.** No `SQLITE_BUSY` ("database is locked"), no deadlock and no data race in any run.
- **Throughput.** With `-race` the median is 78 (10 agents) and 68 (30 agents) commits per second; without it 347 and 247. A merge now builds a merge commit, so it costs more than the earlier head move, and the rate falls as agents grow from 10 to 30. Thirty agents finish in 0.3 to 0.5 s without `-race` and 1.3 to 2.0 s with it.
- **G72 closed.** G62 asked for no commit lost; that holds. The plain run `go test -run TestRepoFileConcurrency .` takes about 1.3 s of test time.

## Not measured

- Files with large content.
- A mix of readers alongside the writers.
- More than one repository.

## Raw data

Protocol: MWR is `mergeWithoutRecheck`, RDH is `recheckDefaultHeadBeforeMerge`. Refused and lost commits and files were 0 in every row.

| Run | Protocol | Agents | Commits | Merges | Refused | Stale | Elapsed | Commits/sec |
|-----|----------|--------|---------|--------|---------|-------|---------|-------------|
| race1 | MWR | 10 | 20 | 10 | 0 | 0 | 449ms | 66.8 |
| race1 | MWR | 30 | 60 | 30 | 0 | 0 | 1.515s | 59.4 |
| race1 | RDH | 10 | 48 | 10 | 0 | 14 | 777ms | 74.7 |
| race1 | RDH | 30 | 60 | 30 | 0 | 0 | 1.295s | 69.5 |
| race2 | MWR | 10 | 20 | 10 | 0 | 0 | 380ms | 78.9 |
| race2 | MWR | 30 | 60 | 30 | 0 | 0 | 1.307s | 68.9 |
| race2 | RDH | 10 | 20 | 10 | 0 | 0 | 385ms | 77.9 |
| race2 | RDH | 30 | 92 | 30 | 0 | 16 | 2.026s | 60.2 |
| race3 | MWR | 10 | 20 | 10 | 0 | 0 | 377ms | 79.6 |
| race3 | MWR | 30 | 60 | 30 | 0 | 0 | 1.299s | 69.3 |
| race3 | RDH | 10 | 20 | 10 | 0 | 0 | 384ms | 78.2 |
| race3 | RDH | 30 | 68 | 30 | 0 | 4 | 1.457s | 67.2 |
| norace1 | MWR | 10 | 20 | 10 | 0 | 0 | 81ms | 370.6 |
| norace1 | MWR | 30 | 60 | 30 | 0 | 0 | 501ms | 179.6 |
| norace1 | RDH | 10 | 32 | 10 | 0 | 6 | 151ms | 277.5 |
| norace1 | RDH | 30 | 60 | 30 | 0 | 0 | 337ms | 267.1 |
| norace2 | MWR | 10 | 20 | 10 | 0 | 0 | 93ms | 324.3 |
| norace2 | MWR | 30 | 60 | 30 | 0 | 0 | 372ms | 242.2 |
| norace2 | RDH | 10 | 20 | 10 | 0 | 0 | 76ms | 392.6 |
| norace2 | RDH | 30 | 60 | 30 | 0 | 0 | 357ms | 252.0 |
| norace3 | MWR | 10 | 20 | 10 | 0 | 0 | 75ms | 400.5 |
| norace3 | MWR | 30 | 60 | 30 | 0 | 0 | 305ms | 295.4 |
| norace3 | RDH | 10 | 20 | 10 | 0 | 0 | 122ms | 245.1 |
| norace3 | RDH | 30 | 60 | 30 | 0 | 0 | 378ms | 238.3 |
