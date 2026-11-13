package mwanachamagit

import (
	"context"

	"github.com/aosanya/mwanachama-backend-shared/events"
	"github.com/aosanya/mwanachama-backend-shared/spec"
)

func RegistrationCheckFromCatalog(c RepositoryCatalog) RegistrationCheck {
	return func(ctx context.Context, fileName string) (bool, error) {
		entries, err := c.List(ctx)
		if err != nil {
			return false, err
		}
		for _, entry := range entries {
			if entry.FileName == fileName {
				return true, nil
			}
		}
		return false, nil
	}
}

func NewRegisteredRepoFileOpener(
	folder string,
	catalog RepositoryCatalog,
	s *spec.Spec,
	pub events.Publisher,
	locker RefLocker,
	searcher BlobSearcher,
	opts ...Option,
) *RegisteredFileOpener[*RepoFile] {
	return NewRegisteredFileOpener(folder, RegistrationCheckFromCatalog(catalog), func(path string) (*RepoFile, error) {
		return OpenRepoFile(path, s, pub, locker, searcher, opts...)
	})
}
