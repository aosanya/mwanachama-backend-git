package gormstore

import (
	"fmt"

	"gorm.io/gorm"
)

// TableNames configures which physical tables a [gitManager] (the root
// package's private struct) reads and writes. Sixteen tables: nine node
// tables (one per domain entity type) and four join tables replacing the
// old entitygraph relationships.
type TableNames struct {
	Agencies        string
	Repositories    string
	Branches        string
	MergeRequests   string
	Tags            string
	Commits         string
	CommitParents   string
	Trees           string
	TreeBlobs       string
	TreeSubtrees    string
	Blobs           string
	Keywords        string
	BlobKeywordTags string
	BlobReferences  string
	ImportJobs      string
	FetchBranchJobs string
}

// DefaultTableNames builds the conventional table set for one mounted
// instance of this package, e.g. DefaultTableNames("git") yields
// git_agencies, git_repositories, and so on.
func DefaultTableNames(instance string) TableNames {
	return TableNames{
		Agencies:        instance + "_agencies",
		Repositories:    instance + "_repositories",
		Branches:        instance + "_branches",
		MergeRequests:   instance + "_merge_requests",
		Tags:            instance + "_tags",
		Commits:         instance + "_commits",
		CommitParents:   instance + "_commit_parents",
		Trees:           instance + "_trees",
		TreeBlobs:       instance + "_tree_blobs",
		TreeSubtrees:    instance + "_tree_subtrees",
		Blobs:           instance + "_blobs",
		Keywords:        instance + "_keywords",
		BlobKeywordTags: instance + "_blob_keyword_tags",
		BlobReferences:  instance + "_blob_references",
		ImportJobs:      instance + "_import_jobs",
		FetchBranchJobs: instance + "_fetch_branch_jobs",
	}
}

// Migrate creates or updates all sixteen tables named by t, via GORM's
// AutoMigrate scoped to each table name in turn. Callers run this once at
// startup (or in test setup) before constructing a GitManager with the same
// db and t. On Postgres, also (re)creates the blob full-text-search index —
// skipped silently on other dialects, since only Postgres supports the GIN
// expression index [BlobFTSExpr] backs.
func Migrate(db *gorm.DB, t TableNames) error {
	tables := []struct {
		name  string
		model any
	}{
		{t.Agencies, &AgencyRow{}},
		{t.Repositories, &RepositoryRow{}},
		{t.Branches, &BranchRow{}},
		{t.MergeRequests, &MergeRequestRow{}},
		{t.Tags, &TagRow{}},
		{t.Commits, &CommitRow{}},
		{t.CommitParents, &CommitParentRow{}},
		{t.Trees, &TreeRow{}},
		{t.TreeBlobs, &TreeBlobRow{}},
		{t.TreeSubtrees, &TreeSubtreeRow{}},
		{t.Blobs, &BlobRow{}},
		{t.Keywords, &KeywordRow{}},
		{t.BlobKeywordTags, &BlobKeywordTagRow{}},
		{t.BlobReferences, &BlobReferenceRow{}},
		{t.ImportJobs, &ImportJobRow{}},
		{t.FetchBranchJobs, &FetchBranchJobRow{}},
	}
	for _, tbl := range tables {
		if err := db.Table(tbl.name).AutoMigrate(tbl.model); err != nil {
			return fmt.Errorf("gormstore.Migrate: %s: %w", tbl.name, err)
		}
	}
	return syncBlobSearchIndex(db, t)
}

// syncBlobSearchIndex creates the GIN expression index backing
// PostgresBlobSearcher (see the root package's git_blobsearch_postgres.go),
// skipped silently on any dialect other than postgres.
func syncBlobSearchIndex(db *gorm.DB, t TableNames) error {
	if db.Dialector.Name() != "postgres" {
		return nil
	}
	sql := fmt.Sprintf(`
CREATE INDEX IF NOT EXISTS %[1]s_fts_idx
    ON %[1]s USING GIN (%[2]s)
    WHERE NOT deleted
`, t.Blobs, BlobFTSExpr)
	if err := db.Exec(sql).Error; err != nil {
		return fmt.Errorf("gormstore.Migrate: blob search index: %w", err)
	}
	return nil
}

// BlobFTSExpr is the tsvector expression PostgresBlobSearcher's query and
// [syncBlobSearchIndex]'s GIN index share. Postgres only uses an expression
// index when the query contains the exact same expression, so the two must
// stay textually identical — hence one shared constant rather than two
// hand-copied strings.
const BlobFTSExpr = `to_tsvector('english', coalesce(name, '') || ' ' || coalesce(content, ''))`
