# Agency report repositories (G21)

Member app decisions 212 and 213: each agency keeps its form reports as
versioned PDFs in a git repository of its own. Replacing a report keeps the
old one in history, and members see the latest.

## What the host uses

The three operations already on `GitManager` are the host-facing surface:

- `WriteFile` writes one file to a path with a commit message. A PDF goes in
  with `Encoding: "base64"`.
- `ReadFile` reads a path's latest content on a branch.
- `Log` with `LogFilter{Path: ...}` is that path's history, newest first.

## One agency, one table set

The store is scoped by the spec's `instance`. A host mounting this module per
agency builds one spec per agency with its own instance and one
`GitManager` over it; every table is `<instance>_…`, so two agencies sharing
one database never share a row. A branch id from one agency resolves to
nothing in another: `ReadFile` and `Log` refuse it, and `ListRepositories`
answers only the agency's own. `TestTwoAgenciesKeepTheirReportsApart` holds
this with two instances in one SQLite database.

## The size limit is the host's

`NewGitManager` takes options after its five arguments, so existing callers
are unchanged. `WithMaxFileSize(bytes)` sets the largest file `WriteFile`
accepts; base64 content is measured decoded, as the bytes the file holds. A
larger write is refused with `ErrFileTooLarge` (413 on the route table) before
anything is stored. With no option, or a limit of zero or less, there is no
limit: the module does not pick a number for the host.

## Not built here

- An embedded domain spec and `SpecFor(instance)`, as forms has. This repo
  ships only examples, and which domain the agency repositories use is the
  host's (wakala-api WK153).
- Restricting a report repository to PDFs. The row says PDFs; nothing in the
  module checks a content type, and whether it should is the host's call.
