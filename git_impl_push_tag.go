package mwanachamagit

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gogitplumbing "github.com/go-git/go-git/v5/plumbing"

	"github.com/aosanya/mwanachama-backend-git/gormstore"
	"github.com/aosanya/mwanachama-backend-git/models"
)

// tagRefPrefix is the ref namespace a Tag row corresponds to.
const tagRefPrefix = "refs/tags/"

// indexPushedTag files a Tag row for a pushed refs/tags/ ref. A lightweight
// tag's ref points straight at a commit; an annotated tag's at a tag object
// whose message and tagger are copied onto the row. Either way Tag.SHA is the
// commit the tag resolves to, and that commit is indexed first if no branch
// push has brought it in yet.
//
// Tags are immutable in this index: a name already filed is left as it is,
// even if a forced push has since moved the ref in git itself.
func (m *gitManager) indexPushedTag(ctx context.Context, repoName, tagRef, newSHA string) error {
	var repoRow gormstore.RepositoryRow
	if err := m.db.WithContext(ctx).Table(m.tables.Repositories).
		Where("name = ? AND NOT deleted", repoName).First(&repoRow).Error; err != nil {
		return fmt.Errorf("indexPushedTag %s/%s: find repository: %w", repoName, tagRef, err)
	}
	if repoRow.BareClonePath == "" {
		return fmt.Errorf("indexPushedTag %s/%s: repository has no on-disk clone to index from", repoName, tagRef)
	}
	repo, err := gogit.PlainOpen(repoRow.BareClonePath)
	if err != nil {
		return fmt.Errorf("indexPushedTag %s/%s: open %s: %w", repoName, tagRef, repoRow.BareClonePath, err)
	}

	name := strings.TrimPrefix(tagRef, tagRefPrefix)
	var count int64
	if err := m.db.WithContext(ctx).Table(m.tables.Tags).
		Where("repository_id = ? AND name = ? AND NOT deleted", repoRow.ID, name).Count(&count).Error; err != nil {
		return fmt.Errorf("indexPushedTag %s/%s: check existing: %w", repoName, tagRef, err)
	}
	if count > 0 {
		log.Printf("[push-index] repo=%q ref=%q: tag already indexed — left unchanged", repoName, tagRef)
		return nil
	}

	tagHash := gogitplumbing.NewHash(newSHA)
	commitHash := tagHash
	var message, taggerName, taggerAt string
	if obj, err := repo.TagObject(tagHash); err == nil {
		commit, err := obj.Commit()
		if err != nil {
			log.Printf("[push-index] repo=%q ref=%q: annotated tag does not point at a commit — not indexed: %v", repoName, tagRef, err)
			return nil
		}
		commitHash = commit.Hash
		message = strings.TrimRight(obj.Message, "\n")
		taggerName = obj.Tagger.Name
		taggerAt = obj.Tagger.When.UTC().Format(time.RFC3339)
	} else if !errors.Is(err, gogitplumbing.ErrObjectNotFound) {
		return fmt.Errorf("indexPushedTag %s/%s: read tag object: %w", repoName, tagRef, err)
	} else if _, err := repo.CommitObject(tagHash); err != nil {
		log.Printf("[push-index] repo=%q ref=%q: not a commit or tag object — not indexed: %v", repoName, tagRef, err)
		return nil
	}

	if _, err := m.walkNewCommits(ctx, repo, commitHash, ""); err != nil {
		return fmt.Errorf("indexPushedTag %s/%s: index tagged commit: %w", repoName, tagRef, err)
	}
	var commitRow gormstore.CommitRow
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).Where("sha = ?", commitHash.String()).First(&commitRow).Error; err != nil {
		return fmt.Errorf("indexPushedTag %s/%s: find commit row for %s: %w", repoName, tagRef, shortSHA(commitHash.String()), err)
	}

	now := models.NowRFC3339()
	if taggerAt == "" {
		taggerAt = now
	}
	row := gormstore.TagToRow(models.Tag{
		RepositoryID: repoRow.ID,
		Name:         name,
		SHA:          commitRow.SHA,
		Message:      message,
		TaggerName:   taggerName,
		TaggerAt:     taggerAt,
		CreatedAt:    now,
	}, commitRow.ID)
	if err := m.db.WithContext(ctx).Table(m.tables.Tags).Create(&row).Error; err != nil {
		return fmt.Errorf("indexPushedTag %s/%s: create tag: %w", repoName, tagRef, err)
	}
	log.Printf("[push-index] repo=%q ref=%q: indexed tag %s -> %s", repoName, tagRef, name, shortSHA(commitRow.SHA))
	return nil
}
