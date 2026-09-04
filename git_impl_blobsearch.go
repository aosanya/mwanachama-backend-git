package mwanachamagit

import (
	"context"
	"fmt"
)

// SearchBlobs performs a ranked full-text search over Blob name and content
// fields using the injected [BlobSearcher]. RepositoryName is accepted but
// ignored (matches the upstream contract: the search index covers all repos
// in this deployment's database). Returns an empty slice without error when
// no BlobSearcher is configured (graceful degradation) — the concrete
// Postgres tsvector/ts_rank BlobSearcher is separate follow-up work (GIT
// task G7); this wrapper has no storage dependency of its own.
func (m *gitManager) SearchBlobs(ctx context.Context, req SearchBlobsRequest) ([]BlobSearchResult, error) {
	if req.Query == "" {
		return nil, fmt.Errorf("SearchBlobs: query must not be empty")
	}
	if m.searcher == nil {
		return []BlobSearchResult{}, nil
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	return m.searcher.Search(ctx, req.Query, limit)
}
