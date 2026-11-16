package mwanachamagit

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	gogit "github.com/go-git/go-git/v5"
	gogitplumbing "github.com/go-git/go-git/v5/plumbing"
	gogitobject "github.com/go-git/go-git/v5/plumbing/object"

	"github.com/aosanya/mwanachama-backend-git/models"
)

const branchRefPrefix = "refs/heads/"

func (m *gitManager) IndexPushedBranch(ctx context.Context, repoName, branchRef, oldSHA, newSHA string) error {
	start := time.Now()
	log.Printf("[push-index] repo=%q ref=%q old=%s new=%s: start", repoName, branchRef, shortSHA(oldSHA), shortSHA(newSHA))

	if strings.HasPrefix(branchRef, tagRefPrefix) {
		return m.indexPushedTag(ctx, repoName, branchRef, newSHA)
	}

	if !strings.HasPrefix(branchRef, branchRefPrefix) {
		log.Printf("[push-index] repo=%q ref=%q: not a branch ref — stored in git, not indexed as a Branch", repoName, branchRef)
		return nil
	}

	var repoRow models.Repository
	if err := m.db.WithContext(ctx).Table(m.tables.Repositories).
		Where("name = ? AND NOT deleted", repoName).First(&repoRow).Error; err != nil {
		return fmt.Errorf("IndexPushedBranch %s/%s: find repository: %w", repoName, branchRef, err)
	}
	if repoRow.BareClonePath == "" {
		return fmt.Errorf("IndexPushedBranch %s/%s: repository has no on-disk clone to index from", repoName, branchRef)
	}
	repo, err := gogit.PlainOpen(repoRow.BareClonePath)
	if err != nil {
		return fmt.Errorf("IndexPushedBranch %s/%s: open %s: %w", repoName, branchRef, repoRow.BareClonePath, err)
	}

	return m.indexPushedBranchFrom(ctx, repo, repoRow, branchRef, oldSHA, newSHA, start)
}

var errStalePush = errors.New("the branch moved while you were pushing; fetch and try again")

type indexedPush struct {
	head       models.Commit
	newCommits int
}

func (m *gitManager) indexPushedBranchFrom(ctx context.Context, repo *gogit.Repository, repoRow models.Repository, branchRef, oldSHA, newSHA string, start time.Time) error {
	indexed, err := m.indexPushedObjects(ctx, repo, repoRow, branchRef, oldSHA, newSHA)
	if err != nil {
		return err
	}
	branchID, err := m.findOrCreatePushedBranch(ctx, repoRow.ID, repoRow.Name, branchRef, models.NowRFC3339())
	if err != nil {
		return fmt.Errorf("IndexPushedBranch %s/%s: resolve branch row: %w", repoRow.Name, branchRef, err)
	}
	if _, err := m.advanceBranchHead(ctx, branchID, indexed.head.ID, ""); err != nil {
		return fmt.Errorf("IndexPushedBranch %s/%s: advance branch head: %w", repoRow.Name, branchRef, err)
	}
	m.announcePush(ctx, repoRow, branchRef, newSHA, branchID, indexed, start)
	return nil
}

func (m *gitManager) indexPushedObjects(ctx context.Context, repo *gogit.Repository, repoRow models.Repository, branchRef, oldSHA, newSHA string) (indexedPush, error) {
	repoName := repoRow.Name
	newHash := gogitplumbing.NewHash(newSHA)
	newCount, err := m.walkNewCommits(ctx, repo, newHash, oldSHA)
	if err != nil {
		return indexedPush{}, fmt.Errorf("IndexPushedBranch %s/%s: walk commits: %w", repoName, branchRef, err)
	}
	log.Printf("[push-index] repo=%q ref=%q: indexed %d new commit(s)", repoName, branchRef, newCount)

	tipCommit, err := repo.CommitObject(newHash)
	if err != nil {
		return indexedPush{}, fmt.Errorf("IndexPushedBranch %s/%s: resolve tip commit %s: %w", repoName, branchRef, shortSHA(newSHA), err)
	}
	tipTree, err := tipCommit.Tree()
	if err != nil {
		return indexedPush{}, fmt.Errorf("IndexPushedBranch %s/%s: resolve tip tree: %w", repoName, branchRef, err)
	}
	rootTreeID, err := m.upsertTreeIdempotent(ctx, repo, tipTree, "", models.NowRFC3339())
	if err != nil {
		return indexedPush{}, fmt.Errorf("IndexPushedBranch %s/%s: upsert tree: %w", repoName, branchRef, err)
	}

	var headCommitRow models.Commit
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).Where("sha = ?", newSHA).First(&headCommitRow).Error; err != nil {
		return indexedPush{}, fmt.Errorf("IndexPushedBranch %s/%s: find head commit row for %s: %w", repoName, branchRef, shortSHA(newSHA), err)
	}
	if rootTreeID != "" && headCommitRow.TreeID == "" {
		if err := m.db.WithContext(ctx).Table(m.tables.Commits).Where("id = ?", headCommitRow.ID).
			Update("tree_id", rootTreeID).Error; err != nil {
			log.Printf("[push-index] repo=%q ref=%q: WARNING set commit.tree_id: %v", repoName, branchRef, err)
		}
	}
	return indexedPush{head: headCommitRow, newCommits: newCount}, nil
}

func (m *gitManager) findPushedBranch(ctx context.Context, repoID, branchRef string) (models.Branch, bool, error) {
	var row models.Branch
	err := m.db.WithContext(ctx).Table(m.tables.Branches).
		Where("repository_id = ? AND name = ? AND NOT deleted", repoID, strings.TrimPrefix(branchRef, branchRefPrefix)).
		Limit(1).Find(&row).Error
	if err != nil {
		return models.Branch{}, false, fmt.Errorf("find branch %q: %w", branchRef, err)
	}
	return row, row.ID != "", nil
}

func expectedBranchSHA(oldSHA string) string {
	if gogitplumbing.NewHash(oldSHA).IsZero() {
		return ""
	}
	return oldSHA
}

func (m *gitManager) requireBranchAt(ctx context.Context, repoID, branchRef, oldSHA string) error {
	branch, _, err := m.findPushedBranch(ctx, repoID, branchRef)
	if err != nil {
		return err
	}
	if branch.SHA != expectedBranchSHA(oldSHA) {
		return errStalePush
	}
	return nil
}

func (m *gitManager) movePushedBranch(ctx context.Context, repoRow models.Repository, branchRef, oldSHA string, head models.Commit) (string, error) {
	repoName := repoRow.Name
	if err := m.requireBranchAt(ctx, repoRow.ID, branchRef, oldSHA); err != nil {
		return "", err
	}
	branchID, err := m.findOrCreatePushedBranch(ctx, repoRow.ID, repoName, branchRef, models.NowRFC3339())
	if err != nil {
		return "", fmt.Errorf("IndexPushedBranch %s/%s: resolve branch row: %w", repoName, branchRef, err)
	}
	if err := m.moveBranchHeadIfUnmoved(ctx, branchID, head, oldSHA); err != nil {
		if errors.Is(err, errStalePush) {
			return "", err
		}
		return "", fmt.Errorf("IndexPushedBranch %s/%s: advance branch head: %w", repoName, branchRef, err)
	}
	return branchID, nil
}

func (m *gitManager) moveBranchHeadIfUnmoved(ctx context.Context, branchID string, head models.Commit, oldSHA string) error {
	updates := map[string]any{"head_commit_id": head.ID, "updated_at": models.NowRFC3339()}
	if head.SHA != "" {
		updates["sha"] = head.SHA
	}
	query := m.db.WithContext(ctx).Table(m.tables.Branches).Where("id = ?", branchID)
	result := query.Where("COALESCE(sha, '') = ?", expectedBranchSHA(oldSHA)).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errStalePush
	}
	return nil
}

func (m *gitManager) announcePush(ctx context.Context, repoRow models.Repository, branchRef, newSHA, branchID string, indexed indexedPush, start time.Time) {
	m.publish(ctx, TopicBranchPushed, BranchPushedPayload{
		BranchID: branchID, RepoName: repoRow.Name, BranchRef: branchRef, NewSHA: newSHA, NewCommits: indexed.newCommits,
	})
	log.Printf("[push-index] repo=%q ref=%q new=%s: done in %s", repoRow.Name, branchRef, shortSHA(newSHA), time.Since(start))
}

func (m *gitManager) walkNewCommits(ctx context.Context, repo *gogit.Repository, tip gogitplumbing.Hash, oldSHA string) (int, error) {
	oldHash := gogitplumbing.NewHash(oldSHA)
	now := models.NowRFC3339()

	visited := map[string]bool{}
	var rows []models.Commit
	parentSHAs := map[string][]string{}
	queue := []gogitplumbing.Hash{tip}
	for len(queue) > 0 {
		h := queue[0]
		queue = queue[1:]
		sha := h.String()
		if visited[sha] || h == oldHash || h.IsZero() {
			continue
		}
		visited[sha] = true

		exists, err := m.commitRowExists(ctx, sha)
		if err != nil {
			return 0, err
		}
		if exists {
			continue
		}

		c, err := repo.CommitObject(h)
		if err != nil {
			return 0, fmt.Errorf("read commit %s: %w", shortSHA(sha), err)
		}
		rows = append(rows, models.Commit{
			SHA:            sha,
			Message:        c.Message,
			AuthorName:     c.Author.Name,
			AuthorEmail:    c.Author.Email,
			AuthorAt:       c.Author.When.UTC().Format(time.RFC3339),
			CommitterName:  c.Committer.Name,
			CommitterEmail: c.Committer.Email,
			CommittedAt:    c.Committer.When.UTC().Format(time.RFC3339),
			CreatedAt:      now,
		})
		shas := make([]string, 0, len(c.ParentHashes))
		for _, p := range c.ParentHashes {
			shas = append(shas, p.String())
			if !visited[p.String()] {
				queue = append(queue, p)
			}
		}
		parentSHAs[sha] = shas
	}
	if len(rows) == 0 {
		return 0, nil
	}
	ensureID(&rows)
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).CreateInBatches(&rows, 200).Error; err != nil {
		return 0, err
	}
	if err := m.linkCommitParents(ctx, rows, parentSHAs); err != nil {
		return 0, err
	}
	return len(rows), nil
}

func (m *gitManager) linkCommitParents(ctx context.Context, rows []models.Commit, parentSHAs map[string][]string) error {
	idBySHA := make(map[string]string, len(rows))
	for _, r := range rows {
		idBySHA[r.SHA] = r.ID
	}

	var lookup []string
	for _, shas := range parentSHAs {
		for _, sha := range shas {
			if _, ok := idBySHA[sha]; !ok {
				lookup = append(lookup, sha)
			}
		}
	}
	if len(lookup) > 0 {
		var existing []models.Commit
		if err := m.db.WithContext(ctx).Table(m.tables.Commits).
			Select("id", "sha").Where("sha IN ?", lookup).Find(&existing).Error; err != nil {
			return fmt.Errorf("resolve parent commit rows: %w", err)
		}
		for _, r := range existing {
			idBySHA[r.SHA] = r.ID
		}
	}

	var links []models.CommitParent
	for _, r := range rows {
		for i, sha := range parentSHAs[r.SHA] {
			parentID, ok := idBySHA[sha]
			if !ok {
				log.Printf("[push-index] commit=%s: parent %s has no Commit row — history link skipped", shortSHA(r.SHA), shortSHA(sha))
				continue
			}
			links = append(links, models.CommitParent{CommitID: r.ID, ParentID: parentID, ParentIndex: i})
		}
	}
	if len(links) == 0 {
		return nil
	}
	return m.db.WithContext(ctx).Table(m.tables.CommitParents).
		Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(&links, 200).Error
}

func (m *gitManager) commitRowExists(ctx context.Context, sha string) (bool, error) {
	var count int64
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).Where("sha = ?", sha).Count(&count).Error; err != nil {
		return false, fmt.Errorf("commitRowExists %s: %w", shortSHA(sha), err)
	}
	return count > 0, nil
}

func (m *gitManager) upsertTreeIdempotent(ctx context.Context, repo *gogit.Repository, tree *gogitobject.Tree, pathPrefix, now string) (string, error) {
	treeSHA := tree.Hash.String()
	if existingID, err := m.findRowIDBySHAAndPath(ctx, m.tables.Trees, treeSHA, pathPrefix); err != nil {
		return "", err
	} else if existingID != "" {
		return existingID, nil
	}

	treeRow := models.Tree{SHA: treeSHA, Path: pathPrefix, CreatedAt: now}
	ensureID(&treeRow)
	if err := m.db.WithContext(ctx).Table(m.tables.Trees).Create(&treeRow).Error; err != nil {
		return "", fmt.Errorf("create tree %s path=%q: %w", shortSHA(treeSHA), pathPrefix, err)
	}
	treeID := treeRow.ID

	for _, entry := range tree.Entries {
		if ctx.Err() != nil {
			return treeID, ctx.Err()
		}
		entryPath := entry.Name
		if pathPrefix != "" {
			entryPath = pathPrefix + "/" + entry.Name
		}
		if entry.Mode.IsFile() {
			blobID, err := m.upsertBlobIdempotent(ctx, repo, entry, entryPath, now)
			if err != nil {
				return treeID, err
			}
			if err := m.db.WithContext(ctx).Table(m.tables.TreeBlobs).
				Clauses(clause.OnConflict{DoNothing: true}).
				Create(&models.TreeBlob{TreeID: treeID, BlobID: blobID}).Error; err != nil {
				log.Printf("[push-index] link tree_blobs path=%q: %v (non-fatal)", entryPath, err)
			}
			continue
		}
		subTree, err := repo.TreeObject(entry.Hash)
		if err != nil {
			log.Printf("[push-index] SKIP subtree path=%q sha=%s: TreeObject err=%v", entryPath, shortSHA(entry.Hash.String()), err)
			continue
		}
		subTreeID, err := m.upsertTreeIdempotent(ctx, repo, subTree, entryPath, now)
		if err != nil {
			return treeID, err
		}
		if err := m.db.WithContext(ctx).Table(m.tables.TreeSubtrees).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(&models.TreeSubtree{TreeID: treeID, SubtreeID: subTreeID}).Error; err != nil {
			log.Printf("[push-index] link tree_subtrees path=%q: %v (non-fatal)", entryPath, err)
		}
	}
	return treeID, nil
}

func (m *gitManager) upsertBlobIdempotent(ctx context.Context, repo *gogit.Repository, entry gogitobject.TreeEntry, fullPath, now string) (string, error) {
	blobSHA := entry.Hash.String()
	if existingID, err := m.findRowIDBySHAAndPath(ctx, m.tables.Blobs, blobSHA, fullPath); err != nil {
		return "", err
	} else if existingID != "" {
		return existingID, nil
	}

	var blobSize int64
	if blobObj, err := repo.BlobObject(entry.Hash); err == nil {
		blobSize = blobObj.Size
	}
	ext := strings.TrimPrefix(filepath.Ext(entry.Name), ".")
	row := models.Blob{
		SHA: blobSHA, Path: fullPath, Name: filepath.Base(fullPath), Extension: ext, Size: blobSize, CreatedAt: now,
	}
	ensureID(&row)
	if err := m.db.WithContext(ctx).Table(m.tables.Blobs).Create(&row).Error; err != nil {
		return "", fmt.Errorf("create blob %s path=%q: %w", shortSHA(blobSHA), fullPath, err)
	}
	return row.ID, nil
}

func (m *gitManager) findRowIDBySHAAndPath(ctx context.Context, table, sha, path string) (string, error) {
	var id string
	err := m.db.WithContext(ctx).Table(table).Select("id").
		Where("sha = ? AND path = ?", sha, path).Limit(1).Scan(&id).Error
	if err != nil {
		return "", fmt.Errorf("findRowIDBySHAAndPath %s path=%q: %w", shortSHA(sha), path, err)
	}
	return id, nil
}

func (m *gitManager) findOrCreatePushedBranch(ctx context.Context, repoID, repoName, branchRef, now string) (string, error) {
	branchName := strings.TrimPrefix(branchRef, branchRefPrefix)
	var row models.Branch
	err := m.db.WithContext(ctx).Table(m.tables.Branches).
		Where("repository_id = ? AND name = ? AND NOT deleted", repoID, branchName).First(&row).Error
	if err == nil {
		return row.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("find branch %q: %w", branchName, err)
	}

	branchRow := models.Branch{Name: branchName, CreatedAt: now, UpdatedAt: now}
	branchRow.RepositoryID = repoID
	ensureID(&branchRow)
	if err := m.db.WithContext(ctx).Table(m.tables.Branches).Create(&branchRow).Error; err != nil {
		return "", fmt.Errorf("create branch %q: %w", branchName, err)
	}
	log.Printf("[push-index] repo=%q: created branch row for %q (pushed directly, no prior CreateBranch)", repoName, branchName)
	return branchRow.ID, nil
}

func pushClonesRoot() string {
	return filepath.Join(os.TempDir(), "mwanachama-backend-git-repos")
}

func (m *gitManager) openOrInitBareRepo(ctx context.Context, repoName string) (*gogit.Repository, error) {
	var repoRow models.Repository
	err := m.db.WithContext(ctx).Table(m.tables.Repositories).
		Where("name = ? AND NOT deleted", repoName).First(&repoRow).Error
	switch {
	case err == nil:
	case errors.Is(err, gorm.ErrRecordNotFound):
		if _, initErr := m.InitRepo(ctx, CreateRepoRequest{Name: repoName}); initErr != nil && !errors.Is(initErr, ErrRepoAlreadyExists) {
			return nil, fmt.Errorf("openOrInitBareRepo %s: auto-InitRepo: %w", repoName, initErr)
		}
		if err := m.db.WithContext(ctx).Table(m.tables.Repositories).
			Where("name = ? AND NOT deleted", repoName).First(&repoRow).Error; err != nil {
			return nil, fmt.Errorf("openOrInitBareRepo %s: re-read after InitRepo: %w", repoName, err)
		}
	default:
		return nil, fmt.Errorf("openOrInitBareRepo %s: %w", repoName, err)
	}

	if repoRow.BareClonePath != "" {
		repo, err := gogit.PlainOpen(repoRow.BareClonePath)
		if err == nil {
			return repo, nil
		}
		log.Printf("[push-index] repo=%q: WARNING BareClonePath %s unreadable (%v) — re-initialising", repoName, repoRow.BareClonePath, err)
	}

	dir := filepath.Join(pushClonesRoot(), repoRow.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("openOrInitBareRepo %s: mkdir %s: %w", repoName, dir, err)
	}
	repo, err := gogit.PlainInit(dir, true /* bare */)
	if err != nil {
		return nil, fmt.Errorf("openOrInitBareRepo %s: git init --bare %s: %w", repoName, dir, err)
	}
	if repoRow.DefaultBranch != "" {
		headRef := gogitplumbing.NewSymbolicReference(gogitplumbing.HEAD, gogitplumbing.NewBranchReferenceName(repoRow.DefaultBranch))
		if err := repo.Storer.SetReference(headRef); err != nil {
			log.Printf("[push-index] repo=%q: WARNING set HEAD to %q: %v", repoName, repoRow.DefaultBranch, err)
		}
	}
	if err := m.db.WithContext(ctx).Table(m.tables.Repositories).Where("id = ?", repoRow.ID).
		Updates(map[string]any{"bare_clone_path": dir, "updated_at": models.NowRFC3339()}).Error; err != nil {
		return nil, fmt.Errorf("openOrInitBareRepo %s: persist bare_clone_path: %w", repoName, err)
	}
	return repo, nil
}

func ensureHEADResolves(repo *gogit.Repository, preferred string) (bool, error) {
	if _, err := repo.Reference(gogitplumbing.HEAD, true); err == nil {
		return false, nil
	}

	var target gogitplumbing.ReferenceName
	if preferred != "" {
		name := gogitplumbing.NewBranchReferenceName(preferred)
		if _, err := repo.Storer.Reference(name); err == nil {
			target = name
		}
	}
	if target == "" {
		iter, err := repo.Branches()
		if err != nil {
			return false, fmt.Errorf("ensureHEADResolves: list branches: %w", err)
		}
		defer iter.Close()
		if err := iter.ForEach(func(r *gogitplumbing.Reference) error {
			if target == "" {
				target = r.Name()
			}
			return nil
		}); err != nil {
			return false, fmt.Errorf("ensureHEADResolves: walk branches: %w", err)
		}
	}
	if target == "" {
		return false, nil
	}
	if err := repo.Storer.SetReference(gogitplumbing.NewSymbolicReference(gogitplumbing.HEAD, target)); err != nil {
		return false, fmt.Errorf("ensureHEADResolves: set HEAD to %s: %w", target, err)
	}
	return true, nil
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
