package mwanachamagit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const repositoryFileSuffix = ".db"

type RegistrationCheck func(ctx context.Context, fileName string) (bool, error)

type RegisteredFileOpener[T any] struct {
	folder     string
	registered RegistrationCheck
	open       func(path string) (T, error)
}

func NewRegisteredFileOpener[T any](folder string, registered RegistrationCheck, open func(path string) (T, error)) *RegisteredFileOpener[T] {
	return &RegisteredFileOpener[T]{folder: folder, registered: registered, open: open}
}

func isPlainRepositoryFileName(fileName string) bool {
	if !strings.HasSuffix(fileName, repositoryFileSuffix) {
		return false
	}
	if strings.ContainsAny(fileName, `/\`) || strings.Contains(fileName, "..") {
		return false
	}
	return filepath.Base(fileName) == fileName
}

func (o *RegisteredFileOpener[T]) Open(ctx context.Context, fileName string) (T, error) {
	var zero T
	if !isPlainRepositoryFileName(fileName) {
		return zero, fmt.Errorf("repository file name %q must be a plain name ending in %s", fileName, repositoryFileSuffix)
	}
	ok, err := o.registered(ctx, fileName)
	if err != nil {
		return zero, fmt.Errorf("check registration of %s: %w", fileName, err)
	}
	if !ok {
		return zero, ErrRepositoryNotRegistered
	}
	return o.open(filepath.Join(o.folder, fileName))
}

func (o *RegisteredFileOpener[T]) ListRegistered(ctx context.Context) ([]string, error) {
	entries, err := os.ReadDir(o.folder)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read repository folder: %w", err)
	}
	names := []string{}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), repositoryFileSuffix) {
			continue
		}
		ok, err := o.registered(ctx, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("check registration of %s: %w", entry.Name(), err)
		}
		if ok {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}
