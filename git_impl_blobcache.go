// git_impl_blobcache.go provides the lazy blob-content hydration helper used
// by [gitManager.ReadFile].
//
// When FetchBranch walks a branch's tip-commit tree it writes Blob entities
// with metadata only (sha, path, name, extension, size) and leaves the
// content field empty — content isn't stored during the walk. The first
// ReadFile call for such a blob triggers loadBlobContentFromBareClone, which:
//
//  1. Looks up the Repository entity's bare_clone_path (the local full clone
//     FetchBranch made — see git_impl_fetchbranch.go's deepenClone).
//  2. Opens that clone directly with go-git's PlainOpen — no network I/O,
//     no storage abstraction; the objects are already on local disk.
//  3. Reads the blob object by its SHA.
//  4. Detects binary vs text and encodes accordingly.
//
// The caller (ReadFile) is responsible for persisting the hydrated content
// back onto the entity.
//
// Adapted from CodeValdGit's version: the original opened blobs through a
// Backend.OpenStorer abstraction (ArangoDB-backed or filesystem, chosen by
// the v1 server). That abstraction doesn't exist here — FetchBranch's full
// clone already lives at a known local path, so this opens it directly.
package mwanachamagit

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"unicode/utf8"

	gogit "github.com/go-git/go-git/v5"
	gogitplumbing "github.com/go-git/go-git/v5/plumbing"
)

// loadBlobContentFromBareClone reads the raw content of blob from the local
// full clone recorded on the owning Repository entity's bare_clone_path
// property. Returns the content string and the encoding ("utf-8" or
// "base64"). Returns [ErrBlobContentUnavailable] if no clone path is
// recorded — the caller should trigger [GitManager.FetchBranch] and retry.
func (m *gitManager) loadBlobContentFromBareClone(ctx context.Context, branch Branch, blob Blob) (content, encoding string, err error) {
	log.Printf("[loadBlobContentFromBareClone] blobID=%s sha=%q repoID=%s", blob.ID, blob.SHA, branch.RepositoryID)
	if blob.SHA == "" {
		return "", "", fmt.Errorf("blob entity %s has no SHA", blob.ID)
	}

	repoEntity, err := m.dm.GetEntity(ctx, m.agencyID, branch.RepositoryID)
	if err != nil {
		log.Printf("[loadBlobContentFromBareClone] GetEntity repoID=%s error: %v", branch.RepositoryID, err)
		return "", "", fmt.Errorf("get repository entity %s: %w", branch.RepositoryID, err)
	}
	cloneDir, _ := repoEntity.Properties["bare_clone_path"].(string)
	log.Printf("[loadBlobContentFromBareClone] bare_clone_path=%q", cloneDir)
	if cloneDir == "" {
		return "", "", ErrBlobContentUnavailable
	}

	repo, err := gogit.PlainOpen(cloneDir)
	if err != nil {
		log.Printf("[loadBlobContentFromBareClone] PlainOpen(%s) error: %v", cloneDir, err)
		return "", "", fmt.Errorf("open local clone %s: %w", cloneDir, err)
	}

	hash := gogitplumbing.NewHash(blob.SHA)
	blobObj, err := repo.BlobObject(hash)
	if err != nil {
		log.Printf("[loadBlobContentFromBareClone] BlobObject sha=%s error: %v", blob.SHA, err)
		return "", "", fmt.Errorf("resolve blob %s in local clone: %w", blob.SHA, err)
	}
	log.Printf("[loadBlobContentFromBareClone] blob object resolved size=%d", blobObj.Size)

	r, err := blobObj.Reader()
	if err != nil {
		return "", "", fmt.Errorf("open blob reader %s: %w", blob.SHA, err)
	}
	defer func() { _ = r.Close() }()

	raw, err := io.ReadAll(r)
	if err != nil {
		return "", "", fmt.Errorf("read blob %s: %w", blob.SHA, err)
	}
	log.Printf("[loadBlobContentFromBareClone] raw bytes read len=%d", len(raw))

	// Detect encoding: treat as binary if the bytes are not valid UTF-8 or
	// contain a null byte (common heuristic used by git itself).
	if bytes.IndexByte(raw, 0) >= 0 || !utf8.Valid(raw) {
		log.Printf("[loadBlobContentFromBareClone] encoding=base64")
		return base64.StdEncoding.EncodeToString(raw), "base64", nil
	}
	log.Printf("[loadBlobContentFromBareClone] encoding=utf-8")
	return string(raw), "utf-8", nil
}
