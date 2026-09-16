// git_impl_push.go implements [GitManager.IndexPushedBranch] (G6 — real
// git-push wire protocol) and the repository-storer resolution the Smart
// HTTP handler (git_smarthttp.go) needs to accept an inbound push at all.
//
// Ported from the original CodeValdGit implementation
// (git_impl_push.go/internal/gitgraph — entitygraph-backed) onto this
// repo's own GORM tables, reusing git_impl_fetchbranch.go's own
// walkCommitsOnly/upsertTreeMetadataWithEdges *pattern* rather than calling
// those functions directly: both are written for FetchBranch's one-time,
// clean-slate walk (a stub branch with zero existing Commit/Tree/Blob rows)
// and unconditionally Create every row they visit. IndexPushedBranch runs
// on every push to a branch that may already be fully indexed, so the
// walks here check for an existing row by SHA first and stop/skip rather
// than re-creating it — otherwise every subsequent push would duplicate
// the branch's entire commit history and file tree.
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

	"github.com/aosanya/mwanachama-backend-git/gormstore"
	"github.com/aosanya/mwanachama-backend-git/models"
)

// IndexPushedBranch indexes the commits a git push (via the Smart HTTP
// receive-pack handler, git_smarthttp.go) just wrote into the repository's
// on-disk bare clone, materialising Commit, Tree, and Blob rows, then
// advances the branch HEAD pointer.
//
// oldSHA is the previous branch tip (all-zeros string for a new branch);
// newSHA is the new tip. Both are the exact values git-receive-pack
// reported for this ref — this method never has to guess them.
//
// Idempotent per (repo, SHA): a commit, tree, or blob whose SHA already has
// a row is left alone, not re-created — see this file's own package doc
// for why that matters here specifically (repeated pushes to the same
// branch), unlike git_impl_fetchbranch.go's one-shot walk.
func (m *gitManager) IndexPushedBranch(ctx context.Context, repoName, branchRef, oldSHA, newSHA string) error {
	start := time.Now()
	log.Printf("[push-index] repo=%q ref=%q old=%s new=%s: start", repoName, branchRef, shortSHA(oldSHA), shortSHA(newSHA))

	var repoRow gormstore.RepositoryRow
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

	newHash := gogitplumbing.NewHash(newSHA)
	newCount, err := m.walkNewCommits(ctx, repo, newHash, oldSHA)
	if err != nil {
		return fmt.Errorf("IndexPushedBranch %s/%s: walk commits: %w", repoName, branchRef, err)
	}
	log.Printf("[push-index] repo=%q ref=%q: indexed %d new commit(s)", repoName, branchRef, newCount)

	tipCommit, err := repo.CommitObject(newHash)
	if err != nil {
		return fmt.Errorf("IndexPushedBranch %s/%s: resolve tip commit %s: %w", repoName, branchRef, shortSHA(newSHA), err)
	}
	tipTree, err := tipCommit.Tree()
	if err != nil {
		return fmt.Errorf("IndexPushedBranch %s/%s: resolve tip tree: %w", repoName, branchRef, err)
	}
	now := models.NowRFC3339()
	rootTreeID, err := m.upsertTreeIdempotent(ctx, repo, tipTree, "", now)
	if err != nil {
		return fmt.Errorf("IndexPushedBranch %s/%s: upsert tree: %w", repoName, branchRef, err)
	}

	var headCommitRow gormstore.CommitRow
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).Where("sha = ?", newSHA).First(&headCommitRow).Error; err != nil {
		return fmt.Errorf("IndexPushedBranch %s/%s: find head commit row for %s: %w", repoName, branchRef, shortSHA(newSHA), err)
	}
	if rootTreeID != "" && headCommitRow.TreeID == nil {
		if err := m.db.WithContext(ctx).Table(m.tables.Commits).Where("id = ?", headCommitRow.ID).
			Update("tree_id", rootTreeID).Error; err != nil {
			log.Printf("[push-index] repo=%q ref=%q: WARNING set commit.tree_id: %v", repoName, branchRef, err)
		}
	}

	branchID, branchErr := m.findOrCreatePushedBranch(ctx, repoRow.ID, repoName, branchRef, now)
	if branchErr != nil {
		return fmt.Errorf("IndexPushedBranch %s/%s: resolve branch row: %w", repoName, branchRef, branchErr)
	}
	if _, err := m.advanceBranchHead(ctx, branchID, headCommitRow.ID, ""); err != nil {
		return fmt.Errorf("IndexPushedBranch %s/%s: advance branch head: %w", repoName, branchRef, err)
	}

	m.publish(ctx, TopicBranchPushed, BranchPushedPayload{
		BranchID: branchID, RepoName: repoName, BranchRef: branchRef, NewSHA: newSHA, NewCommits: newCount,
	})
	log.Printf("[push-index] repo=%q ref=%q new=%s: done in %s", repoName, branchRef, shortSHA(newSHA), time.Since(start))
	return nil
}

// walkNewCommits walks backward from tip (following parent links,
// breadth-first) and batch-inserts a Commit row for every commit reached
// that does not already have one, then wires each one's git_commit_parents
// rows — stopping a path the moment it reaches oldSHA or a commit already
// indexed by an earlier push or fetch. Returns the number of newly-created
// rows.
func (m *gitManager) walkNewCommits(ctx context.Context, repo *gogit.Repository, tip gogitplumbing.Hash, oldSHA string) (int, error) {
	oldHash := gogitplumbing.NewHash(oldSHA)
	now := models.NowRFC3339()

	visited := map[string]bool{}
	var rows []gormstore.CommitRow
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
			// Already indexed by an earlier push/fetch — this path is done;
			// its own parents were already walked when it was first indexed.
			continue
		}

		c, err := repo.CommitObject(h)
		if err != nil {
			return 0, fmt.Errorf("read commit %s: %w", shortSHA(sha), err)
		}
		rows = append(rows, gormstore.CommitToRow(models.Commit{
			SHA:            sha,
			Message:        c.Message,
			AuthorName:     c.Author.Name,
			AuthorEmail:    c.Author.Email,
			AuthorAt:       c.Author.When.UTC().Format(time.RFC3339),
			CommitterName:  c.Committer.Name,
			CommitterEmail: c.Committer.Email,
			CommittedAt:    c.Committer.When.UTC().Format(time.RFC3339),
			CreatedAt:      now,
		}))
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
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).CreateInBatches(&rows, 200).Error; err != nil {
		return 0, err
	}
	if err := m.linkCommitParents(ctx, rows, parentSHAs); err != nil {
		return 0, err
	}
	return len(rows), nil
}

// linkCommitParents writes the git_commit_parents rows for a batch of
// just-inserted commits. Without them a pushed branch's history is
// unreachable: [gormstore.CommitChainIDs]' recursive CTE — the only way Log
// walks backwards from a branch tip — follows this table and nothing else,
// so an unlinked tip reports a one-commit history no matter how many
// commits the push actually carried.
//
// A parent's row id comes from this same batch where the parent was new
// too, and from one SHA lookup where it was already indexed (the branch's
// previous tip, or anything an earlier push or fetch brought in). ParentIndex
// keeps git's own parent order even if a link is skipped, so a merge parent
// can never be mistaken for a first parent.
func (m *gitManager) linkCommitParents(ctx context.Context, rows []gormstore.CommitRow, parentSHAs map[string][]string) error {
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
		var existing []gormstore.CommitRow
		if err := m.db.WithContext(ctx).Table(m.tables.Commits).
			Select("id", "sha").Where("sha IN ?", lookup).Find(&existing).Error; err != nil {
			return fmt.Errorf("resolve parent commit rows: %w", err)
		}
		for _, r := range existing {
			idBySHA[r.SHA] = r.ID
		}
	}

	var links []gormstore.CommitParentRow
	for _, r := range rows {
		for i, sha := range parentSHAs[r.SHA] {
			parentID, ok := idBySHA[sha]
			if !ok {
				log.Printf("[push-index] commit=%s: parent %s has no Commit row — history link skipped", shortSHA(r.SHA), shortSHA(sha))
				continue
			}
			links = append(links, gormstore.CommitParentRow{CommitID: r.ID, ParentID: parentID, ParentIndex: i})
		}
	}
	if len(links) == 0 {
		return nil
	}
	return m.db.WithContext(ctx).Table(m.tables.CommitParents).
		Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(&links, 200).Error
}

// commitRowExists reports whether a Commit row with this SHA is already
// indexed (import, a prior fetch, or a prior push).
func (m *gitManager) commitRowExists(ctx context.Context, sha string) (bool, error) {
	var count int64
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).Where("sha = ?", sha).Count(&count).Error; err != nil {
		return false, fmt.Errorf("commitRowExists %s: %w", shortSHA(sha), err)
	}
	return count > 0, nil
}

// upsertTreeIdempotent is upsertTreeMetadataWithEdges (git_impl_fetchbranch.go),
// made safe to call on every push: a Tree or Blob already carrying a row for
// this (SHA, path) is reused instead of recreated, so an unchanged subtree or
// file pushed again costs one SELECT, not a duplicate row. Returns the row ID
// of tree.
func (m *gitManager) upsertTreeIdempotent(ctx context.Context, repo *gogit.Repository, tree *gogitobject.Tree, pathPrefix, now string) (string, error) {
	treeSHA := tree.Hash.String()
	if existingID, err := m.findRowIDBySHAAndPath(ctx, m.tables.Trees, treeSHA, pathPrefix); err != nil {
		return "", err
	} else if existingID != "" {
		return existingID, nil
	}

	treeRow := gormstore.TreeToRow(models.Tree{SHA: treeSHA, Path: pathPrefix, CreatedAt: now})
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
				Create(&gormstore.TreeBlobRow{TreeID: treeID, BlobID: blobID}).Error; err != nil {
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
			Create(&gormstore.TreeSubtreeRow{TreeID: treeID, SubtreeID: subTreeID}).Error; err != nil {
			log.Printf("[push-index] link tree_subtrees path=%q: %v (non-fatal)", entryPath, err)
		}
	}
	return treeID, nil
}

// upsertBlobIdempotent is upsertBlobMetadataWithID, reused-if-exists —
// see upsertTreeIdempotent's own doc.
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
	row := gormstore.BlobToRow(models.Blob{
		SHA: blobSHA, Path: fullPath, Name: filepath.Base(fullPath), Extension: ext, Size: blobSize, CreatedAt: now,
	})
	if err := m.db.WithContext(ctx).Table(m.tables.Blobs).Create(&row).Error; err != nil {
		return "", fmt.Errorf("create blob %s path=%q: %w", shortSHA(blobSHA), fullPath, err)
	}
	return row.ID, nil
}

// findRowIDBySHAAndPath returns the id of the row in table matching both sha
// and path, or "" if none exists yet.
//
// Path belongs in the key because it is not a property of the content the SHA
// identifies — the same bytes legitimately sit at more than one path (a
// repeated LICENSE, an empty __init__.py, a vendored file). Keying on SHA
// alone filed every occurrence under whichever path was indexed first and left
// the rest with no row at all, so ReadFile could not find them.
func (m *gitManager) findRowIDBySHAAndPath(ctx context.Context, table, sha, path string) (string, error) {
	var id string
	err := m.db.WithContext(ctx).Table(table).Select("id").
		Where("sha = ? AND path = ?", sha, path).Limit(1).Scan(&id).Error
	if err != nil {
		return "", fmt.Errorf("findRowIDBySHAAndPath %s path=%q: %w", shortSHA(sha), path, err)
	}
	return id, nil
}

// findOrCreatePushedBranch resolves branchRef to a Branch row id, creating
// one (a branch pushed for the first time via `git push`, never created
// through CreateBranch first) if none exists yet.
func (m *gitManager) findOrCreatePushedBranch(ctx context.Context, repoID, repoName, branchRef, now string) (string, error) {
	branchName := strings.TrimPrefix(branchRef, "refs/heads/")
	var row gormstore.BranchRow
	err := m.db.WithContext(ctx).Table(m.tables.Branches).
		Where("repository_id = ? AND name = ? AND NOT deleted", repoID, branchName).First(&row).Error
	if err == nil {
		return row.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("find branch %q: %w", branchName, err)
	}

	branchRow := gormstore.BranchToRow(models.Branch{Name: branchName, CreatedAt: now, UpdatedAt: now})
	branchRow.RepositoryID = gormstore.StringToNullable(repoID)
	if err := m.db.WithContext(ctx).Table(m.tables.Branches).Create(&branchRow).Error; err != nil {
		return "", fmt.Errorf("create branch %q: %w", branchName, err)
	}
	log.Printf("[push-index] repo=%q: created branch row for %q (pushed directly, no prior CreateBranch)", repoName, branchName)
	return branchRow.ID, nil
}

// pushClonesRoot is the persistent (never wiped) directory push-target bare
// clones live under — distinct from cloneRootDir's temp scratch directories
// (git_impl_import.go), which are recreated empty on every call and are
// wrong for a repo that must still be there the next time someone pushes.
func pushClonesRoot() string {
	return filepath.Join(os.TempDir(), "mwanachama-backend-git-repos")
}

// openOrInitBareRepo resolves repoName to its on-disk bare clone, git-init
// --bare'ing a fresh one (and persisting BareClonePath onto the Repository
// row) the first time a repo is pushed to before anything else — an
// InitRepo-created repo has a Repository row but no clone on disk yet, and
// neither does a repo git_smarthttp.go's loader auto-creates on first
// contact from a client that never called InitRepo at all.
func (m *gitManager) openOrInitBareRepo(ctx context.Context, repoName string) (*gogit.Repository, error) {
	var repoRow gormstore.RepositoryRow
	err := m.db.WithContext(ctx).Table(m.tables.Repositories).
		Where("name = ? AND NOT deleted", repoName).First(&repoRow).Error
	switch {
	case err == nil:
		// fall through
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
	if err := m.db.WithContext(ctx).Table(m.tables.Repositories).Where("id = ?", repoRow.ID).
		Updates(map[string]any{"bare_clone_path": dir, "updated_at": models.NowRFC3339()}).Error; err != nil {
		return nil, fmt.Errorf("openOrInitBareRepo %s: persist bare_clone_path: %w", repoName, err)
	}
	return repo, nil
}

// shortSHA is sha[:8], or sha unchanged if shorter — every log line above
// uses this instead of a raw full SHA purely for line-length readability.
func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
