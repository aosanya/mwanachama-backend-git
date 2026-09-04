// git_impl_converters.go — shared graph lookup utilities for [gitManager].
//
// Entity<->domain conversion now lives in gormstore's *ToRow/*FromRow
// functions; this file keeps only the one helper that isn't a pure
// conversion — allBlobsAtCommit, which issues a real query.
package mwanachamagit

import (
	"context"
	"fmt"

	"github.com/aosanya/mwanachama-backend-git/gormstore"
	"github.com/aosanya/mwanachama-backend-git/models"
)

// allBlobsAtCommitMaxDepth bounds [gormstore.BlobsAtCommit]'s tree-nesting
// recursion: commit -> root tree -> subtree* (up to 3 further levels).
// Comfortably covers realistic directory nesting; deeper trees simply stop
// contributing blobs past that point rather than erroring — unchanged
// behavior from the entitygraph-era allBlobsAtCommitMaxDepth=5 hop budget
// (see gormstore.BlobsAtCommit's doc for the hop-by-hop equivalence).
const allBlobsAtCommitMaxDepth = 5

// allBlobsAtCommit returns all Blob rows reachable from the commit's tree.
func (m *gitManager) allBlobsAtCommit(ctx context.Context, commitID string) ([]models.Blob, error) {
	rows, err := gormstore.BlobsAtCommit(m.db.WithContext(ctx), m.tables, commitID)
	if err != nil {
		return nil, fmt.Errorf("allBlobsAtCommit %s: %w", commitID, err)
	}
	blobs := make([]models.Blob, len(rows))
	for i, r := range rows {
		blobs[i] = gormstore.BlobFromRow(r)
	}
	return blobs, nil
}
