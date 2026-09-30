package mwanachamagit

import (
	"os"
	"path/filepath"
	"testing"
)

func examplePaths(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("spec", "examples", "*.git.json"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(paths) < 2 {
		t.Fatalf("the module ships %d domain specs; at least two are needed for domain-neutrality to be exercised", len(paths))
	}
	return paths
}

func TestBlueprintParses(t *testing.T) {
	b, err := Blueprint()
	if err != nil {
		t.Fatalf("blueprint: %v", err)
	}
	if b.Module != "git" {
		t.Fatalf("module = %q, want %q", b.Module, "git")
	}
	if len(b.Objects) != 15 {
		t.Fatalf("the blueprint declares %d objects, want 15", len(b.Objects))
	}
}

func TestEveryShippedSpecLoadsThroughTheBlueprint(t *testing.T) {
	for _, path := range examplePaths(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			s, err := LoadSpec(path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			for _, o := range s.Objects {
				if o.Role == "" {
					continue
				}
				if len(o.Fields) == 0 {
					t.Errorf("role %q arrived with no fields, so the blueprint was not merged in", o.Role)
				}
			}
		})
	}
}

func TestASpecLoadedWithoutTheBlueprintIsRefused(t *testing.T) {
	for _, path := range examplePaths(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		b, err := Blueprint()
		if err != nil {
			t.Fatalf("blueprint: %v", err)
		}
		if _, err := b.Parse(raw); err != nil {
			t.Fatalf("parse %s through the blueprint: %v", path, err)
		}
	}
}
