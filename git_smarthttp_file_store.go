package mwanachamagit

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	gogit "github.com/go-git/go-git/v5"
	gogitplumbing "github.com/go-git/go-git/v5/plumbing"
	gogitobject "github.com/go-git/go-git/v5/plumbing/object"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func (m *gitManager) storePushedObjects(ctx context.Context, repo *gogit.Repository, repoRow models.Repository, tip gogitplumbing.Hash) error {
	now := models.NowRFC3339()
	visited := map[gogitplumbing.Hash]bool{}
	storedTrees := map[gogitplumbing.Hash]bool{}
	queue := []gogitplumbing.Hash{tip}
	for len(queue) > 0 {
		hash := queue[0]
		queue = queue[1:]
		if visited[hash] || hash.IsZero() {
			continue
		}
		visited[hash] = true

		var row models.Commit
		if err := m.db.WithContext(ctx).Table(m.tables.Commits).Where("sha = ?", hash.String()).First(&row).Error; err != nil {
			return fmt.Errorf("find commit row %s: %w", shortSHA(hash.String()), err)
		}
		if row.Data != "" {
			continue
		}
		commit, err := repo.CommitObject(hash)
		if err != nil {
			return fmt.Errorf("read commit %s: %w", shortSHA(hash.String()), err)
		}
		raw, err := encodedBytes(repo, gogitplumbing.CommitObject, hash)
		if err != nil {
			return fmt.Errorf("read commit bytes %s: %w", shortSHA(hash.String()), err)
		}
		tree, err := commit.Tree()
		if err != nil {
			return fmt.Errorf("read tree of %s: %w", shortSHA(hash.String()), err)
		}
		rootID, err := m.upsertTreeIdempotent(ctx, repo, tree, "", now)
		if err != nil {
			return err
		}
		if err := m.storeTreeObjects(ctx, repo, tree, storedTrees); err != nil {
			return err
		}
		err = m.db.WithContext(ctx).Table(m.tables.Commits).Where("id = ?", row.ID).Updates(map[string]any{
			"data":          base64.StdEncoding.EncodeToString(raw),
			"size":          int64(len(raw)),
			"tree_id":       rootID,
			"repository_id": repoRow.ID,
		}).Error
		if err != nil {
			return fmt.Errorf("store commit %s: %w", shortSHA(hash.String()), err)
		}
		queue = append(queue, commit.ParentHashes...)
	}
	return nil
}

type storedTreeEntry struct {
	Name string `json:"name"`
	Mode string `json:"mode"`
	SHA  string `json:"sha"`
}

func (m *gitManager) storeTreeObjects(ctx context.Context, repo *gogit.Repository, tree *gogitobject.Tree, done map[gogitplumbing.Hash]bool) error {
	if done[tree.Hash] {
		return nil
	}
	done[tree.Hash] = true

	raw, err := encodedBytes(repo, gogitplumbing.TreeObject, tree.Hash)
	if err != nil {
		return fmt.Errorf("read tree bytes %s: %w", shortSHA(tree.Hash.String()), err)
	}
	entries := make([]storedTreeEntry, 0, len(tree.Entries))
	for _, e := range tree.Entries {
		entries = append(entries, storedTreeEntry{Name: e.Name, Mode: e.Mode.String(), SHA: e.Hash.String()})
	}
	entriesJSON, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("encode tree entries %s: %w", shortSHA(tree.Hash.String()), err)
	}
	err = m.db.WithContext(ctx).Table(m.tables.Trees).
		Where("sha = ? AND (data = '' OR data IS NULL)", tree.Hash.String()).
		Updates(map[string]any{
			"data":    base64.StdEncoding.EncodeToString(raw),
			"size":    int64(len(raw)),
			"entries": string(entriesJSON),
		}).Error
	if err != nil {
		return fmt.Errorf("store tree %s: %w", shortSHA(tree.Hash.String()), err)
	}

	for _, entry := range tree.Entries {
		if entry.Mode.IsFile() {
			if err := m.storeBlobObject(ctx, repo, entry.Hash); err != nil {
				return err
			}
			continue
		}
		sub, err := repo.TreeObject(entry.Hash)
		if err != nil {
			continue
		}
		if err := m.storeTreeObjects(ctx, repo, sub, done); err != nil {
			return err
		}
	}
	return nil
}

func (m *gitManager) storeBlobObject(ctx context.Context, repo *gogit.Repository, hash gogitplumbing.Hash) error {
	raw, err := encodedBytes(repo, gogitplumbing.BlobObject, hash)
	if err != nil {
		return fmt.Errorf("read blob %s: %w", shortSHA(hash.String()), err)
	}
	if len(raw) == 0 {
		return nil
	}
	content, encoding := string(raw), "utf-8"
	if bytes.IndexByte(raw, 0) >= 0 || !utf8.Valid(raw) {
		content, encoding = base64.StdEncoding.EncodeToString(raw), "base64"
	}
	err = m.db.WithContext(ctx).Table(m.tables.Blobs).
		Where("sha = ? AND (data = '' OR data IS NULL)", hash.String()).
		Updates(map[string]any{
			"data":     base64.StdEncoding.EncodeToString(raw),
			"content":  content,
			"encoding": encoding,
			"size":     int64(len(raw)),
		}).Error
	if err != nil {
		return fmt.Errorf("store blob %s: %w", shortSHA(hash.String()), err)
	}
	return nil
}
