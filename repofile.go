package mwanachamagit

import (
	"database/sql"
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/events"
	"github.com/aosanya/mwanachama-backend-shared/spec"
)

const repoFileBusyTimeoutMillis = 10000

type RepoFile struct {
	GitManager
	path string
	sql  *sql.DB
}

func withSingleRepository() Option {
	return func(m *gitManager) { m.singleRepository = true }
}

func OpenRepoFile(
	path string,
	s *spec.Spec,
	pub events.Publisher,
	locker RefLocker,
	searcher BlobSearcher,
	opts ...Option,
) (*RepoFile, error) {
	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(%d)", path, repoFileBusyTimeoutMillis)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("OpenRepoFile %s: %w", path, err)
	}
	handle, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("OpenRepoFile %s: %w", path, err)
	}
	handle.SetMaxOpenConns(1)
	if err := Provision(db, s); err != nil {
		_ = handle.Close()
		return nil, fmt.Errorf("OpenRepoFile %s: provision: %w", path, err)
	}
	manager, err := NewGitManager(db, s, pub, locker, searcher, append(opts, withSingleRepository())...)
	if err != nil {
		_ = handle.Close()
		return nil, fmt.Errorf("OpenRepoFile %s: %w", path, err)
	}
	return &RepoFile{GitManager: manager, path: path, sql: handle}, nil
}

func (r *RepoFile) Path() string { return r.path }

func (r *RepoFile) Close() error { return r.sql.Close() }
