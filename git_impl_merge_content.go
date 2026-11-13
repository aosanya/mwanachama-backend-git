package mwanachamagit

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

type mergeSide struct {
	present bool
	usable  bool
	text    string
}

func (m *gitManager) loadMergeSide(ctx context.Context, entry mergeEntry, present bool) (mergeSide, error) {
	if !present {
		return mergeSide{}, nil
	}
	var blob models.Blob
	if err := m.db.WithContext(ctx).Table(m.tables.Blobs).
		Where("id = ?", entry.BlobID).First(&blob).Error; err != nil {
		return mergeSide{}, fmt.Errorf("merge content: read blob %s: %w", entry.BlobID, err)
	}
	return mergeSide{present: true, usable: blobIsMergeableText(blob), text: blob.Content}, nil
}

func blobIsMergeableText(blob models.Blob) bool {
	if blob.Encoding != "" && blob.Encoding != "utf-8" {
		return false
	}
	if blob.Content == "" && blob.Size > 0 {
		return false
	}
	return verifyBlobContent(blob) == nil
}

func mergeSideText(base, ours, theirs mergeSide) (string, bool) {
	for _, side := range []mergeSide{base, ours, theirs} {
		if !side.present || !side.usable {
			return "", false
		}
	}
	merged, conflict := mergeLines(base.text, ours.text, theirs.text)
	return merged, !conflict
}

func (m *gitManager) resolveOverlaps(ctx context.Context, base, ours, theirs mergeTree, overlapping []string) (resolved map[string]string, conflicts []string, err error) {
	resolved = map[string]string{}
	for _, path := range overlapping {
		sides := make([]mergeSide, 0, 3)
		for _, tree := range []mergeTree{base, ours, theirs} {
			entry, present := tree[path]
			side, loadErr := m.loadMergeSide(ctx, entry, present)
			if loadErr != nil {
				return nil, nil, loadErr
			}
			sides = append(sides, side)
		}
		text, clean := mergeSideText(sides[0], sides[1], sides[2])
		if !clean {
			conflicts = append(conflicts, path)
			continue
		}
		resolved[path] = text
	}
	sort.Strings(conflicts)
	return resolved, conflicts, nil
}

func (m *gitManager) storeMergedBlob(ctx context.Context, path, content string) (mergeEntry, error) {
	sha := plumbing.ComputeHash(plumbing.BlobObject, []byte(content)).String()
	row := models.Blob{
		SHA:       sha,
		Path:      path,
		Name:      fileName(path),
		Extension: fileExtension(path),
		Size:      int64(len(content)),
		Encoding:  "utf-8",
		Content:   content,
		CreatedAt: models.NowRFC3339(),
	}
	row.Data = base64.StdEncoding.EncodeToString([]byte(content))
	ensureID(&row)
	if err := m.db.WithContext(ctx).Table(m.tables.Blobs).Create(&row).Error; err != nil {
		return mergeEntry{}, fmt.Errorf("merge content: create blob for %q: %w", path, err)
	}
	return mergeEntry{BlobID: row.ID, SHA: sha}, nil
}

func (m *gitManager) storeMergedBlobs(ctx context.Context, merged mergeTree, resolved map[string]string) error {
	for path, content := range resolved {
		entry, err := m.storeMergedBlob(ctx, path, content)
		if err != nil {
			return err
		}
		merged[path] = entry
	}
	return nil
}
