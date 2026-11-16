package mwanachamagit

import "context"

func (m *gitManager) SearchBlobs(ctx context.Context, req SearchBlobsRequest) ([]BlobSearchResult, error) {
	if req.Query == "" {
		return nil, ErrQueryRequired
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
