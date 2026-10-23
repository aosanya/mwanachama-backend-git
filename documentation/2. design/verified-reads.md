# Verified reads (G27)

A published result must not be quietly amended. Every blob row records the
git object id of its content (`sha`, a real go-git hash computed by
`WriteFile` or carried in from a push). Before G27 nothing recomputed it, so a
direct SQL write to `content` (or to `sha`) went unnoticed by every reader.

## What is built

- `ReadFile` recomputes the blob's git hash (`blob <len>\0<bytes>`, SHA-1, via
  `plumbing.ComputeHash`) over the content it is about to return, on both
  paths: content stored in the row and content hydrated lazily from the bare
  clone.
- Content with `encoding = base64` is accepted when the hash matches either
  the stored text or its decoded bytes, so binary files written either way
  verify.
- A mismatch is `ErrContentTampered`, declared in `git.operations.json`'s
  error map as **409**. The read is refused rather than served with a warning:
  a caller cannot be trusted to inspect a flag on a document that carries
  weight.
- Pinned by `integrity_test.go`, which amends a row by direct SQL (content,
  then the recorded `sha`) and sees the refusal.

## What it does not cover

- **Commits and trees are not recomputed.** A commit row stores its message,
  author and tree reference, but not the exact serialised object (timezone,
  committer, parent order as git wrote it), so its id cannot be rebuilt from
  the row alone. Verifying the chain needs either the raw object stored or a
  walk of the bare clone; not built.
- **A coordinated rewrite of both `content` and `sha`** to a consistent pair
  passes. Recomputation detects accidental or careless edits and any edit
  that leaves the recorded hash alone; it cannot detect an attacker who also
  rewrites the hash, because the hash lives in the same database. Closing
  that needs the commit chain verified up to a ref anchored outside the
  database, or a signature — the open half of G27.
- **Signing** is a product decision and is not built: whose key signs, which
  documents count as carrying political weight, and where a verifier gets the
  public key.
