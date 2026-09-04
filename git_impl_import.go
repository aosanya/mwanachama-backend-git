// git_impl_import.go implements [GitManager.ImportRepo], [GitManager.GetImportStatus],
// and [GitManager.CancelImport].
//
// GIT-023c — Lazy Import v2 (Phase 1):
//
// ImportRepo begins an async background goroutine that:
//  1. Creates an ImportJob row (status=pending) and returns immediately.
//  2. Performs a bare shallow clone (Depth=1, Bare=true, NoTags) into a
//     persistent directory under the deployment's clone root.  The directory
//     is NOT cleaned up — FetchBranch reuses it for on-demand deepening.
//  3. Iterates remote refs to discover branches.
//  4. Writes one Repository row (carrying bare_clone_path) and one stub
//     Branch row per ref (status="stub"; no commits, trees, or blobs).
//  5. Transitions the job to completed.
//
// walkBranchCommits is retained for use by FetchBranch (GIT-023d).  It now
// accepts a seenSHAs set so shared commit history across branches is processed
// only once.
//
// A per-job cancel function is stored in an in-process map so that
// CancelImport can interrupt a running goroutine.
package mwanachamagit

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	gogit "github.com/go-git/go-git/v5"
	gogitplumbing "github.com/go-git/go-git/v5/plumbing"

	"github.com/aosanya/mwanachama-backend-git/gormstore"
	"github.com/aosanya/mwanachama-backend-git/models"
)

// importJobStatus values used as the "status" column of the ImportJob row.
const (
	importStatusPending   = "pending"
	importStatusRunning   = "running"
	importStatusCompleted = "completed"
	importStatusFailed    = "failed"
	importStatusCancelled = "cancelled"
)

// branchStatus values for the Branch row "status" column (lazy import v2).
const (
	branchStatusStub        = "stub"
	branchStatusFetching    = "fetching"
	branchStatusFetched     = "fetched"
	branchStatusFetchFailed = "fetch_failed"
)

// cloneRootDir returns the persistent directory that holds the bare clone for
// this import job.  If the directory already exists (e.g., from a previous
// failed run) it is removed and recreated so that PlainClone always starts
// with an empty target.
func cloneRootDir(jobID string) (string, error) {
	base := filepath.Join(os.TempDir(), "mwanachama-backend-git-clones", jobID)
	if err := os.RemoveAll(base); err != nil {
		return "", fmt.Errorf("cloneRootDir remove stale %s: %w", base, err)
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", fmt.Errorf("cloneRootDir %s: %w", base, err)
	}
	return base, nil
}

// importCancelEntry holds the cancel function and in-memory progress log for an
// in-flight import goroutine. A pointer is stored in importJobs so that step
// messages can be appended without replacing the map entry.
type importCancelEntry struct {
	cancel context.CancelFunc
	mu     sync.Mutex
	steps  []string
}

// appendStep adds a progress message to the entry.
func (e *importCancelEntry) appendStep(msg string) {
	e.mu.Lock()
	e.steps = append(e.steps, msg)
	e.mu.Unlock()
}

// getSteps returns a copy of the current progress steps.
func (e *importCancelEntry) getSteps() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, len(e.steps))
	copy(out, e.steps)
	return out
}

// importJobsMu guards importJobs.
var importJobsMu sync.Mutex

// importJobs maps jobID → cancel entry for all active (pending/running) import goroutines.
// Goroutines remove their entry on completion, failure, or cancellation.
var importJobs = make(map[string]*importCancelEntry)

// appendImportStep appends a progress message for the given job (no-op if terminal).
func appendImportStep(jobID, msg string) {
	importJobsMu.Lock()
	entry, ok := importJobs[jobID]
	importJobsMu.Unlock()
	if ok {
		entry.appendStep(msg)
	}
}

// ImportRepo begins an async import of a public Git repository.
// It returns immediately with an ImportJob whose ID can be used to poll
// [GitManager.GetImportStatus].
//
// Returns [ErrRepoAlreadyExists] if a Repository row already exists.
// Returns [ErrImportInProgress] if a pending or running import already exists.
func (m *gitManager) ImportRepo(ctx context.Context, req ImportRepoRequest) (models.ImportJob, error) {
	var repoCount int64
	if err := m.db.WithContext(ctx).Table(m.tables.Repositories).
		Where("NOT deleted").Count(&repoCount).Error; err != nil {
		return models.ImportJob{}, fmt.Errorf("ImportRepo: check existing repos: %w", err)
	}
	if repoCount > 0 {
		return models.ImportJob{}, ErrRepoAlreadyExists
	}

	var activeCount int64
	if err := m.db.WithContext(ctx).Table(m.tables.ImportJobs).
		Where("status IN ? AND NOT deleted", []string{importStatusPending, importStatusRunning}).
		Count(&activeCount).Error; err != nil {
		return models.ImportJob{}, fmt.Errorf("ImportRepo: check active jobs: %w", err)
	}
	if activeCount > 0 {
		return models.ImportJob{}, ErrImportInProgress
	}

	now := models.NowRFC3339()
	if req.DefaultBranch == "" {
		req.DefaultBranch = "main"
	}
	jobRow := gormstore.ImportJobToRow(models.ImportJob{
		Name:          req.Name,
		SourceURL:     req.SourceURL,
		DefaultBranch: req.DefaultBranch,
		Status:        importStatusPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err := m.db.WithContext(ctx).Table(m.tables.ImportJobs).Create(&jobRow).Error; err != nil {
		return models.ImportJob{}, fmt.Errorf("ImportRepo: create job row: %w", err)
	}
	jobID := jobRow.ID
	job := gormstore.ImportJobFromRow(jobRow)

	jobCtx, cancel := context.WithCancel(context.Background())
	entry := &importCancelEntry{cancel: cancel}
	importJobsMu.Lock()
	importJobs[jobID] = entry
	importJobsMu.Unlock()

	go m.runImport(jobCtx, jobID, req, req.DefaultBranch)

	return job, nil
}

// GetImportStatus returns the current state of an import job.
// Returns [ErrImportJobNotFound] if no job with the given ID exists.
func (m *gitManager) GetImportStatus(ctx context.Context, jobID string) (models.ImportJob, error) {
	var row gormstore.ImportJobRow
	err := m.db.WithContext(ctx).Table(m.tables.ImportJobs).Where("id = ?", jobID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.ImportJob{}, ErrImportJobNotFound
		}
		return models.ImportJob{}, fmt.Errorf("GetImportStatus %s: %w", jobID, err)
	}
	job := gormstore.ImportJobFromRow(row)
	importJobsMu.Lock()
	entry, ok := importJobs[jobID]
	importJobsMu.Unlock()
	if ok {
		job.ProgressSteps = entry.getSteps()
	}
	return job, nil
}

// CancelImport cancels a pending or running import job.
// Returns [ErrImportJobNotFound] if the job does not exist.
// Returns [ErrImportJobNotCancellable] if the job is already in a terminal state.
func (m *gitManager) CancelImport(ctx context.Context, jobID string) error {
	job, err := m.GetImportStatus(ctx, jobID)
	if err != nil {
		return err
	}

	switch job.Status {
	case importStatusCompleted, importStatusFailed, importStatusCancelled:
		return ErrImportJobNotCancellable
	}

	importJobsMu.Lock()
	entry, ok := importJobs[jobID]
	importJobsMu.Unlock()
	if ok {
		entry.cancel()
	}

	return m.updateImportJobStatus(context.Background(), jobID, importStatusCancelled, "")
}

// runImport is the background goroutine started by [ImportRepo].
//
// GIT-023c — Phase 1 (lazy import v2):
//
//  1. Bare shallow clone (Depth=1, Bare=true, NoTags) into a persistent directory.
//  2. Iterate remote refs to discover branch names and tip SHAs.
//  3. Create one Repository row (with bare_clone_path) and one stub Branch
//     row per discovered branch ref (status="stub").
//  4. Automatically trigger [GitManager.FetchBranch] for the default branch
//     so it is fully populated (commits, trees, blobs) without user interaction.
//  5. Transition job to completed.
func (m *gitManager) runImport(ctx context.Context, jobID string, req ImportRepoRequest, defaultBranch string) {
	defer func() {
		importJobsMu.Lock()
		delete(importJobs, jobID)
		importJobsMu.Unlock()
	}()

	runStart := time.Now()
	appendImportStep(jobID, "Starting import…")

	if err := m.updateImportJobStatus(ctx, jobID, importStatusRunning, ""); err != nil {
		return
	}

	cloneDir, err := cloneRootDir(jobID)
	if err != nil {
		m.failImportJob(ctx, jobID, fmt.Sprintf("allocate clone dir: %v", err))
		return
	}

	appendImportStep(jobID, fmt.Sprintf("Cloning %s (shallow, all branches)…", req.SourceURL))
	t0 := time.Now()
	log.Printf("[import] job=%s: starting bare shallow clone of %s", jobID, req.SourceURL)
	cloneOpts := &gogit.CloneOptions{
		URL:          req.SourceURL,
		Depth:        1,
		SingleBranch: false,
		Tags:         gogit.NoTags,
	}
	repo, err := gogit.PlainCloneContext(ctx, cloneDir, true /* isBare */, cloneOpts)
	if err != nil {
		if ctx.Err() != nil {
			_ = m.updateImportJobStatus(context.Background(), jobID, importStatusCancelled, "")
			m.publish(context.Background(), TopicRepoImportCancelled, RepoImportCancelledPayload{JobID: jobID})
			return
		}
		m.failImportJob(ctx, jobID, fmt.Sprintf("bare shallow clone %s: %v", req.SourceURL, err))
		return
	}
	log.Printf("[import] job=%s: bare shallow clone done in %s", jobID, time.Since(t0))
	appendImportStep(jobID, "Clone complete. Discovering branches…")

	now := models.NowRFC3339()
	repoRow := gormstore.RepositoryToRow(models.Repository{
		Name:          req.Name,
		Description:   req.Description,
		DefaultBranch: defaultBranch,
		SourceURL:     req.SourceURL,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	repoRow.BareClonePath = cloneDir
	if err := m.db.WithContext(ctx).Table(m.tables.Repositories).Create(&repoRow).Error; err != nil {
		m.failImportJob(ctx, jobID, fmt.Sprintf("create Repository row: %v", err))
		return
	}
	repoID := repoRow.ID
	appendImportStep(jobID, fmt.Sprintf("Repository row created (id=%s).", repoID))

	refs, err := repo.References()
	if err != nil {
		m.failImportJob(ctx, jobID, fmt.Sprintf("list refs: %v", err))
		return
	}

	var branchCount int
	seenBranches := make(map[string]bool)
	if err := refs.ForEach(func(ref *gogitplumbing.Reference) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Resolve a human-readable branch name from this ref.
		//
		// A bare go-git clone stores remote branches as refs/remotes/origin/<name>
		// (IsBranch() == false, IsRemote() == true) rather than under refs/heads/.
		// We accept both forms and normalise to a plain branch name in both cases.
		var branchName string
		switch {
		case ref.Name().IsBranch():
			branchName = ref.Name().Short()
		case ref.Name().IsRemote():
			short := ref.Name().Short() // e.g. "origin/main"
			const prefix = "origin/"
			if !strings.HasPrefix(short, prefix) {
				return nil
			}
			branchName = strings.TrimPrefix(short, prefix)
			if branchName == "HEAD" || branchName == "" {
				return nil
			}
		default:
			return nil
		}

		if seenBranches[branchName] {
			return nil
		}
		seenBranches[branchName] = true

		branchCount++
		appendImportStep(jobID, fmt.Sprintf("Creating stub branch: %s", branchName))
		return m.upsertStubBranchNamed(ctx, branchName, ref.Hash().String(), repoID, req.SourceURL, now)
	}); err != nil {
		if ctx.Err() != nil {
			_ = m.updateImportJobStatus(context.Background(), jobID, importStatusCancelled, "")
			m.publish(context.Background(), TopicRepoImportCancelled, RepoImportCancelledPayload{JobID: jobID})
			return
		}
		m.failImportJob(ctx, jobID, fmt.Sprintf("walk refs: %v", err))
		return
	}
	appendImportStep(jobID, fmt.Sprintf("%d branch stub(s) discovered.", branchCount))

	// Automatically fetch the default branch so it is immediately usable.
	appendImportStep(jobID, fmt.Sprintf("Auto-fetching default branch %q…", defaultBranch))
	var defaultBranchRow gormstore.BranchRow
	err = m.db.WithContext(ctx).Table(m.tables.Branches).
		Where("name = ? AND NOT deleted", defaultBranch).First(&defaultBranchRow).Error
	if err == nil {
		_, fetchErr := m.FetchBranch(ctx, FetchBranchRequest{RepoID: repoID, BranchID: defaultBranchRow.ID})
		if fetchErr != nil {
			appendImportStep(jobID, fmt.Sprintf("Auto-fetch for %q skipped: %v", defaultBranch, fetchErr))
		} else {
			appendImportStep(jobID, fmt.Sprintf("Default branch %q fetch started in background.", defaultBranch))
		}
	} else {
		appendImportStep(jobID, fmt.Sprintf("Default branch %q not found in stubs; skipping auto-fetch.", defaultBranch))
	}

	m.publish(ctx, TopicRepoImported, RepoImportedPayload{JobID: jobID})
	if err := m.updateImportJobStatus(context.Background(), jobID, importStatusCompleted, ""); err != nil {
		log.Printf("[import] job=%s: WARNING failed to mark import completed: %v", jobID, err)
	}
	log.Printf("[import] job=%s: import phase done (stub+auto-fetch triggered) — total elapsed %s", jobID, time.Since(runStart))
}

// upsertStubBranchNamed creates (or updates) a Branch row with status="stub"
// for the given branch name and tip SHA.
func (m *gitManager) upsertStubBranchNamed(ctx context.Context, branchName, tipSHA, repoID, sourceURL, now string) error {
	var existing gormstore.BranchRow
	err := m.db.WithContext(ctx).Table(m.tables.Branches).
		Where("name = ? AND NOT deleted", branchName).First(&existing).Error
	switch {
	case err == nil:
		updates := map[string]any{
			"sha":        tipSHA,
			"status":     branchStatusStub,
			"source_url": sourceURL,
			"updated_at": now,
		}
		if err := m.db.WithContext(ctx).Table(m.tables.Branches).
			Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("stub branch %s: update: %w", branchName, err)
		}
	case errors.Is(err, gorm.ErrRecordNotFound):
		row := gormstore.BranchToRow(models.Branch{
			Name:      branchName,
			SHA:       tipSHA,
			CreatedAt: now,
			UpdatedAt: now,
		})
		row.RepositoryID = gormstore.StringToNullable(repoID)
		row.Status = branchStatusStub
		row.SourceURL = sourceURL
		if err := m.db.WithContext(ctx).Table(m.tables.Branches).Create(&row).Error; err != nil {
			return fmt.Errorf("stub branch %s: create: %w", branchName, err)
		}
	default:
		return fmt.Errorf("stub branch %s: list: %w", branchName, err)
	}
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

// updateImportJobStatus transitions an ImportJob row to the given status.
func (m *gitManager) updateImportJobStatus(ctx context.Context, jobID, status, errMsg string) error {
	return m.db.WithContext(ctx).Table(m.tables.ImportJobs).Where("id = ?", jobID).Updates(map[string]any{
		"status":        status,
		"error_message": errMsg,
		"updated_at":    models.NowRFC3339(),
	}).Error
}

// failImportJob transitions the job to failed, logs, and publishes the failure event.
func (m *gitManager) failImportJob(ctx context.Context, jobID, errMsg string) {
	log.Printf("[import] job=%s: FAILED — %s", jobID, errMsg)
	_ = m.updateImportJobStatus(context.Background(), jobID, importStatusFailed, errMsg)
	m.publish(ctx, TopicRepoImportFailed, RepoImportFailedPayload{JobID: jobID, ErrorMessage: errMsg})
}
