package mwanachamagit

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/aosanya/mwanachama-backend-shared/spec"
)

func openSpecDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return db
}

func TestTwoDomainsCoexist(t *testing.T) {
	db := openSpecDB(t)
	paths := examplePaths(t)

	for _, path := range paths {
		s, err := LoadSpec(path)
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		if err := spec.Migrate(db, s); err != nil {
			t.Fatalf("migrate %s: %v", filepath.Base(path), err)
		}
	}

	var tables []string
	if err := db.Raw(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE '%_spec_table_names'`,
	).Scan(&tables).Error; err != nil {
		t.Fatalf("list tables: %v", err)
	}
	want := 15 * len(paths)
	if len(tables) != want {
		t.Fatalf("%d domains migrated %d tables, want %d — the domains are sharing a table", len(paths), len(tables), want)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	db := openSpecDB(t)
	s, err := LoadSpec(filepath.Join("spec", "examples", "engineering.git.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for i := range 2 {
		if err := spec.Migrate(db, s); err != nil {
			t.Fatalf("migrate pass %d: %v", i+1, err)
		}
	}
}

func TestRequiredFieldsHaveNoDefault(t *testing.T) {
	for _, path := range examplePaths(t) {
		s, err := LoadSpec(path)
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		for _, o := range s.Objects {
			for _, f := range o.Fields {
				if f.Required && f.Default != "" {
					t.Errorf("%s.%s.%s is required and defaulted; the default is exactly what lets an omitted value pass",
						filepath.Base(path), o.Name, f.Name)
				}
			}
		}
	}
}
