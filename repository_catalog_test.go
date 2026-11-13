package mwanachamagit

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"testing"
)

func sampleCatalogEntry() RepositoryCatalogEntry {
	return RepositoryCatalogEntry{
		ID:           "repo-1",
		Name:         "alpha",
		FileName:     "alpha.git.json",
		SizeBytes:    2048,
		LastCommitAt: "2026-11-13T09:30:00Z",
	}
}

func TestRepositoryCatalogDocumentCarriesExactlyThreeKeys(t *testing.T) {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(sampleCatalogEntry().Document()), &decoded); err != nil {
		t.Fatalf("document is not JSON: %v", err)
	}
	keys := make([]string, 0, len(decoded))
	for key := range decoded {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	want := []string{"file_name", "last_commit_at", "size_bytes"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
}

func TestRepositoryCatalogDocumentRoundTripsThroughParse(t *testing.T) {
	entry := sampleCatalogEntry()
	parsed, err := ParseRepositoryCatalogDocument(entry.ID, entry.Name, entry.Document())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed != entry {
		t.Fatalf("parsed = %+v, want %+v", parsed, entry)
	}
}

func TestRepositoryCatalogDocumentWithoutCommitKeepsEmptyLastCommit(t *testing.T) {
	entry := sampleCatalogEntry()
	entry.LastCommitAt = ""
	parsed, err := ParseRepositoryCatalogDocument(entry.ID, entry.Name, entry.Document())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.LastCommitAt != "" {
		t.Fatalf("LastCommitAt = %q, want empty", parsed.LastCommitAt)
	}
}

func TestRepositoryCatalogParseRejectsAnExtraKey(t *testing.T) {
	document := `{"file_name":"a.git.json","size_bytes":1,"last_commit_at":"","owner":"someone"}`
	if _, err := ParseRepositoryCatalogDocument("repo-1", "alpha", document); err == nil {
		t.Fatal("expected an error for the extra key")
	}
}

func TestRepositoryCatalogParseRejectsMalformedJSON(t *testing.T) {
	if _, err := ParseRepositoryCatalogDocument("repo-1", "alpha", "not json"); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func TestMemoryRepositoryCatalogRegistersAndLooksUpAnEntry(t *testing.T) {
	ctx := context.Background()
	catalog := NewMemoryRepositoryCatalog()
	entry := sampleCatalogEntry()
	if err := catalog.Register(ctx, entry); err != nil {
		t.Fatalf("register: %v", err)
	}
	found, err := catalog.Lookup(ctx, entry.ID)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if found != entry {
		t.Fatalf("found = %+v, want %+v", found, entry)
	}
}

func TestMemoryRepositoryCatalogUpdateChangesSizeAndLastCommitOnly(t *testing.T) {
	ctx := context.Background()
	catalog := NewMemoryRepositoryCatalog()
	entry := sampleCatalogEntry()
	if err := catalog.Register(ctx, entry); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := catalog.Update(ctx, entry.ID, 4096, "2026-11-14T10:00:00Z"); err != nil {
		t.Fatalf("update: %v", err)
	}
	found, err := catalog.Lookup(ctx, entry.ID)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	want := entry
	want.SizeBytes = 4096
	want.LastCommitAt = "2026-11-14T10:00:00Z"
	if found != want {
		t.Fatalf("found = %+v, want %+v", found, want)
	}
}

func TestMemoryRepositoryCatalogListsByNameThenID(t *testing.T) {
	ctx := context.Background()
	catalog := NewMemoryRepositoryCatalog()
	for _, entry := range []RepositoryCatalogEntry{
		{ID: "c", Name: "beta"},
		{ID: "b", Name: "beta"},
		{ID: "z", Name: "alpha"},
	} {
		if err := catalog.Register(ctx, entry); err != nil {
			t.Fatalf("register %s: %v", entry.ID, err)
		}
	}
	listed, err := catalog.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var got []string
	for _, entry := range listed {
		got = append(got, entry.Name+"/"+entry.ID)
	}
	want := []string{"alpha/z", "beta/b", "beta/c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestMemoryRepositoryCatalogRemoveForgetsTheEntry(t *testing.T) {
	ctx := context.Background()
	catalog := NewMemoryRepositoryCatalog()
	entry := sampleCatalogEntry()
	if err := catalog.Register(ctx, entry); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := catalog.Remove(ctx, entry.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := catalog.Lookup(ctx, entry.ID); !errors.Is(err, ErrRepositoryNotCatalogued) {
		t.Fatalf("lookup after remove error = %v, want ErrRepositoryNotCatalogued", err)
	}
	listed, err := catalog.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("listed = %v, want none", listed)
	}
}

func TestMemoryRepositoryCatalogRejectsRegisteringTheSameIDTwice(t *testing.T) {
	ctx := context.Background()
	catalog := NewMemoryRepositoryCatalog()
	entry := sampleCatalogEntry()
	if err := catalog.Register(ctx, entry); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := catalog.Register(ctx, entry); !errors.Is(err, ErrRepositoryAlreadyCatalogued) {
		t.Fatalf("second register error = %v, want ErrRepositoryAlreadyCatalogued", err)
	}
}

func TestMemoryRepositoryCatalogReportsUnknownIDsAsNotCatalogued(t *testing.T) {
	ctx := context.Background()
	catalog := NewMemoryRepositoryCatalog()
	if _, err := catalog.Lookup(ctx, "missing"); !errors.Is(err, ErrRepositoryNotCatalogued) {
		t.Fatalf("lookup error = %v, want ErrRepositoryNotCatalogued", err)
	}
	if err := catalog.Update(ctx, "missing", 1, ""); !errors.Is(err, ErrRepositoryNotCatalogued) {
		t.Fatalf("update error = %v, want ErrRepositoryNotCatalogued", err)
	}
	if err := catalog.Remove(ctx, "missing"); !errors.Is(err, ErrRepositoryNotCatalogued) {
		t.Fatalf("remove error = %v, want ErrRepositoryNotCatalogued", err)
	}
}
