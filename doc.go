// Package mwanachamagit provides git-like versioned content: repositories,
// branches, commits, trees, blobs, merge requests, tags, and a keyword
// tagging layer over files. Domain logic and storage both live in this
// package, imported directly by whatever mounts it — no separate service, no
// gRPC, no proto.
//
// The objects are declared, not written. git.blueprint.json names the
// fifteen objects and every field; a domain spec under spec/examples names
// which object fills each role, what the domain calls it and where it lands.
// git.operations.json declares the HTTP surface. What remains in Go is what
// a declaration cannot state: the go-git object model, the wire protocol,
// the recursive CTEs, and the rules that depend on more than one field.
//
// Layout:
//   - git.blueprint.json, blueprint.go — the objects, declared once
//   - spec/examples/          — two domains, so neutrality is exercised
//   - git.operations.json, operations.go — the route table, declared once
//   - store.go               — the roles, the carriers, the physical names
//   - provision.go, cmd/ddl  — the DDL, applied and printed
//   - validate.go, patterns.go — the rules the spec states
//   - models/                — the domain types, which are also the carriers
//   - queries.go             — the recursive CTEs and the edge catalogue
//   - git.go                 — GitManager, gitManager, the constructor
//   - git_impl_*.go          — the behaviour behind each method
//   - git_smarthttp*.go      — the real git wire protocol
//   - routes/                — the declared table, plus the three addresses
//     a declaration cannot state
//
// See CLAUDE.md for what is superseded and why.
package mwanachamagit
