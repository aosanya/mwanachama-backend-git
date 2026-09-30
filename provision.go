package mwanachamagit

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"
)

const BlobFTSExpr = `to_tsvector('english', coalesce(name, '') || ' ' || coalesce(content, ''))`

func Provision(db *gorm.DB, s *spec.Spec) error {
	if err := spec.Migrate(db, s); err != nil {
		return err
	}
	return syncBlobSearchIndex(db, s)
}

func syncBlobSearchIndex(db *gorm.DB, s *spec.Spec) error {
	if db.Dialector.Name() != "postgres" {
		return nil
	}
	o, ok := s.ByRole(roleBlob)
	if !ok {
		return fmt.Errorf("Provision: the spec fills no %q role", roleBlob)
	}
	table := s.TableFor(o)
	sql := fmt.Sprintf(`
CREATE INDEX IF NOT EXISTS %[1]s_fts_idx
    ON %[1]s USING GIN (%[2]s)
    WHERE NOT deleted
`, table, BlobFTSExpr)
	if err := db.Exec(sql).Error; err != nil {
		return fmt.Errorf("Provision: blob search index: %w", err)
	}
	return nil
}
