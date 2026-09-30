package mwanachamagit_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/aosanya/mwanachama-backend-git"
)

var domainWords = []string{
	"agency", "agencies", "archetype", "sector",
	"engrossment", "chambers", "negotiation",
	"venture", "founder", "fund",
}

func carriesDomainWord(name string) (string, bool) {
	lower := strings.ToLower(name)
	for _, w := range domainWords {
		if strings.Contains(lower, w) {
			return w, true
		}
	}
	return "", false
}

func TestNoDomainWordsInIdentifiers(t *testing.T) {
	for _, dir := range []string{"models", "routes", "."} {
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
			return !strings.HasSuffix(fi.Name(), "_test.go")
		}, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", dir, err)
		}
		seen := map[string]bool{}
		for _, pkg := range pkgs {
			for path, file := range pkg.Files {
				ast.Inspect(file, func(n ast.Node) bool {
					id, ok := n.(*ast.Ident)
					if !ok {
						return true
					}
					w, bad := carriesDomainWord(id.Name)
					if !bad {
						return true
					}
					key := filepath.Base(path) + ":" + id.Name
					if seen[key] {
						return true
					}
					seen[key] = true
					t.Errorf("%s: identifier %q carries the domain word %q — a domain names its own objects in its spec, this module does not learn the word",
						filepath.Base(path), id.Name, w)
					return true
				})
			}
		}
	}
}

func TestNoDomainWordsInStoredValues(t *testing.T) {
	b, err := git.Blueprint()
	if err != nil {
		t.Fatalf("blueprint: %v", err)
	}
	for _, o := range b.Objects {
		if w, bad := carriesDomainWord(o.Role); bad {
			t.Errorf("the role %q carries the domain word %q", o.Role, w)
		}
		for _, f := range o.Fields {
			if w, bad := carriesDomainWord(f.Name); bad {
				t.Errorf("%s.%s carries the domain word %q", o.Role, f.Name, w)
			}
			for _, v := range f.Values {
				if w, bad := carriesDomainWord(v); bad {
					t.Errorf("%s.%s may be stored as %q, which carries the domain word %q — a stored value outlives a rename, which is the worst version of this",
						o.Role, f.Name, v, w)
				}
			}
		}
	}
}
