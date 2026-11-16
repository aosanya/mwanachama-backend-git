package mwanachamagit

import (
	"context"
	"errors"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"gorm.io/gorm"
)

func (m *gitManager) withDB(db *gorm.DB) *gitManager {
	scoped := *m
	scoped.db = db
	return &scoped
}

func (s *lazyStorage) readingThrough(ctx context.Context, m *gitManager) *lazyStorage {
	scoped := newLazyStorage(ctx, m)
	scoped.Storage = s.Storage
	return scoped
}

func (m *gitManager) recordFilePush(ctx context.Context, snap *fileSnapshot, cmd *packp.Command) error {
	start := time.Now()
	repoRow := snap.repo
	branchRef, oldSHA, newSHA := cmd.Name.String(), cmd.Old.String(), cmd.New.String()
	var indexed indexedPush
	var branchID string
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := takeWriteLock(tx, m.tables.Branches); err != nil {
			return err
		}
		scoped := m.withDB(tx)
		repo, err := gogit.Open(snap.storage.readingThrough(ctx, scoped), nil)
		if err != nil {
			return err
		}
		if err := scoped.requireBranchAt(ctx, repoRow.ID, branchRef, oldSHA); err != nil {
			return err
		}
		if indexed, err = scoped.indexPushedObjects(ctx, repo, repoRow, branchRef, oldSHA, newSHA); err != nil {
			return err
		}
		if err = scoped.storePushedObjects(ctx, repo, repoRow, cmd.New); err != nil {
			return err
		}
		branchID, err = scoped.movePushedBranch(ctx, repoRow, branchRef, oldSHA, indexed.head)
		return err
	})
	if err != nil {
		return err
	}
	m.recordCommitInCatalog(ctx, repoRow.ID, indexed.head)
	m.announcePush(ctx, repoRow, branchRef, newSHA, branchID, indexed, start)
	return nil
}

func takeWriteLock(tx *gorm.DB, table string) error {
	return tx.Table(table).Where("1 = 0").UpdateColumn("id", gorm.Expr("id")).Error
}

func acceptedRefs(status *packp.ReportStatus) map[string]bool {
	accepted := map[string]bool{}
	if status.UnpackStatus != "ok" {
		return accepted
	}
	for _, cs := range status.CommandStatuses {
		if cs.Status == "ok" {
			accepted[cs.ReferenceName.String()] = true
		}
	}
	return accepted
}

func pushFailureMessage(err error) string {
	if errors.Is(err, errStalePush) {
		return errStalePush.Error()
	}
	return "the hub could not record this push: " + err.Error()
}
