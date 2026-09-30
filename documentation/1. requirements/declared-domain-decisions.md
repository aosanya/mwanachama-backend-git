# Decisions taken converting this module to a declared domain

Recorded 2026-09-30, as the conversion was made. Each row is a choice that
could reasonably have gone the other way; the reasoning is here so a later
reader does not re-open it by accident.

## Does this module name a domain?

**No, and it needed no renames.** The rule is that a word meaning something
in one domain and nothing in another does not belong in the module. Git's
nouns — repository, branch, commit, tree, blob, tag, merge request — are
*version control's* words, and version control is what this module is, the
same standing `entry` has in `mwanachama-backend-catalog`. The one genuine
tenant word, `Agency`, was removed outright in 2026-09 as vestigial.

The two shipped domain specs are the test of this. `engineering` names the
objects repository/branch/commit/file; `chambers` names the same objects
matter/negotiation/revision/document, a law firm versioning contract drafts.
Both load, both migrate into one database, and no Go changed between them.

| Decision | Why |
| --- | --- |
| Roles keep git's own vocabulary | A librarian would not recognise `commit`, but neither would they recognise `ledger` in an accounting module. The test is whether the word names a *tenant's* domain, not whether it is universal. |
| `chambers` is shipped as the second example | A second domain nobody has provisioned, so the shipped example is not also production config — catalog's CAT6 is that mistake already filed. |

## Field-level decisions

| Field | Decision | Why |
| --- | --- | --- |
| `commit.sha`, `tree.sha`, `blob.sha` | **Not `unique`** | Entitygraph declared uniqueness and never enforced it — every write went through create, not upsert. Every indexing path re-materialises content, and identical bytes are shared across repositories the way a fork shares objects on a real git host. A unique index would break writes that succeed today. |
| `branch.status` | **`string`, not `enum`** | Its four values apply only to a lazily imported branch. Empty is the ordinary case, and an enum would refuse it. |
| `blob_keyword_tag.signal` | **`string`, not `enum`** | An unrecognised signal ranks last rather than being refused, and that tolerance is the existing behaviour. |
| `blob_reference.name` | **`enum`** | Genuinely closed and already enforced in Go by `validDocEdges`, which the enum replaced. This is the one rule the spec took over outright. |
| `commit.tree_id`, `keyword.parent_id` | **Not nullable** | Nothing distinguished NULL from empty: the row converters mapped `""` to `nil` and back in both directions. The four `IS NULL` predicates became equality on empty, which keeps the JSON contract on plain strings rather than growing pointers. |
| Nothing carries `matches` | **Deliberate** | `patterns.go` supplies `sha` and `ref_name` so a domain *may* name them, but declaring them on the module's fields would start refusing writes that succeed today. Behaviour is held exactly; tightening it is a separate decision. |

## Fields that are not columns

Six fields are carried on the domain types and stored nowhere:
`Commit.ParentIDs`, `Tree.BlobIDs`, `Tree.SubtreeIDs`, `Tree.CommitID`,
`Keyword.ChildIDs` — all read from join tables or an inverse — and
`Blob.TreeID`, which is carried for the JSON contract and has never been
populated at all.

`specstore.New` refused any carrier field the object did not declare, in both
directions, so before this conversion the only ways forward were to delete
the fields and break whatever depended on them, or to keep row structs and
hand-written converters — the `gormstore/` shape the standard exists to
delete. The engine grew a `spec:"-"` tag instead
(`mwanachama-backend-shared`'s S36), which is where the generic answer
belongs. An untagged extra field is still refused by name.

## What stayed in Go, and why

- **`SmartHTTPHandler()`** streams packfiles over the git wire protocol. It
  is not JSON in, JSON out, and it is not in the route table.
- **`WithMergeLock`** takes a Go closure.
- **The recursive CTEs** (`BlobsAtCommit`, `CommitChainIDs`,
  `KeywordDescendantIDs`), the sixteen-shape edge catalogue and the node-type
  probe. These are queries, not declarations.
- **The blob full-text index.** `spec` has no expression or GIN index, only
  btree over columns and document paths, so `Provision` applies the
  `to_tsvector` GIN index after `spec.Migrate`. Filed as an engine gap.
- **Three HTTP addresses** — see `2. design/routes.md`.

## Preconditions that were checked

No live table set holds data: neither consumer (`api-gateway`, `api-kazi`)
compiles, both were already broken by earlier sibling conversions, and
`wakala-api` — the live consumer — does not import this module. Git was never
mirrored in the gateway's `migrations/` either; it relied on `AutoMigrate` at
startup, so `cmd/migrate up` could not provision it before this change and
still cannot. `cmd/ddl` now prints the statements a mirror would need.
