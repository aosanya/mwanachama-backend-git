package mwanachamagit

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"

	gogit "github.com/go-git/go-git/v5"
	gogitplumbing "github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/aosanya/mwanachama-backend-git/models"
)

type fileSnapshot struct {
	storage   *memory.Storage
	repo      models.Repository
	objectRow func(sha string) bool
}

func (s *fileSnapshot) repository() (*gogit.Repository, error) {
	return gogit.Open(s.storage, nil)
}

func (m *gitManager) snapshotFile(ctx context.Context) (*fileSnapshot, error) {
	repoRow, err := m.defaultRepositoryRow(ctx)
	if err != nil {
		return nil, err
	}
	snap := &fileSnapshot{storage: memory.NewStorage(), repo: repoRow}
	if err := m.loadObjects(ctx, snap); err != nil {
		return nil, err
	}
	if err := m.loadRefs(ctx, snap); err != nil {
		return nil, err
	}
	return snap, nil
}

func (s *fileSnapshot) put(kind gogitplumbing.ObjectType, sha, encoded string) error {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil
	}
	if gogitplumbing.ComputeHash(kind, raw).String() != sha {
		return nil
	}
	obj := s.storage.NewEncodedObject()
	obj.SetType(kind)
	writer, err := obj.Writer()
	if err != nil {
		return fmt.Errorf("snapshot %s %s: %w", kind, shortSHA(sha), err)
	}
	if _, err := writer.Write(raw); err != nil {
		_ = writer.Close()
		return fmt.Errorf("snapshot %s %s: %w", kind, shortSHA(sha), err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("snapshot %s %s: %w", kind, shortSHA(sha), err)
	}
	if _, err := s.storage.SetEncodedObject(obj); err != nil {
		return fmt.Errorf("snapshot %s %s: %w", kind, shortSHA(sha), err)
	}
	return nil
}

func (m *gitManager) loadObjects(ctx context.Context, snap *fileSnapshot) error {
	db := m.db.WithContext(ctx)

	var commits []models.Commit
	if err := db.Table(m.tables.Commits).Select("sha", "data").Where("data <> ''").Find(&commits).Error; err != nil {
		return fmt.Errorf("snapshot commits: %w", err)
	}
	for _, c := range commits {
		if err := snap.put(gogitplumbing.CommitObject, c.SHA, c.Data); err != nil {
			return err
		}
	}

	var trees []models.Tree
	if err := db.Table(m.tables.Trees).Select("sha", "data").Where("data <> ''").Find(&trees).Error; err != nil {
		return fmt.Errorf("snapshot trees: %w", err)
	}
	for _, t := range trees {
		if err := snap.put(gogitplumbing.TreeObject, t.SHA, t.Data); err != nil {
			return err
		}
	}

	var blobs []models.Blob
	if err := db.Table(m.tables.Blobs).Select("sha", "data", "size").Where("data <> '' OR size = 0").Find(&blobs).Error; err != nil {
		return fmt.Errorf("snapshot blobs: %w", err)
	}
	for _, b := range blobs {
		if err := snap.put(gogitplumbing.BlobObject, b.SHA, b.Data); err != nil {
			return err
		}
	}
	return nil
}

func (s *fileSnapshot) hasObject(sha string) bool {
	if s.objectRow != nil {
		return s.objectRow(sha)
	}
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
