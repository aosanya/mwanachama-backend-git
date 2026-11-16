package mwanachamagit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gogitplumbing "github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-git/models"
)

const (
	refusedTagMadeOnce    = "tags are made once; delete one with the file tools"
	refusedTagNotACommit  = "a tag must point at a commit"
	refusedInvalidTagName = "not a valid tag name"
	refusedTagNameBarred  = "this repository does not allow that tag name"
)

type pushRefusal string

func (e pushRefusal) Error() string { return string(e) }

type tagRules struct {
	existing map[string]bool
	allowed  func(string) bool
}

type tagTarget struct {
	commit   gogitplumbing.Hash
	message  string
	tagger   string
	taggedAt string
}

func (m *gitManager) tagRulesFor(ctx context.Context, repoID string) (tagRules, error) {
	var names []string
	err := m.db.WithContext(ctx).Table(m.tables.Tags).
		Where("repository_id = ? AND NOT deleted", repoID).Pluck("name", &names).Error
	if err != nil {
		return tagRules{}, fmt.Errorf("list tags: %w", err)
	}
	patterns, err := m.TagPatterns(ctx)
	if err != nil {
		return tagRules{}, err
	}
	rules := tagRules{existing: map[string]bool{}}
	for _, name := range names {
		rules.existing[name] = true
	}
	rules.allowed = func(name string) bool {
		ok, matchErr := TagNameAllowed(patterns, name)
		return matchErr == nil && ok
	}
	return rules, nil
}

func tagRefusal(rules tagRules, cmd *packp.Command) (string, bool) {
	name := cmd.Name.String()
	short := strings.TrimPrefix(name, tagRefPrefix)
	switch {
	case !cmd.Old.IsZero() || cmd.New.IsZero() || rules.existing[short]:
		return refusedTagMadeOnce, true
	case cmd.Name.Validate() != nil:
		return refusedInvalidTagName, true
	case !rules.allowed(short):
		return refusedTagNameBarred, true
	}
	return "", false
}

func resolveTagTarget(repo *gogit.Repository, hash gogitplumbing.Hash) (tagTarget, error) {
	tag, err := repo.TagObject(hash)
	if err == nil {
		commit, commitErr := tag.Commit()
		if commitErr != nil {
			return tagTarget{}, pushRefusal(refusedTagNotACommit)
		}
		return tagTarget{
			commit:   commit.Hash,
			message:  strings.TrimRight(tag.Message, "\n"),
			tagger:   tag.Tagger.Name,
			taggedAt: tag.Tagger.When.UTC().Format(time.RFC3339),
		}, nil
	}
	if !errors.Is(err, gogitplumbing.ErrObjectNotFound) {
		return tagTarget{}, err
	}
	if _, err := repo.CommitObject(hash); err != nil {
		return tagTarget{}, pushRefusal(refusedTagNotACommit)
	}
	return tagTarget{commit: hash}, nil
}

func (m *gitManager) recordFileTag(ctx context.Context, snap *fileSnapshot, cmd *packp.Command) error {
	name := strings.TrimPrefix(cmd.Name.String(), tagRefPrefix)
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := takeWriteLock(tx, m.tables.Branches); err != nil {
			return err
		}
		scoped := m.withDB(tx)
		repo, err := gogit.Open(snap.storage.readingThrough(ctx, scoped), nil)
		if err != nil {
			return err
		}
		target, err := resolveTagTarget(repo, cmd.New)
		if err != nil {
			return err
		}
		if err := scoped.requireNewTag(ctx, snap.repo.ID, name); err != nil {
			return err
		}
		if _, err := scoped.walkNewCommits(ctx, repo, target.commit, ""); err != nil {
			return fmt.Errorf("index tagged commit: %w", err)
		}
		if err := scoped.storePushedObjects(ctx, repo, snap.repo, target.commit); err != nil {
			return err
		}
		return scoped.createPushedTag(ctx, snap.repo.ID, name, target)
	})
}

func (m *gitManager) requireNewTag(ctx context.Context, repoID, name string) error {
	var count int64
	err := m.db.WithContext(ctx).Table(m.tables.Tags).
		Where("repository_id = ? AND name = ? AND NOT deleted", repoID, name).Count(&count).Error
	if err != nil {
		return fmt.Errorf("check existing tag: %w", err)
	}
	if count > 0 {
		return pushRefusal(refusedTagMadeOnce)
	}
	return nil
}

func (m *gitManager) createPushedTag(ctx context.Context, repoID, name string, target tagTarget) error {
	var commitRow models.Commit
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).
		Where("sha = ?", target.commit.String()).First(&commitRow).Error; err != nil {
		return fmt.Errorf("find commit row for %s: %w", shortSHA(target.commit.String()), err)
	}
	now := models.NowRFC3339()
	if target.taggedAt == "" {
		target.taggedAt = now
	}
	row := models.Tag{
		RepositoryID: repoID,
		Name:         name,
		SHA:          commitRow.SHA,
		Message:      target.message,
		TaggerName:   target.tagger,
		TaggerAt:     target.taggedAt,
		CreatedAt:    now,
		CommitID:     commitRow.ID,
	}
	ensureID(&row)
	if err := m.db.WithContext(ctx).Table(m.tables.Tags).Create(&row).Error; err != nil {
		return fmt.Errorf("create tag: %w", err)
	}
	return nil
}
