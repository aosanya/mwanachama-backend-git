// git_impl_fetchbranch.go implements the lazy import v2 on-demand branch fetch
// methods on [gitManager] (GIT-023d):
//
//   - [GitManager.FetchBranch] — creates a FetchBranchJob row, transitions
//     the Branch status to "fetching", and launches a background goroutine that
//     deepens (or re-clones) the bare repository, walks the full commit history
//     and the tip-commit tree (blob metadata only), then transitions the
//     branch to "fetched" or "fetch_failed".
//   - [GitManager.GetFetchBranchStatus] — returns the current state of a fetch job.
package mwanachamagit

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	gogit "github.com/go-git/go-git/v5"
	gogitplumbing "github.com/go-git/go-git/v5/plumbing"
	gogitobject "github.com/go-git/go-git/v5/plumbing/object"

	"github.com/aosanya/mwanachama-backend-git/gormstore"
	"github.com/aosanya/mwanachama-backend-git/models"
)

// fetchJobStatus values for the FetchBranchJob row "status" column.
const (
	fetchJobStatusPending   = "pending"
	fetchJobStatusRunning   = "running"
	fetchJobStatusCompleted = "completed"
	fetchJobStatusFailed    = "failed"
)

// fetchCancelEntry holds the cancel function for an in-flight fetch goroutine.
type fetchCancelEntry struct {
	cancel context.CancelFunc
}

// fetchJobsMu guards fetchJobs.
var fetchJobsMu sync.Mutex

// fetchJobs maps jobID → cancel entry for all active fetch goroutines.
var fetchJobs = make(map[string]fetchCancelEntry)

// FetchBranch triggers an async on-demand fetch of the full commit history
// and tip-commit file tree for a branch that is currently in "stub" status.
// Returns immediately with a [models.FetchBranchJob]. Returns
// [ErrBranchAlreadyFetched] if the branch status is "fetching" or "fetched".
func (m *gitManager) FetchBranch(ctx context.Context, req FetchBranchRequest) (models.FetchBranchJob, error) {
	var branchRow gormstore.BranchRow
	err := m.db.WithContext(ctx).Table(m.tables.Branches).
		Where("id = ? AND NOT deleted", req.BranchID).First(&branchRow).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.FetchBranchJob{}, fmt.Errorf("FetchBranch: branch %s not found", req.BranchID)
		}
		return models.FetchBranchJob{}, fmt.Errorf("FetchBranch %s: get branch: %w", req.BranchID, err)
	}
	if branchRow.Status == branchStatusFetching || branchRow.Status == branchStatusFetched {
		return models.FetchBranchJob{}, ErrBranchAlreadyFetched
	}

	now := models.NowRFC3339()

	// Short-circuit for locally-complete branches. If the branch already has
	// a HEAD commit, its commits/trees/blobs were populated by push-indexing
	// (or a prior successful fetch) and the objects live in the local clone
	// — re-cloning from source_url is unnecessary.
	if branchRow.HeadCommitID != nil && *branchRow.HeadCommitID != "" {
		jobRow := gormstore.FetchBranchJobToRow(models.FetchBranchJob{
			RepoID: req.RepoID, BranchName: branchRow.Name,
			Status: fetchJobStatusCompleted, CreatedAt: now, UpdatedAt: now,
		})
		if err := m.db.WithContext(ctx).Table(m.tables.FetchBranchJobs).Create(&jobRow).Error; err != nil {
			return models.FetchBranchJob{}, fmt.Errorf("FetchBranch %s: create job row: %w", req.BranchID, err)
		}
		if err := m.db.WithContext(ctx).Table(m.tables.Branches).Where("id = ?", req.BranchID).
			Updates(map[string]any{"status": branchStatusFetched, "updated_at": now}).Error; err != nil {
			return models.FetchBranchJob{}, fmt.Errorf("FetchBranch %s: mark fetched: %w", req.BranchID, err)
		}
		return gormstore.FetchBranchJobFromRow(jobRow), nil
	}

	jobRow := gormstore.FetchBranchJobToRow(models.FetchBranchJob{
		RepoID: req.RepoID, BranchName: branchRow.Name,
		Status: fetchJobStatusPending, CreatedAt: now, UpdatedAt: now,
	})
	if err := m.db.WithContext(ctx).Table(m.tables.FetchBranchJobs).Create(&jobRow).Error; err != nil {
		return models.FetchBranchJob{}, fmt.Errorf("FetchBranch %s: create job row: %w", req.BranchID, err)
	}
	jobID := jobRow.ID
	job := gormstore.FetchBranchJobFromRow(jobRow)

	if err := m.db.WithContext(ctx).Table(m.tables.Branches).Where("id = ?", req.BranchID).
		Updates(map[string]any{"status": branchStatusFetching, "updated_at": now}).Error; err != nil {
		return models.FetchBranchJob{}, fmt.Errorf("FetchBranch %s: transition to fetching: %w", req.BranchID, err)
	}

	jobCtx, cancel := context.WithCancel(context.Background())
	fetchJobsMu.Lock()
	fetchJobs[jobID] = fetchCancelEntry{cancel: cancel}
	fetchJobsMu.Unlock()

	go m.runFetchBranch(jobCtx, jobID, req.RepoID, req.BranchID, branchRow.Name)

	return job, nil
}

// GetFetchBranchStatus returns the current state of a fetch job.
// Returns [ErrImportJobNotFound] if no job with the given ID exists.
func (m *gitManager) GetFetchBranchStatus(ctx context.Context, jobID string) (models.FetchBranchJob, error) {
	var row gormstore.FetchBranchJobRow
	err := m.db.WithContext(ctx).Table(m.tables.FetchBranchJobs).Where("id = ?", jobID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.FetchBranchJob{}, ErrImportJobNotFound
		}
		return models.FetchBranchJob{}, fmt.Errorf("GetFetchBranchStatus %s: %w", jobID, err)
	}
	return gormstore.FetchBranchJobFromRow(row), nil
}

// runFetchBranch is the background goroutine started by [FetchBranch].
func (m *gitManager) runFetchBranch(ctx context.Context, jobID, repoID, branchID, branchName string) {
	runStart := time.Now()
	log.Printf("[fetchbranch] job=%s branch=%q repoID=%s: starting", jobID, branchName, repoID)
	defer func() {
		fetchJobsMu.Lock()
		delete(fetchJobs, jobID)
		fetchJobsMu.Unlock()
	}()

	fail := func(msg string) {
		log.Printf("[fetchbranch] job=%s branch=%q: FAILED — %s (elapsed %s)", jobID, branchName, msg, time.Since(runStart))
		bg := context.Background()
		_ = m.updateFetchJobStatus(bg, jobID, fetchJobStatusFailed, msg)
		_ = m.updateBranchFetchStatus(bg, branchID, branchStatusFetchFailed, msg)
	}

	if err := m.updateFetchJobStatus(ctx, jobID, fetchJobStatusRunning, ""); err != nil {
		return
	}

	var repoRow gormstore.RepositoryRow
	if err := m.db.WithContext(ctx).Table(m.tables.Repositories).Where("id = ?", repoID).First(&repoRow).Error; err != nil {
		fail(fmt.Sprintf("get repo row %s: %v", repoID, err))
		return
	}
	sourceURL := repoRow.SourceURL
	if sourceURL == "" {
		var branchRow gormstore.BranchRow
		if err := m.db.WithContext(ctx).Table(m.tables.Branches).Where("id = ?", branchID).First(&branchRow).Error; err == nil {
			sourceURL = branchRow.SourceURL
		}
	}

	// Perform a fresh full single-branch clone so we have the complete commit
	// history without shallow-object problems.
	repo, newCloneDir, err := m.deepenClone(ctx, branchName, sourceURL)
	if err != nil {
		fail(fmt.Sprintf("full clone branch=%q: %v", branchName, err))
		return
	}

	// Persist the new bare_clone_path back onto the Repository row so that
	// loadBlobContentFromBareClone always opens the correct (fully-hydrated) clone.
	if err := m.db.WithContext(ctx).Table(m.tables.Repositories).Where("id = ?", repoID).
		Updates(map[string]any{"bare_clone_path": newCloneDir, "updated_at": models.NowRFC3339()}).Error; err != nil {
		log.Printf("[fetchbranch] job=%s branch=%q: WARNING failed to update bare_clone_path to %s: %v", jobID, branchName, newCloneDir, err)
	}

	ref, err := findBranchRef(repo, branchName)
	if err != nil {
		fail(fmt.Sprintf("find ref for branch=%q: %v", branchName, err))
		return
	}

	seenSHAs := make(map[string]bool)
	if err := m.walkCommitsOnly(ctx, repo, ref, seenSHAs); err != nil {
		if ctx.Err() != nil {
			bg := context.Background()
			_ = m.updateFetchJobStatus(bg, jobID, fetchJobStatusFailed, "context cancelled")
			_ = m.updateBranchFetchStatus(bg, branchID, branchStatusFetchFailed, "context cancelled")
			return
		}
		fail(fmt.Sprintf("walk commits branch=%q: %v", branchName, err))
		return
	}

	tipCommit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		fail(fmt.Sprintf("resolve tip commit branch=%q sha=%q: %v", branchName, ref.Hash().String(), err))
		return
	}
	tipTree, err := tipCommit.Tree()
	if err != nil {
		fail(fmt.Sprintf("resolve tip tree branch=%q: %v", branchName, err))
		return
	}
	now := models.NowRFC3339()
	rootTreeID, err := m.upsertTreeMetadataWithEdges(ctx, repo, tipTree, "", now)
	if err != nil {
		fail(fmt.Sprintf("walk tip tree branch=%q: %v", branchName, err))
		return
	}

	tipSHA := ref.Hash().String()
	var headCommitRow gormstore.CommitRow
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).Where("sha = ?", tipSHA).First(&headCommitRow).Error; err == nil {
		if rootTreeID != "" {
			_ = m.db.WithContext(ctx).Table(m.tables.Commits).Where("id = ?", headCommitRow.ID).
				Update("tree_id", rootTreeID).Error
		}
		_, _ = m.advanceBranchHead(ctx, branchID, headCommitRow.ID, "")
	}

	bg := context.Background()
	if err := m.updateBranchFetchStatus(bg, branchID, branchStatusFetched, ""); err != nil {
		log.Printf("[fetchbranch] job=%s branch=%q: WARNING failed to mark branch fetched: %v", jobID, branchName, err)
	}
	if err := m.updateFetchJobStatus(bg, jobID, fetchJobStatusCompleted, ""); err != nil {
		log.Printf("[fetchbranch] job=%s branch=%q: WARNING failed to mark job completed: %v", jobID, branchName, err)
	}
	m.publish(bg, TopicBranchFetched, BranchFetchedPayload{JobID: jobID, BranchID: branchID})
	log.Printf("[fetchbranch] job=%s branch=%q: ALL DONE — total elapsed %s", jobID, branchName, time.Since(runStart))
}

// deepenClone performs a fresh full (non-shallow) single-branch clone of
// branchName from sourceURL into a new temp directory and returns the opened
// repository.
//
// A fresh clone is used instead of deepening the existing shallow clone because
// go-git v5 has no reliable Unshallow option: fetching into a shallow repo
// where the tip SHA already exists returns NoErrAlreadyUpToDate without
// fetching parent commits, leaving the object store incomplete.
func (m *gitManager) deepenClone(ctx context.Context, branchName, sourceURL string) (*gogit.Repository, string, error) {
	if sourceURL == "" {
		return nil, "", fmt.Errorf("deepenClone: source_url is empty for branch %q", branchName)
	}
	dir, err := cloneRootDir(branchName + "-full")
	if err != nil {
		return nil, "", fmt.Errorf("deepenClone: create temp dir: %w", err)
	}
	cloneRef := gogitplumbing.ReferenceName("refs/heads/" + branchName)
	repo, err := gogit.PlainCloneContext(ctx, dir, true, &gogit.CloneOptions{
		URL:           sourceURL,
		SingleBranch:  true,
		ReferenceName: cloneRef,
		Tags:          gogit.NoTags,
	})
	if err != nil {
		return nil, "", fmt.Errorf("deepenClone: clone branch %q: %w", branchName, err)
	}
	return repo, dir, nil
}

// walkCommitsOnly walks all commits reachable from ref and upserts Commit
// rows in one batch. seenSHAs deduplicates across multiple FetchBranch calls.
func (m *gitManager) walkCommitsOnly(ctx context.Context, repo *gogit.Repository, ref *gogitplumbing.Reference, seenSHAs map[string]bool) error {
	iter, err := repo.Log(&gogit.LogOptions{
		From:  ref.Hash(),
		Order: gogit.LogOrderCommitterTime,
	})
	if err != nil {
		return fmt.Errorf("log %s: %w", ref.Name().Short(), err)
	}
	defer iter.Close()

	now := models.NowRFC3339()
	var rows []gormstore.CommitRow
	if err := iter.ForEach(func(c *gogitobject.Commit) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		sha := c.Hash.String()
		if seenSHAs[sha] {
			return nil
		}
		seenSHAs[sha] = true
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
		return nil
	}); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	t0 := time.Now()
	err = m.db.WithContext(ctx).Table(m.tables.Commits).CreateInBatches(&rows, 200).Error
	if elapsed := time.Since(t0); elapsed > 500*time.Millisecond {
		log.Printf("[fetchbranch] SLOW bulk-insert %d commits took %s", len(rows), elapsed)
	}
	return err
}

// upsertTreeMetadataWithEdges creates a Tree row and its Blob children
// (metadata only, no content) and wires tree_blobs/tree_subtrees rows so that
// allBlobsAtCommit can traverse them. Returns the row ID of the created tree.
// Recursive for subdirectories.
func (m *gitManager) upsertTreeMetadataWithEdges(ctx context.Context, repo *gogit.Repository, tree *gogitobject.Tree, pathPrefix, now string) (string, error) {
	treeSHA := tree.Hash.String()
	treeRow := gormstore.TreeToRow(models.Tree{SHA: treeSHA, Path: pathPrefix, CreatedAt: now})
	if err := m.db.WithContext(ctx).Table(m.tables.Trees).Create(&treeRow).Error; err != nil {
		return "", fmt.Errorf("create tree %s path=%q: %w", treeSHA, pathPrefix, err)
	}
	treeID := treeRow.ID

	for _, entry := range tree.Entries {
		if ctx.Err() != nil {
			return treeID, ctx.Err()
		}
		var entryPath string
		if pathPrefix == "" {
			entryPath = entry.Name
		} else {
			entryPath = pathPrefix + "/" + entry.Name
		}
		if entry.Mode.IsFile() {
			blobID, err := m.upsertBlobMetadataWithID(ctx, repo, entry, entryPath, now)
			if err != nil {
				return treeID, err
			}
			if blobID != "" {
				if err := m.db.WithContext(ctx).Table(m.tables.TreeBlobs).
					Clauses(clause.OnConflict{DoNothing: true}).
					Create(&gormstore.TreeBlobRow{TreeID: treeID, BlobID: blobID}).Error; err != nil {
					log.Printf("[upsertTree] link tree_blobs path=%q: %v (non-fatal)", entryPath, err)
				}
			}
		} else {
			subTree, err := repo.TreeObject(entry.Hash)
			if err != nil {
				// Subtree has no raw data (import-job metadata-only entity) —
				// skip recursive walk; it will be deepened by a later fetch.
				log.Printf("[upsertTree] SKIP subtree path=%q sha=%s: TreeObject err=%v", entryPath, entry.Hash.String()[:8], err)
				continue
			}
			subTreeID, err := m.upsertTreeMetadataWithEdges(ctx, repo, subTree, entryPath, now)
			if err != nil {
				return treeID, err
			}
			if subTreeID != "" {
				if err := m.db.WithContext(ctx).Table(m.tables.TreeSubtrees).
					Clauses(clause.OnConflict{DoNothing: true}).
					Create(&gormstore.TreeSubtreeRow{TreeID: treeID, SubtreeID: subTreeID}).Error; err != nil {
					log.Printf("[upsertTree] link tree_subtrees path=%q: %v (non-fatal)", entryPath, err)
				}
			}
		}
	}
	return treeID, nil
}

// upsertBlobMetadataWithID creates a Blob row with sha, path, name,
// extension, and size — content is omitted for lazy population by ReadFile
// (GIT-023e). Returns the row ID of the created blob.
func (m *gitManager) upsertBlobMetadataWithID(ctx context.Context, repo *gogit.Repository, entry gogitobject.TreeEntry, fullPath, now string) (string, error) {
	blobSHA := entry.Hash.String()

	// Size: read from the storer only when the raw object bytes are available
	// (i.e. objects that were pushed, not import-job metadata-only entities).
	// For metadata-only blobs we use size 0 — it will be backfilled lazily.
	var blobSize int64
	if err := repo.Storer.HasEncodedObject(entry.Hash); err == nil {
		if blobObj, blobErr := repo.BlobObject(entry.Hash); blobErr == nil {
			blobSize = blobObj.Size
		}
	}

	ext := strings.TrimPrefix(filepath.Ext(entry.Name), ".")
	name := filepath.Base(fullPath)

	row := gormstore.BlobToRow(models.Blob{
		SHA:       blobSHA,
		Path:      fullPath,
		Name:      name,
		Extension: ext,
		Size:      blobSize,
		CreatedAt: now,
	})
	if err := m.db.WithContext(ctx).Table(m.tables.Blobs).Create(&row).Error; err != nil {
		return "", fmt.Errorf("create blob metadata %s path=%q: %w", blobSHA, fullPath, err)
	}
	return row.ID, nil
}

// updateFetchJobStatus transitions a FetchBranchJob row to the given status.
func (m *gitManager) updateFetchJobStatus(ctx context.Context, jobID, status, errMsg string) error {
	return m.db.WithContext(ctx).Table(m.tables.FetchBranchJobs).Where("id = ?", jobID).Updates(map[string]any{
		"status":        status,
		"error_message": errMsg,
		"updated_at":    models.NowRFC3339(),
	}).Error
}

// updateBranchFetchStatus patches the Branch row status and optional error_message.
func (m *gitManager) updateBranchFetchStatus(ctx context.Context, branchID, status, errMsg string) error {
	updates := map[string]any{
		"status":     status,
		"updated_at": models.NowRFC3339(),
	}
	if errMsg != "" {
		updates["error_message"] = errMsg
	}
	return m.db.WithContext(ctx).Table(m.tables.Branches).Where("id = ?", branchID).Updates(updates).Error
}

// findBranchRef resolves a branch name to a reference in the local clone.
// Checks refs/heads/<name> first, then refs/remotes/origin/<name>.
func findBranchRef(repo *gogit.Repository, branchName string) (*gogitplumbing.Reference, error) {
	candidates := []gogitplumbing.ReferenceName{
		gogitplumbing.ReferenceName("refs/heads/" + branchName),
		gogitplumbing.ReferenceName("refs/remotes/origin/" + branchName),
	}
	for _, name := range candidates {
		ref, err := repo.Reference(name, true)
		if err == nil {
			return ref, nil
		}
	}
	return nil, fmt.Errorf("branch %q not found in local clone", branchName)
}
