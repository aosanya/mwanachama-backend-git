# mwanachama-git — documentation

## Layout

Four folders, in SDLC order, and everything lives under one of them.

| Folder | What's inside |
|--------|---------------|
| [1. requirements/](1.%20requirements/) | Problem, vision and scope for versioned content in `mwanachama-kazi`. |
| [2. design/](2.%20design/) | The git-object-graph schema (repos/branches/commits/trees/blobs/merge requests/tags) and how it maps onto `mwanachama-go-shared`'s entity-graph store. |
| [3. implementation/](3.%20implementation/) | The work: `todo.md` (open board), `todo_done.md` (completed rows + board context). |
| [4. qa/](4.%20qa/) | Test coverage and results. |

## Boards and status

| File | What it holds |
| --- | --- |
| [todo.md](3.%20implementation/todo.md) | Open task board |
| [todo_done.md](3.%20implementation/todo_done.md) | Completed rows + board context |

## What this repo is

A Postgres port of `CodeValdGit`'s v2 `GitManager` — an entitygraph-native
git-like object graph (branches/commits/trees/blobs/merge requests/tags/
rollback/history), **not** a real `git` wire-protocol server. Built on
[mwanachama-go-shared](../mwanachama-go-shared)'s entity-graph store instead
of ArangoDB, and imported directly by
[mwanachama-api-gateway](../mwanachama-api-gateway) — no gRPC, no
sub-service shape.
