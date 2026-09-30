package mwanachamagit

import (
	"context"
	"testing"
)

func TestListKeywordsReturnsOnlyRoots(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	root, err := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "storage"})
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	if _, err := m.CreateKeyword(ctx, CreateKeywordRequest{Name: "postgres", ParentID: root.ID}); err != nil {
		t.Fatalf("create child: %v", err)
	}

	roots, err := m.ListKeywords(ctx, KeywordFilter{})
	if err != nil {
		t.Fatalf("list roots: %v", err)
	}
	if len(roots) != 1 || roots[0].ID != root.ID {
		t.Fatalf("a filter naming no parent must return the roots and nothing else; got %d: %+v", len(roots), roots)
	}

	children, err := m.ListKeywords(ctx, KeywordFilter{ParentID: root.ID})
	if err != nil {
		t.Fatalf("list children: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("a filter naming a parent must return its children; got %d: %+v", len(children), children)
	}
}
