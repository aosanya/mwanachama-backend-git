package mwanachamagit

import (
	"context"
	"path/filepath"
)

type RepoFiles struct {
	folder string
	opener *RegisteredFileOpener[string]
	cache  *RepoFileCache[*RepoFile]
}

func NewRepoFiles(folder string, max int, registered RegistrationCheck, open func(path string) (*RepoFile, error)) (*RepoFiles, error) {
	cache, err := NewRepoFileCache[*RepoFile](max, open, func(rf *RepoFile) error { return rf.Close() })
	if err != nil {
		return nil, err
	}
	opener := NewRegisteredFileOpener[string](folder, registered, func(path string) (string, error) { return path, nil })
	return &RepoFiles{folder: folder, opener: opener, cache: cache}, nil
}

func (f *RepoFiles) With(ctx context.Context, fileName string, use func(*RepoFile) error) error {
	path, err := f.opener.Open(ctx, fileName)
	if err != nil {
		return err
	}
	return f.cache.With(path, use)
}

func (f *RepoFiles) Open() int { return f.cache.Open() }

func (f *RepoFiles) Close() error { return f.cache.Close() }

func (f *RepoFiles) Evict(fileName string) error {
	return f.cache.Evict(filepath.Join(f.folder, fileName))
}
