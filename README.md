# mwanachama-git

Postgres port of `CodeValdGit`'s entitygraph-native `GitManager` — a
git-like object graph (repositories, branches, commits, trees, blobs, merge
requests, tags, rollback, history) for versioned content in
[mwanachama-frontend-kazi](../mwanachama-frontend-kazi).

Not a real `git` wire-protocol server — no `git clone`/`push` interop, no
gRPC, no sub-service shape. Built on
[mwanachama-go-shared](../mwanachama-go-shared)'s Postgres entity-graph store
and imported directly by
[mwanachama-api-gateway](../mwanachama-api-gateway).

See [documentation/](documentation/) for design and task board.
