package mwanachamagit

import (
	"context"
	"sort"
	"sync"
)

type MemoryRepositoryCatalog struct {
	mu      sync.Mutex
	entries map[string]RepositoryCatalogEntry
}

var _ RepositoryCatalog = (*MemoryRepositoryCatalog)(nil)

func NewMemoryRepositoryCatalog() *MemoryRepositoryCatalog {
	return &MemoryRepositoryCatalog{entries: map[string]RepositoryCatalogEntry{}}
}

func (c *MemoryRepositoryCatalog) Register(_ context.Context, entry RepositoryCatalogEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[entry.ID]; exists {
		return ErrRepositoryAlreadyCatalogued
	}
	c.entries[entry.ID] = entry
	return nil
}

func (c *MemoryRepositoryCatalog) Update(_ context.Context, id string, sizeBytes int64, lastCommitAt string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, exists := c.entries[id]
	if !exists {
		return ErrRepositoryNotCatalogued
	}
	entry.SizeBytes = sizeBytes
	entry.LastCommitAt = lastCommitAt
	c.entries[id] = entry
	return nil
}

func (c *MemoryRepositoryCatalog) List(_ context.Context) ([]RepositoryCatalogEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	listed := make([]RepositoryCatalogEntry, 0, len(c.entries))
	for _, entry := range c.entries {
		listed = append(listed, entry)
	}
	sort.Slice(listed, func(i, j int) bool {
		if listed[i].Name != listed[j].Name {
			return listed[i].Name < listed[j].Name
		}
		return listed[i].ID < listed[j].ID
	})
	return listed, nil
}

func (c *MemoryRepositoryCatalog) Lookup(_ context.Context, id string) (RepositoryCatalogEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, exists := c.entries[id]
	if !exists {
		return RepositoryCatalogEntry{}, ErrRepositoryNotCatalogued
	}
	return entry, nil
}

func (c *MemoryRepositoryCatalog) Remove(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[id]; !exists {
		return ErrRepositoryNotCatalogued
	}
	delete(c.entries, id)
	return nil
}
