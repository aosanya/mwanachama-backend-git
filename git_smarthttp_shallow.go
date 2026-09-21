package mwanachamagit

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/packfile"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/capability"
)

// go-git's upload-pack session refuses every shallow request and never
// advertises the "shallow" capability, so a real `git clone --depth N` dies
// client-side with "Server does not support shallow clients". This file adds
// the one shallow case worth serving: a fresh clone (no haves, no shallow
// lines from the client) limited to the newest N commits. Deepening or
// fetching shallowly into an existing repository, and --shallow-since /
// --shallow-exclude, are refused with a message saying so.

// shallowRefusal returns why req cannot be served as a shallow clone, or ""
// when it can.
func shallowRefusal(req *packp.UploadPackRequest) string {
	depth, ok := req.Depth.(packp.DepthCommits)
	if !ok || depth <= 0 {
		return "only --depth N is supported; --shallow-since and --shallow-exclude are not"
	}
	if len(req.Shallows) > 0 || len(req.Haves) > 0 {
		return "shallow fetch into an existing repository (deepen, unshallow, or fetch --depth) is not supported; use a fresh `git clone --depth N`"
	}
	return ""
}

// serveShallowClone answers a fresh `--depth N` clone: it announces the
// boundary commits as shallow, then sends a pack holding only the commits
// within N of a want and their trees and blobs. done is whether the client's
// request ended with "done" — see the two-phase note below.
func (h *smartHTTPHandler) serveShallowClone(w http.ResponseWriter, ctx context.Context, repoName string, req *packp.UploadPackRequest, done bool) {
	repo, err := h.m.openOrInitBareRepo(ctx, repoName)
	if err != nil {
		http.Error(w, "internal error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	objs, shallows, err := shallowObjects(repo, req.Wants, int(req.Depth.(packp.DepthCommits)))
	if err != nil {
		http.Error(w, "shallow clone: "+err.Error(), http.StatusInternalServerError)
		return
	}

	setNoCacheHeaders(w)
	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	w.WriteHeader(http.StatusOK)

	// Stateless RPC splits a shallow clone with nothing to negotiate in two
	// requests, and both answers open with the shallow list (this is what
	// `git upload-pack --stateless-rpc` sends). The first, without "done",
	// ends there; the second, with "done", continues with NAK and the pack.
	update := packp.ShallowUpdate{Shallows: shallows}
	if err := update.Encode(w); err != nil {
		log.Printf("[upload-pack] repo=%q: shallow list: %v", repoName, err)
		return
	}
	if !done {
		return
	}

	pr, pw := io.Pipe()
	enc := packfile.NewEncoder(pw, repo.Storer, false)
	go func() {
		_, err := enc.Encode(objs, 10)
		pw.CloseWithError(err)
	}()
	packOnly := *req
	packOnly.Depth = packp.DepthCommits(0) // the shallow list is already written; this encodes NAK and the pack
	resp := packp.NewUploadPackResponseWithPackfile(&packOnly, pr)
	if err := resp.Encode(w); err != nil {
		log.Printf("[upload-pack] repo=%q: shallow pack: %v", repoName, err)
	}
}

// shallowObjects returns every object a depth-limited clone of wants needs,
// plus the shallow boundary: the included commits whose parents were cut off.
// Depth 1 is the wanted commits alone.
func shallowObjects(repo *gogit.Repository, wants []plumbing.Hash, depth int) ([]plumbing.Hash, []plumbing.Hash, error) {
	included := map[plumbing.Hash]*object.Commit{}
	var order []plumbing.Hash
	seen := map[plumbing.Hash]bool{}
	add := func(h plumbing.Hash) {
		if !seen[h] {
			seen[h] = true
			order = append(order, h)
		}
	}

	level := []plumbing.Hash{}
	for _, want := range wants {
		if tag, err := repo.TagObject(want); err == nil {
			add(want)
			c, err := tag.Commit()
			if err != nil {
				return nil, nil, fmt.Errorf("want %s: %w", want, err)
			}
			want = c.Hash
		}
		level = append(level, want)
	}

	for d := 0; d < depth && len(level) > 0; d++ {
		var next []plumbing.Hash
		for _, h := range level {
			if _, done := included[h]; done {
				continue
			}
			c, err := repo.CommitObject(h)
			if err != nil {
				return nil, nil, fmt.Errorf("read commit %s: %w", h, err)
			}
			included[h] = c
			add(h)
			next = append(next, c.ParentHashes...)
		}
		level = next
	}

	var shallows []plumbing.Hash
	for h, c := range included {
		for _, p := range c.ParentHashes {
			if _, in := included[p]; !in {
				shallows = append(shallows, h)
				break
			}
		}
	}

	for _, h := range append([]plumbing.Hash(nil), order...) {
		c, ok := included[h]
		if !ok {
			continue
		}
		tree, err := c.Tree()
		if err != nil {
			return nil, nil, fmt.Errorf("tree of %s: %w", h, err)
		}
		add(tree.Hash)
		walker := object.NewTreeWalker(tree, true, nil)
		for {
			_, entry, err := walker.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				walker.Close()
				return nil, nil, fmt.Errorf("walk tree of %s: %w", h, err)
			}
			if entry.Mode == 0o160000 { // submodule: the commit lives in another repo
				continue
			}
			add(entry.Hash)
		}
		walker.Close()
	}
	return order, shallows, nil
}

// stripShallowCap drops the "shallow" capability from a request headed for
// go-git's own session, which rejects any capability it does not support.
func stripShallowCap(req *packp.UploadPackRequest) {
	if req.Capabilities != nil && req.Capabilities.Supports(capability.Shallow) {
		req.Capabilities.Delete(capability.Shallow)
	}
}
