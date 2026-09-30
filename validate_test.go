package mwanachamagit

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
	"github.com/aosanya/mwanachama-backend-shared/spec"
)

func engineeringSpec(t *testing.T) *spec.Spec {
	t.Helper()
	s, err := LoadSpec(filepath.Join("spec", "examples", "engineering.git.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return s
}

func TestCheckReadsRequiredOffTheSpec(t *testing.T) {
	s := engineeringSpec(t)
	if err := Check(s, RoleRepository, models.Repository{}); err == nil {
		t.Fatal("a repository with no name must be refused, because the spec says name is required")
	}
	if err := Check(s, RoleRepository, models.Repository{Name: "widgets"}); err != nil {
		t.Fatalf("a named repository must pass: %v", err)
	}
}

func TestCheckReadsEnumValuesOffTheSpec(t *testing.T) {
	s := engineeringSpec(t)
	o, ok := s.ByRole(RoleBlobReference)
	if !ok {
		t.Fatal("the engineering domain fills no blob_reference role")
	}
	for _, name := range []string{"tagged_with", "references", "imported_by"} {
		if err := checkField(o, "name", name); err != nil {
			t.Errorf("%q is declared and must pass: %v", name, err)
		}
	}
	for _, name := range []string{"", "supersedes", "TAGGED_WITH"} {
		if err := checkField(o, "name", name); err == nil {
			t.Errorf("%q is not declared and must be refused", name)
		}
	}
}

func TestAnUndeclaredEdgeNameStillAnswersItsSentinel(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	err := m.CreateEdge(ctx, CreateEdgeRequest{RelationshipName: "supersedes"})
	if !errors.Is(err, ErrInvalidRelationship) {
		t.Fatalf("an undeclared relationship must answer ErrInvalidRelationship, got %v", err)
	}
}

func TestASpecNamingAnUnsuppliedPatternIsAnError(t *testing.T) {
	o := spec.Object{
		Name:   "thing",
		Fields: []spec.Field{{Name: "code", Type: spec.TypeString, Matches: "nobody_supplies_this"}},
	}
	err := checkField(o, "code", "anything")
	if err == nil || !strings.Contains(err.Error(), "does not supply") {
		t.Fatalf("a pattern this module does not supply must be an error, not a rule that never runs; got %v", err)
	}
}

func TestEverySpecPatternIsSupplied(t *testing.T) {
	for _, path := range examplePaths(t) {
		s, err := LoadSpec(path)
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		for _, o := range s.Objects {
			for _, f := range o.Fields {
				if f.Matches == "" {
					continue
				}
				if _, ok := patterns[f.Matches]; !ok {
					t.Errorf("%s.%s.%s names the pattern %q, which patterns.go does not supply",
						filepath.Base(path), o.Name, f.Name, f.Matches)
				}
			}
		}
	}
}
