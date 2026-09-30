package mwanachamagit

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"
)

type PostgresBlobSearcher struct {
	db    *gorm.DB
	table string // e.g. "git_blobs"
}

func NewPostgresBlobSearcher(db *gorm.DB, s *spec.Spec) (*PostgresBlobSearcher, error) {
	o, ok := s.ByRole(roleBlob)
	if !ok {
		return nil, fmt.Errorf("NewPostgresBlobSearcher: the spec fills no %q role", roleBlob)
	}
	return &PostgresBlobSearcher{db: db, table: s.TableFor(o)}, nil
}

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
`, s.table, BlobFTSExpr)

	var results []BlobSearchResult
	if err := s.db.WithContext(ctx).Raw(q, query, query, query, limit).Scan(&results).Error; err != nil {
		return nil, fmt.Errorf("PostgresBlobSearcher.Search: %w", err)
	}
	if results == nil {
		results = []BlobSearchResult{}
	}
	return results, nil
}
