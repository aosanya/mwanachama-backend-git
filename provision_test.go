package mwanachamagit

import (
	"path/filepath"
	"slices"
	"sort"
	"strings"
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

func tableNames(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var names []string
	if err := db.Raw(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE '%_spec_table_names'`,
	).Scan(&names).Error; err != nil {
		t.Fatalf("list tables: %v", err)
	}
	return names
}

func migrateSpecFile(t *testing.T, db *gorm.DB, path string) {
	t.Helper()
	s, err := LoadSpec(path)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	if err := spec.Migrate(db, s); err != nil {
		t.Fatalf("migrate %s: %v", filepath.Base(path), err)
	}
}

func tableNamesByDomain(t *testing.T) map[string][]string {
	t.Helper()
	byDomain := map[string][]string{}
	for _, path := range examplePaths(t) {
		db := openSpecDB(t)
		migrateSpecFile(t, db, path)
		byDomain[filepath.Base(path)] = tableNames(t, db)
	}
	return byDomain
}

func sharedNames(sets map[string][]string) []string {
	domains := make([]string, 0, len(sets))
	for domain := range sets {
		domains = append(domains, domain)
	}
	sort.Strings(domains)

	owners := map[string][]string{}
	for _, domain := range domains {
		seen := map[string]bool{}
		for _, name := range sets[domain] {
			if seen[name] {
				continue
			}
			seen[name] = true
			owners[name] = append(owners[name], domain)
		}
	}

	var shared []string
	for name, domainsWithName := range owners {
		if len(domainsWithName) > 1 {
			shared = append(shared, name+" is created by "+strings.Join(domainsWithName, " and "))
		}
	}
	sort.Strings(shared)
	return shared
}

func TestTwoDomainsCoexist(t *testing.T) {
	want := 0
	for _, names := range tableNamesByDomain(t) {
		want += len(names)
	}

	db := openSpecDB(t)
	paths := examplePaths(t)
	for _, path := range paths {
		migrateSpecFile(t, db, path)
	}

	if got := len(tableNames(t, db)); got != want {
		t.Fatalf("%d domains migrated into one database made %d tables, but migrated alone they make %d", len(paths), got, want)
	}
}

func TestDomainsShareNoTable(t *testing.T) {
	sets := tableNamesByDomain(t)
	if len(sets) < 2 {
		t.Fatalf("need at least two example domains, found %d", len(sets))
	}
	for _, overlap := range sharedNames(sets) {
		t.Errorf("%s", overlap)
	}
}

func TestSharedNames(t *testing.T) {
	overlapping := map[string][]string{
		"chambers":    {"a", "b", "c"},
		"engineering": {"c", "d"},
		"third":       {"e", "b"},
	}
	got := sharedNames(overlapping)
	want := []string{
		"b is created by chambers and third",
		"c is created by chambers and engineering",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("sharedNames(overlapping) = %q, want %q", got, want)
	}

	disjoint := map[string][]string{
		"chambers":    {"a", "b"},
		"engineering": {"c", "d"},
	}
	if got := sharedNames(disjoint); len(got) != 0 {
		t.Fatalf("sharedNames(disjoint) = %q, want none", got)
	}

	repeatedWithinOne := map[string][]string{"chambers": {"a", "a"}, "engineering": {"b"}}
	if got := sharedNames(repeatedWithinOne); len(got) != 0 {
		t.Fatalf("a name repeated inside one domain is not shared, got %q", got)
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
