// git_blobsearch_postgres.go implements [BlobSearcher] against Postgres full-
// text search (tsvector/ts_rank), replacing CodeValdGit's ArangoSearch/BM25
// view (GIT task G7).
//
// Blob content now lives on real columns of the blobs table (see
// gormstore.BlobRow) rather than a JSONB properties blob — the only change
// from the entitygraph era, since this file never depended on
// entitygraph.DataManager in the first place (it always queried Postgres
// directly). Search matches against
// to_tsvector('english', name || ' ' || content), computed at query time. A
// GIN index over that same expression ([gormstore.Migrate]'s
// syncBlobSearchIndex) keeps this fast without needing a generated column or
// a sync job to keep one up to date.
package mwanachamagit

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-git/gormstore"
)

// PostgresBlobSearcher is the Postgres-backed [BlobSearcher].
type PostgresBlobSearcher struct {
	db    *gorm.DB
	table string // e.g. "git_blobs"
}

// NewPostgresBlobSearcher constructs a [PostgresBlobSearcher] reading the
// blobs table named by t.Blobs. Run [gormstore.Migrate] against db (which
// creates the backing GIN index) before using it.
func NewPostgresBlobSearcher(db *gorm.DB, t gormstore.TableNames) *PostgresBlobSearcher {
	return &PostgresBlobSearcher{db: db, table: t.Blobs}
}

// Search implements [BlobSearcher]. It matches query (via plainto_tsquery,
// so callers pass plain search terms rather than tsquery syntax) against
// each Blob's name and content, and returns results ordered by descending
// relevance.
func (s *PostgresBlobSearcher) Search(ctx context.Context, query string, limit int) ([]BlobSearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	q := fmt.Sprintf(`
SELECT
    id,
    coalesce(path, '')      AS path,
    coalesce(name, '')      AS name,
    coalesce(extension, '') AS extension,
    ts_headline('english', coalesce(content, ''), plainto_tsquery('english', ?),
        'MaxFragments=1,MaxWords=20,MinWords=5,ShortWord=3') AS snippet,
    ts_rank(%[2]s, plainto_tsquery('english', ?)) AS score
FROM %[1]s
WHERE NOT deleted
  AND %[2]s @@ plainto_tsquery('english', ?)
ORDER BY score DESC
LIMIT ?
`, s.table, gormstore.BlobFTSExpr)

	var results []BlobSearchResult
	if err := s.db.WithContext(ctx).Raw(q, query, query, query, limit).Scan(&results).Error; err != nil {
		return nil, fmt.Errorf("PostgresBlobSearcher.Search: %w", err)
	}
	if results == nil {
		results = []BlobSearchResult{}
	}
	return results, nil
}
