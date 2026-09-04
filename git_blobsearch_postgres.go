// git_blobsearch_postgres.go implements [BlobSearcher] against Postgres full-
// text search (tsvector/ts_rank), replacing CodeValdGit's ArangoSearch/BM25
// view (GIT task G7).
//
// Blob content lives inline on each row's jsonb `properties` column in the
// shared [entitygraph]-backed `entities` table (see
// [github.com/aosanya/mwanachama-backend-shared/postgres.DDL]) — there is no
// separate blob-search index table. Search matches directly against
// `to_tsvector('english', name || ' ' || content)`, computed at query time.
// A GIN index over that same expression (see [BlobSearchIndexDDL]) keeps
// this fast without needing a generated column or a sync job to keep one
// up to date.
package mwanachamagit

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/aosanya/mwanachama-backend-shared/postgres"
)

// PostgresBlobSearcher is the Postgres-backed [BlobSearcher]. It reads the
// same `entities` table a [postgres.Backend] configured with the same
// [postgres.TableNames] writes — construct one alongside the other so both
// point at the same table.
type PostgresBlobSearcher struct {
	db    *sql.DB
	table string // e.g. "git_entities"
}

// NewPostgresBlobSearcher constructs a [PostgresBlobSearcher] reading the
// entities table named by tables.Entities. Run [BlobSearchIndexDDL] against
// db once (in the same migration that applies
// [github.com/aosanya/mwanachama-backend-shared/postgres.DDL]) before using it.
func NewPostgresBlobSearcher(db *sql.DB, tables postgres.TableNames) *PostgresBlobSearcher {
	return &PostgresBlobSearcher{db: db, table: tables.Entities}
}

// BlobSearchIndexDDL returns the CREATE INDEX statement backing
// [PostgresBlobSearcher]'s full-text search over the entities table named by
// t.Entities. Append this to the same migration that applies
// [github.com/aosanya/mwanachama-backend-shared/postgres.DDL](t) — it indexes an
// expression over that table's `properties` column, so the table must exist
// first.
//
// The expression is deliberately duplicated between the index and the
// query in [PostgresBlobSearcher.Search] — Postgres only uses an expression
// index when the query contains the exact same expression, so they must
// stay textually identical.
func BlobSearchIndexDDL(t postgres.TableNames) string {
	return fmt.Sprintf(`
CREATE INDEX IF NOT EXISTS %[1]s_blob_fts_idx
    ON %[1]s USING GIN (
        to_tsvector('english', coalesce(properties->>'name', '') || ' ' || coalesce(properties->>'content', ''))
    )
    WHERE type_id = 'Blob' AND NOT deleted;
`, t.Entities)
}

// Search implements [BlobSearcher]. It matches query (via
// plainto_tsquery, so callers pass plain search terms rather than
// tsquery syntax) against each Blob's name and content, and returns results
// ordered by descending relevance.
func (s *PostgresBlobSearcher) Search(ctx context.Context, query string, limit int) ([]BlobSearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	q := fmt.Sprintf(`
SELECT
    id,
    coalesce(properties->>'path', '') AS path,
    coalesce(properties->>'name', '') AS name,
    coalesce(properties->>'extension', '') AS extension,
    ts_headline(
        'english',
        coalesce(properties->>'content', ''),
        plainto_tsquery('english', $1),
        'MaxFragments=1,MaxWords=20,MinWords=5,ShortWord=3'
    ) AS snippet,
    ts_rank(
        to_tsvector('english', coalesce(properties->>'name', '') || ' ' || coalesce(properties->>'content', '')),
        plainto_tsquery('english', $1)
    ) AS score
FROM %s
WHERE type_id = 'Blob'
  AND NOT deleted
  AND to_tsvector('english', coalesce(properties->>'name', '') || ' ' || coalesce(properties->>'content', ''))
      @@ plainto_tsquery('english', $1)
ORDER BY score DESC
LIMIT $2
`, s.table)

	rows, err := s.db.QueryContext(ctx, q, query, limit)
	if err != nil {
		return nil, fmt.Errorf("PostgresBlobSearcher.Search: %w", err)
	}
	defer rows.Close()

	var results []BlobSearchResult
	for rows.Next() {
		var r BlobSearchResult
		if err := rows.Scan(&r.ID, &r.Path, &r.Name, &r.Extension, &r.Snippet, &r.Score); err != nil {
			return nil, fmt.Errorf("PostgresBlobSearcher.Search: scan: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("PostgresBlobSearcher.Search: %w", err)
	}
	if results == nil {
		results = []BlobSearchResult{}
	}
	return results, nil
}
