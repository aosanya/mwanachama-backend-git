package mwanachamagit

import (
	"context"
	"fmt"

	"github.com/aosanya/mwanachama-backend-git/models"
)

const allBlobsAtCommitMaxDepth = 5

func (m *gitManager) allBlobsAtCommit(ctx context.Context, commitID string) ([]models.Blob, error) {
	blobs, err := BlobsAtCommit(m.db.WithContext(ctx), m.tables, commitID)
	if err != nil {
		return nil, fmt.Errorf("allBlobsAtCommit %s: %w", commitID, err)
	}
	return blobs, nil
}
