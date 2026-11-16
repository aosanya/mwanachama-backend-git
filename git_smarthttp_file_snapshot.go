package mwanachamagit

import (
	"context"
	"fmt"
	"io"

	gogit "github.com/go-git/go-git/v5"
	gogitplumbing "github.com/go-git/go-git/v5/plumbing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

type fileSnapshot struct {
	storage *lazyStorage
	repo    models.Repository
}

func (s *fileSnapshot) repository() (*gogit.Repository, error) {
	return gogit.Open(s.storage, nil)
}

func (m *gitManager) defaultRepositoryRow(ctx context.Context) (models.Repository, error) {
	var repoRow models.Repository
	err := m.db.WithContext(ctx).Table(m.tables.Repositories).
		Where("NOT deleted").Order("created_at").First(&repoRow).Error
	if err != nil {
		return repoRow, ErrRepoNotInitialised
	}
	return repoRow, nil
}

func (m *gitManager) snapshotFile(ctx context.Context) (*fileSnapshot, error) {
	repoRow, err := m.defaultRepositoryRow(ctx)
	if err != nil {
		return nil, err
	}
	snap := &fileSnapshot{storage: newLazyStorage(ctx, m), repo: repoRow}
	if err := m.loadRefs(ctx, snap); err != nil {
		return nil, err
	}
	return snap, nil
}

func (s *fileSnapshot) hasObject(sha string) bool {
	return s.storage.HasEncodedObject(gogitplumbing.NewHash(sha)) == nil
}

func (m *gitManager) loadRefs(ctx context.Context, snap *fileSnapshot) error {
	db := m.db.WithContext(ctx)

	var branches []models.Branch
	if err := db.Table(m.tables.Branches).Where("NOT deleted AND sha <> ''").Find(&branches).Error; err != nil {
		return fmt.Errorf("snapshot branches: %w", err)
	}
	names := make([]string, 0, len(branches))
	for _, b := range branches {
		names = append(names, b.Name)
	}
	usable := map[string]bool{}
	for _, name := range usableBranchNames(snap.repo.DefaultBranch, names) {
		usable[name] = true
	}
	for _, b := range branches {
		if !usable[b.Name] || !snap.hasObject(b.SHA) {
			continue
		}
		ref := gogitplumbing.NewHashReference(gogitplumbing.NewBranchReferenceName(b.Name), gogitplumbing.NewHash(b.SHA))
		if err := snap.storage.SetReference(ref); err != nil {
			return fmt.Errorf("snapshot branch %s: %w", b.Name, err)
		}
	}

	var tags []models.Tag
	if err := db.Table(m.tables.Tags).Where("NOT deleted AND sha <> ''").Find(&tags).Error; err != nil {
		return fmt.Errorf("snapshot tags: %w", err)
	}
	for _, tag := range tags {
		if !snap.hasObject(tag.SHA) {
			continue
		}
		ref := gogitplumbing.NewHashReference(gogitplumbing.NewTagReferenceName(tag.Name), gogitplumbing.NewHash(tag.SHA))
		if err := snap.storage.SetReference(ref); err != nil {
			return fmt.Errorf("snapshot tag %s: %w", tag.Name, err)
		}
	}

	if snap.repo.DefaultBranch != "" {
		head := gogitplumbing.NewSymbolicReference(gogitplumbing.HEAD, gogitplumbing.NewBranchReferenceName(snap.repo.DefaultBranch))
		if err := snap.storage.SetReference(head); err != nil {
			return fmt.Errorf("snapshot HEAD: %w", err)
		}
	}
	return nil
}

func encodedBytes(repo *gogit.Repository, kind gogitplumbing.ObjectType, hash gogitplumbing.Hash) ([]byte, error) {
	obj, err := repo.Storer.EncodedObject(kind, hash)
	if err != nil {
		return nil, err
	}
	reader, err := obj.Reader()
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	return io.ReadAll(reader)
}
