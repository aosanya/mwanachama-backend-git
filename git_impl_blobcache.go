// git_impl_blobcache.go provides the lazy blob-content hydration helper used
// by [gitManager.ReadFile].
//
// When FetchBranch walks a branch's tip-commit tree it writes Blob rows
// with metadata only (sha, path, name, extension, size) and leaves the
// content field empty — content isn't stored during the walk. The first
// ReadFile call for such a blob triggers loadBlobContentFromBareClone, which:
//
//  1. Looks up the Repository row's BareClonePath (the local full clone
//     FetchBranch made — see git_impl_fetchbranch.go's deepenClone).
//  2. Opens that clone directly with go-git's PlainOpen — no network I/O,
//     no storage abstraction; the objects are already on local disk.
//  3. Reads the blob object by its SHA.
//  4. Detects binary vs text and encodes accordingly.
//
// The caller (ReadFile) is responsible for persisting the hydrated content
// back onto the row.
package mwanachamagit

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"unicode/utf8"

	gogit "github.com/go-git/go-git/v5"
	gogitplumbing "github.com/go-git/go-git/v5/plumbing"

	"github.com/aosanya/mwanachama-backend-git/gormstore"
	"github.com/aosanya/mwanachama-backend-git/models"
)

// loadBlobContentFromBareClone reads the raw content of blob from the local
// full clone recorded on the owning Repository row's BareClonePath field.
// Returns the content string and the encoding ("utf-8" or "base64").
// Returns [ErrBlobContentUnavailable] if no clone path is recorded — the
// caller should trigger [GitManager.FetchBranch] and retry.
func (m *gitManager) loadBlobContentFromBareClone(ctx context.Context, branch models.Branch, blob models.Blob) (content, encoding string, err error) {
	if blob.SHA == "" {
		return "", "", fmt.Errorf("blob %s has no SHA", blob.ID)
	}

	var repoRow gormstore.RepositoryRow
	if err := m.db.WithContext(ctx).Table(m.tables.Repositories).
		Where("id = ?", branch.RepositoryID).First(&repoRow).Error; err != nil {
		return "", "", fmt.Errorf("get repository %s: %w", branch.RepositoryID, err)
	}
	if repoRow.BareClonePath == "" {
		return "", "", ErrBlobContentUnavailable
	}

	repo, err := gogit.PlainOpen(repoRow.BareClonePath)
	if err != nil {
		return "", "", fmt.Errorf("open local clone %s: %w", repoRow.BareClonePath, err)
	}

	hash := gogitplumbing.NewHash(blob.SHA)
	blobObj, err := repo.BlobObject(hash)
	if err != nil {
		return "", "", fmt.Errorf("resolve blob %s in local clone: %w", blob.SHA, err)
	}

	r, err := blobObj.Reader()
	if err != nil {
		return "", "", fmt.Errorf("open blob reader %s: %w", blob.SHA, err)
	}
	defer func() { _ = r.Close() }()

	raw, err := io.ReadAll(r)
	if err != nil {
		return "", "", fmt.Errorf("read blob %s: %w", blob.SHA, err)
	}

	// Detect encoding: treat as binary if the bytes are not valid UTF-8 or
	// contain a null byte (common heuristic used by git itself).
	if bytes.IndexByte(raw, 0) >= 0 || !utf8.Valid(raw) {
		return base64.StdEncoding.EncodeToString(raw), "base64", nil
	}
	return string(raw), "utf-8", nil
}
