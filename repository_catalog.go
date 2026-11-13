package mwanachamagit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
)

var ErrRepositoryNotCatalogued = errors.New("repository not catalogued")

var ErrRepositoryAlreadyCatalogued = errors.New("repository already catalogued")

type RepositoryCatalogEntry struct {
	ID           string
	Name         string
	FileName     string
	SizeBytes    int64
	LastCommitAt string
}

type repositoryCatalogDocument struct {
	FileName     string `json:"file_name"`
	SizeBytes    int64  `json:"size_bytes"`
	LastCommitAt string `json:"last_commit_at"`
}

func (e RepositoryCatalogEntry) Document() string {
	raw, err := json.Marshal(repositoryCatalogDocument{
		FileName:     e.FileName,
		SizeBytes:    e.SizeBytes,
		LastCommitAt: e.LastCommitAt,
	})
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func ParseRepositoryCatalogDocument(id, name, document string) (RepositoryCatalogEntry, error) {
	decoder := json.NewDecoder(bytes.NewReader([]byte(document)))
	decoder.DisallowUnknownFields()
	var parsed repositoryCatalogDocument
	if err := decoder.Decode(&parsed); err != nil {
		return RepositoryCatalogEntry{}, err
	}
	return RepositoryCatalogEntry{
		ID:           id,
		Name:         name,
		FileName:     parsed.FileName,
		SizeBytes:    parsed.SizeBytes,
		LastCommitAt: parsed.LastCommitAt,
	}, nil
}

type RepositoryCatalog interface {
	Register(ctx context.Context, entry RepositoryCatalogEntry) error
	Update(ctx context.Context, id string, sizeBytes int64, lastCommitAt string) error
	List(ctx context.Context) ([]RepositoryCatalogEntry, error)
	Lookup(ctx context.Context, id string) (RepositoryCatalogEntry, error)
	Remove(ctx context.Context, id string) error
}
