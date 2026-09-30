package mwanachamagit

import (
	"path/filepath"
	"testing"

	"github.com/aosanya/mwanachama-backend-shared/spec"
)

func TestEveryExampleFitsTheTypes(t *testing.T) {
	for _, path := range examplePaths(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			db := openSpecDB(t)
			s, err := LoadSpec(path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if err := spec.Migrate(db, s); err != nil {
				t.Fatalf("migrate: %v", err)
			}
			if _, err := newStore(db, s); err != nil {
				t.Fatalf("the spec and the Go types disagree: %v", err)
			}
		})
	}
}

func TestEveryRoleIsFilled(t *testing.T) {
	b, err := Blueprint()
	if err != nil {
		t.Fatalf("blueprint: %v", err)
	}
	filled := carriers()
	for _, o := range b.Objects {
		if _, ok := filled[o.Role]; !ok {
			t.Errorf("the blueprint declares role %q and no carrier fills it", o.Role)
		}
	}
	for role := range filled {
		var found bool
		for _, o := range b.Objects {
			if o.Role == role {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("a carrier fills role %q, which the blueprint does not declare", role)
		}
	}
}
