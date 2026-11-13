# Many agents on one repository file (G62)

Date: 2026-11-13. Hub issue G62.

## Machine

Apple M3 Pro, 11 logical CPUs, 18 GiB, macOS 15.7.3. go1.27.3 darwin/arm64.

## The test

`repofile_concurrency_test.go`, build tag `g62defect` because it currently fails. Run with:

```
go test -race -tags g62defect -run TestRepoFileConcurrency ./...
```

- N agents (10 and 30) start together on a barrier, on one `OpenRepoFile`: a real SQLite file, one connection, busy timeout 10 s, rollback journal.
- Each agent cuts its own branch from the default branch, makes two `WriteFile` commits to its own file, and runs `MergeBranch` into the default branch.
- A refused merge (`ErrMergeConcurrencyConflict`) is retried on a fresh branch.
- Two protocols:
  - `mergeWithoutRecheck`: merge as soon as the commits are written.
  - `recheckDefaultHeadBeforeMerge`: the agent checks that the default head did not move before merging, and re-cuts the branch if it did. A re-cut is counted as stale.
- A deterministic two-branch repro test shows the defect below without any concurrency.

Five runs with `-race` and three without, each running both protocols at 10 and 30 agents: 32 subtest runs.

## Results

Commits per second counts commits written plus merges.

| Build | Agents | Min | Median | Max |
|-------|--------|-----|--------|-----|
| `-race` | 10 | 70.3 | 85.5 | 107.0 |
| `-race` | 30 | 57.3 | 76.2 | 103.0 |
| no `-race` | 10 | 405.6 | 448.9 | 504.7 |
| no `-race` | 30 | 392.4 | 451.4 | 494.0 |

## What they mean

- **Throughput.** With `-race` the median is 85 (10 agents) and 76 (30 agents); without `-race` it is 449 and 451. The single writer per file does not slow as agents grow from 10 to 30. Thirty agents finish in about 0.2 s without `-race`.
- **Locking.** No `SQLITE_BUSY` ("database is locked"), no deadlock and no data race in any of the 32 subtest runs.
- **Refused merges.** Zero in every run. `MergeBranch` re-reads the default head inside the lock and compares against it, so the compare-and-swap cannot fail.
- **Defect: no commit lost failed in all 32 runs.** `MergeBranch` moves the default head to the source head without checking ancestry, so a branch cut from an older default head drops the merged commits that came before it. Ten agents lose 8 to 9 of 10 files; 30 agents lose 28 to 29 of 30. The default history keeps 2 entries.
- **Done condition not met.** G62 asks for no commit lost, and for refused merges that return `ErrMergeConcurrencyConflict` and succeed on retry. Neither holds. It is blocked on hub bug G72: `mergeBranch` must refuse a source that does not descend from the default head. The throughput figures measure the merge as built and will be re-run after G72.

## Not measured

- Files with large content.
- A mix of readers alongside the writers.
- More than one repository.

## Raw data

Protocol: MWR is `mergeWithoutRecheck`, RDH is `recheckDefaultHeadBeforeMerge`. Lost files and lost commits are out of the files written and commits written by the agents.

| Run | Protocol | Agents | Commits | Merges | Refused | Stale | Elapsed | Commits/sec | Lost files | Lost commits |
|-----|----------|--------|---------|--------|---------|-------|---------|-------------|------------|--------------|
| race1 | MWR | 10 | 20 | 10 | 0 | 0 | 385ms | 78.0 | 9/10 | 18/20 |
| race1 | MWR | 30 | 60 | 30 | 0 | 0 | 999ms | 90.1 | 29/30 | 58/60 |
| race1 | RDH | 10 | 20 | 10 | 0 | 0 | 352ms | 85.3 | 9/10 | 18/20 |
| race1 | RDH | 30 | 144 | 30 | 0 | 42 | 2.452s | 71.0 | 28/30 | 56/60 |
| race2 | MWR | 10 | 20 | 10 | 0 | 0 | 423ms | 70.9 | 9/10 | 18/20 |
| race2 | MWR | 30 | 60 | 30 | 0 | 0 | 1.17s | 76.9 | 29/30 | 58/60 |
| race2 | RDH | 10 | 24 | 10 | 0 | 2 | 484ms | 70.3 | 8/10 | 16/20 |
| race2 | RDH | 30 | 152 | 30 | 0 | 46 | 2.572s | 70.8 | 28/30 | 56/60 |
| race3 | MWR | 10 | 20 | 10 | 0 | 0 | 413ms | 72.6 | 9/10 | 18/20 |
| race3 | MWR | 30 | 60 | 30 | 0 | 0 | 1.571s | 57.3 | 29/30 | 58/60 |
| race3 | RDH | 10 | 20 | 10 | 0 | 0 | 350ms | 85.7 | 9/10 | 18/20 |
| race3 | RDH | 30 | 132 | 30 | 0 | 36 | 2.145s | 75.5 | 28/30 | 56/60 |
| race4 | MWR | 10 | 20 | 10 | 0 | 0 | 280ms | 107.0 | 9/10 | 18/20 |
| race4 | MWR | 30 | 60 | 30 | 0 | 0 | 945ms | 95.3 | 29/30 | 58/60 |
| race4 | RDH | 10 | 24 | 10 | 0 | 2 | 353ms | 96.3 | 8/10 | 16/20 |
| race4 | RDH | 30 | 60 | 30 | 0 | 0 | 873ms | 103.0 | 29/30 | 58/60 |
| race5 | MWR | 10 | 20 | 10 | 0 | 0 | 334ms | 89.9 | 9/10 | 18/20 |
| race5 | MWR | 30 | 60 | 30 | 0 | 0 | 883ms | 102.0 | 29/30 | 58/60 |
| race5 | RDH | 10 | 20 | 10 | 0 | 0 | 294ms | 102.1 | 9/10 | 18/20 |
| race5 | RDH | 30 | 64 | 30 | 0 | 2 | 1.283s | 73.3 | 28/30 | 56/60 |
| norace1 | MWR | 10 | 20 | 10 | 0 | 0 | 69ms | 437.5 | 9/10 | 18/20 |
| norace1 | MWR | 30 | 60 | 30 | 0 | 0 | 229ms | 392.4 | 29/30 | 58/60 |
| norace1 | RDH | 10 | 20 | 10 | 0 | 0 | 65ms | 464.7 | 9/10 | 18/20 |
| norace1 | RDH | 30 | 104 | 30 | 0 | 22 | 323ms | 414.6 | 28/30 | 56/60 |
| norace2 | MWR | 10 | 20 | 10 | 0 | 0 | 67ms | 444.9 | 9/10 | 18/20 |
| norace2 | MWR | 30 | 60 | 30 | 0 | 0 | 203ms | 443.0 | 29/30 | 58/60 |
| norace2 | RDH | 10 | 20 | 10 | 0 | 0 | 74ms | 405.6 | 9/10 | 18/20 |
| norace2 | RDH | 30 | 60 | 30 | 0 | 0 | 194ms | 463.5 | 29/30 | 58/60 |
| norace3 | MWR | 10 | 20 | 10 | 0 | 0 | 66ms | 452.9 | 9/10 | 18/20 |
| norace3 | MWR | 30 | 60 | 30 | 0 | 0 | 182ms | 494.0 | 29/30 | 58/60 |
| norace3 | RDH | 10 | 20 | 10 | 0 | 0 | 59ms | 504.7 | 9/10 | 18/20 |
| norace3 | RDH | 30 | 60 | 30 | 0 | 0 | 196ms | 459.7 | 29/30 | 58/60 |
