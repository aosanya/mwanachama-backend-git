// File operations and commit history implementations for [gitManager].
//
// WriteFile creates Commit, Tree, and Blob rows, wires them, and advances
// the branch HEAD pointer. ReadFile, DeleteFile, and ListDirectory traverse
// the commit + tree graph (via allBlobsAtCommit, git_impl_converters.go) to
// locate blobs. Log walks the commit-parent chain; Diff compares two commit
// trees.
package mwanachamagit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm/clause"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/aosanya/mwanachama-backend-git/gormstore"
	"github.com/aosanya/mwanachama-backend-git/models"
)

// ── File Operations ───────────────────────────────────────────────────────────

// treeRecord holds the git object data for one tree node built by buildNestedTrees.
type treeRecord struct {
	path    string // directory path: "" = root, "lib", "lib/providers", …
	sha     string
	rawData []byte
	size    int64
	entries string // JSON [{name,mode,sha}] for storage
}

// buildNestedTrees constructs a complete, properly-nested git tree from a flat
// map of full file paths to blob hashes. It processes directories bottom-up so
// subtree hashes are known before parent trees are encoded.
// Returns the root tree hash and one treeRecord per directory.
func buildNestedTrees(files map[string]plumbing.Hash) (plumbing.Hash, []treeRecord, error) {
	// Collect every directory that appears as an ancestor of any file path.
	dirSet := map[string]bool{"": true}
	for p := range files {
		parts := strings.Split(p, "/")
		for i := 1; i < len(parts); i++ {
			dirSet[strings.Join(parts[:i], "/")] = true
		}
	}

	// Sort directories deepest-first so subtrees are built before their parents.
	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		dirs = append(dirs, d)
	}
	sort.Slice(dirs, func(i, j int) bool {
		di := strings.Count(dirs[i], "/")
		dj := strings.Count(dirs[j], "/")
		if di != dj {
			return di > dj
		}
		return dirs[i] > dirs[j]
	})

	subtreeHashes := map[string]plumbing.Hash{} // dir path → encoded tree hash
	var records []treeRecord

	for _, dir := range dirs {
		var entries []object.TreeEntry

		// File entries directly inside this directory.
		for filePath, blobHash := range files {
			if dirPath(filePath) == dir {
				entries = append(entries, object.TreeEntry{
					Name: fileName(filePath),
					Mode: filemode.Regular,
					Hash: blobHash,
				})
			}
		}

		// Immediate subdirectory entries (already encoded in earlier iterations).
		for subPath, subHash := range subtreeHashes {
			if dirPath(subPath) == dir {
				entries = append(entries, object.TreeEntry{
					Name: fileName(subPath),
					Mode: filemode.Dir,
					Hash: subHash,
				})
			}
		}

		// Git requires tree entries sorted by name.
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Name < entries[j].Name
		})

		treeObj := &object.Tree{Entries: entries}
		memObj := &plumbing.MemoryObject{}
		if err := treeObj.Encode(memObj); err != nil {
			return plumbing.ZeroHash, nil, fmt.Errorf("buildNestedTrees: encode %q: %w", dir, err)
		}
		r, _ := memObj.Reader()
		raw, _ := io.ReadAll(r)
		r.Close()

		h := memObj.Hash()
		subtreeHashes[dir] = h

		entryMaps := make([]map[string]string, len(entries))
		for i, e := range entries {
			mode := "100644"
			if e.Mode == filemode.Dir {
				mode = "040000"
			}
			entryMaps[i] = map[string]string{"name": e.Name, "mode": mode, "sha": e.Hash.String()}
		}
		entriesJSON, _ := json.Marshal(entryMaps)

		records = append(records, treeRecord{
			path:    dir,
			sha:     h.String(),
			rawData: raw,
			size:    memObj.Size(),
			entries: string(entriesJSON),
		})
	}

	rootHash, ok := subtreeHashes[""]
	if !ok {
		return plumbing.ZeroHash, nil, fmt.Errorf("buildNestedTrees: root tree missing")
	}
	return rootHash, records, nil
}

// WriteFile commits a single file to the specified branch.
// Each call builds a complete nested git tree that includes all files from
// the parent commit plus the new file, so the branch accumulates files
// correctly across successive writes.
//
// The entire operation runs inside the [RefLocker] lock so that
// concurrent writes chain commits onto each other instead of racing the
// branch ref. Without this, each caller reads the same parent HEAD, builds a
// sibling commit, and the unsynchronised [gitManager.advanceBranchHead] call
// leaves the branch pointing at only the last writer's commit.
//
// Returns [ErrBranchNotFound] if the branch does not exist.
// Returns [ErrRepoNotInitialised] if no repository entity exists.
func (m *gitManager) WriteFile(ctx context.Context, req WriteFileRequest) (models.Commit, error) {
	var result models.Commit
	lockErr := m.locker.WithMergeLock(ctx, func() error {
		c, err := m.writeFileLocked(ctx, req)
		if err != nil {
			return err
		}
		result = c
		return nil
	})
	if lockErr != nil {
		return models.Commit{}, lockErr
	}
	return result, nil
}

// writeFileLocked is the body of WriteFile that must run under the
// [RefLocker]. Do not call directly outside of WriteFile.
func (m *gitManager) writeFileLocked(ctx context.Context, req WriteFileRequest) (models.Commit, error) {
	branch, err := m.GetBranch(ctx, req.BranchID)
	if err != nil {
		return models.Commit{}, fmt.Errorf("WriteFile: %w", err)
	}
	repo, err := m.GetRepository(ctx, branch.RepositoryID)
	if err != nil {
		return models.Commit{}, fmt.Errorf("WriteFile: %w", err)
	}

	// Normalise the path: strip leading slashes.
	req.Path = strings.TrimLeft(req.Path, "/")

	encoding := req.Encoding
	if encoding == "" {
		encoding = "utf-8"
	}
	message := req.Message
	if message == "" {
		message = "Update " + req.Path
	}
	commitTime := time.Now().UTC()
	now := commitTime.Format(time.RFC3339)

	// ── 1. Create the new Blob row ────────────────────────────────────────────
	blobObj := &plumbing.MemoryObject{}
	blobObj.SetType(plumbing.BlobObject)
	blobW, _ := blobObj.Writer()
	_, _ = blobW.Write([]byte(req.Content))
	_ = blobW.Close()
	blobR, _ := blobObj.Reader()
	blobRaw, _ := io.ReadAll(blobR)
	_ = blobR.Close()
	blobDataB64 := base64.StdEncoding.EncodeToString(blobRaw)
	blobHash := blobObj.Hash()
	blobSHA := blobHash.String()

	blobRow := gormstore.BlobToRow(models.Blob{
		SHA:       blobSHA,
		Path:      req.Path,
		Name:      fileName(req.Path),
		Extension: fileExtension(req.Path),
		Size:      int64(len(req.Content)),
		Encoding:  encoding,
		Content:   req.Content,
		CreatedAt: now,
	})
	blobRow.Data = blobDataB64
	if err := m.db.WithContext(ctx).Table(m.tables.Blobs).Create(&blobRow).Error; err != nil {
		return models.Commit{}, fmt.Errorf("WriteFile: create blob: %w", err)
	}

	// ── 2. Build the complete file map (parent files + new file) ──────────────
	var parentIDs []string
	var parentHashes []plumbing.Hash
	if branch.HeadCommitID != "" {
		parentIDs = []string{branch.HeadCommitID}
		var parentRow gormstore.CommitRow
		if err := m.db.WithContext(ctx).Table(m.tables.Commits).
			Where("id = ?", branch.HeadCommitID).First(&parentRow).Error; err != nil {
			return models.Commit{}, fmt.Errorf("WriteFile: get parent commit: %w", err)
		}
		if parentRow.SHA != "" {
			parentHashes = []plumbing.Hash{plumbing.NewHash(parentRow.SHA)}
		}
	}

	// path → blob git SHA for all files on the branch after this write.
	fileMap := map[string]plumbing.Hash{}
	// path → blob row ID for wiring tree_blobs rows to the new root tree.
	blobEntityByPath := map[string]string{}

	if len(parentIDs) > 0 {
		parentBlobs, err := m.allBlobsAtCommit(ctx, parentIDs[0])
		if err != nil {
			log.Printf("WriteFile: allBlobsAtCommit parent=%s: %v (continuing with empty parent)", parentIDs[0], err)
		}
		for _, b := range parentBlobs {
			if b.Path != "" && b.SHA != "" {
				fileMap[b.Path] = plumbing.NewHash(b.SHA)
				blobEntityByPath[b.Path] = b.ID
			}
		}
	}
	// Add/replace with the file being written.
	fileMap[req.Path] = blobHash
	blobEntityByPath[req.Path] = blobRow.ID

	// ── 3. Build nested git trees ─────────────────────────────────────────────
	rootTreeHash, treeRecords, err := buildNestedTrees(fileMap)
	if err != nil {
		return models.Commit{}, fmt.Errorf("WriteFile: %w", err)
	}

	// ── 4. Persist all tree rows (root + subtrees) ────────────────────────────
	// Only the root tree is ever linked to a blob or a commit below — matches
	// the pre-GORM behavior, where every blob was wired flat onto the root
	// tree (see step 6) and non-root tree rows were created but never linked
	// by any edge. Not fixed here; a behavior-preserving port.
	var rootTreeID string
	for _, tr := range treeRecords {
		treeRow := gormstore.TreeToRow(models.Tree{SHA: tr.sha, Path: tr.path, CreatedAt: now})
		treeRow.Entries = tr.entries
		treeRow.Data = base64.StdEncoding.EncodeToString(tr.rawData)
		treeRow.Size = tr.size
		if err := m.db.WithContext(ctx).Table(m.tables.Trees).Create(&treeRow).Error; err != nil {
			return models.Commit{}, fmt.Errorf("WriteFile: create tree row path=%q: %w", tr.path, err)
		}
		if tr.sha == rootTreeHash.String() {
			rootTreeID = treeRow.ID
		}
	}

	// ── 5. Encode and persist the Commit row ──────────────────────────────────
	// Fall back to a bot identity when the caller didn't supply an author —
	// go-git renders empty signatures as "Author: <>" which leaves commits
	// unattributable. Any downstream tool that filters / blames by author
	// then has nothing to bind to.
	authorName := req.AuthorName
	if authorName == "" {
		authorName = "mwanachama-bot"
	}
	authorEmail := req.AuthorEmail
	if authorEmail == "" {
		authorEmail = "bot@mwanachama.local"
	}
	gitCommitObj := &object.Commit{
		TreeHash:     rootTreeHash,
		ParentHashes: parentHashes,
		Author:       object.Signature{Name: authorName, Email: authorEmail, When: commitTime},
		Committer:    object.Signature{Name: authorName, Email: authorEmail, When: commitTime},
		Message:      message,
	}
	commitMemObj := &plumbing.MemoryObject{}
	if err := gitCommitObj.Encode(commitMemObj); err != nil {
		return models.Commit{}, fmt.Errorf("WriteFile: encode commit: %w", err)
	}
	commitR, _ := commitMemObj.Reader()
	commitRaw, _ := io.ReadAll(commitR)
	_ = commitR.Close()
	commitDataB64 := base64.StdEncoding.EncodeToString(commitRaw)
	commitSHA := commitMemObj.Hash().String()

	commitRow := gormstore.CommitToRow(models.Commit{
		SHA:            commitSHA,
		Message:        message,
		AuthorName:     req.AuthorName,
		AuthorEmail:    req.AuthorEmail,
		AuthorAt:       now,
		CommitterName:  req.AuthorName,
		CommitterEmail: req.AuthorEmail,
		CommittedAt:    now,
		TreeID:         rootTreeID,
		CreatedAt:      now,
	})
	commitRow.RepositoryID = gormstore.StringToNullable(repo.ID)
	commitRow.Data = commitDataB64
	commitRow.Size = commitMemObj.Size()
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).Create(&commitRow).Error; err != nil {
		return models.Commit{}, fmt.Errorf("WriteFile: create commit: %w", err)
	}

	// ── 6. Wire edges ─────────────────────────────────────────────────────────
	if len(parentIDs) > 0 {
		parentRows := gormstore.CommitParentsToRows(commitRow.ID, parentIDs)
		if err := m.db.WithContext(ctx).Table(m.tables.CommitParents).Create(&parentRows).Error; err != nil {
			return models.Commit{}, fmt.Errorf("WriteFile: link parents: %w", err)
		}
	}
	// Wire root tree → every blob so allBlobsAtCommit finds them all (flat,
	// matching the pre-GORM has_blob wiring — see step 4's note).
	treeBlobRows := make([]gormstore.TreeBlobRow, 0, len(blobEntityByPath))
	for _, blobID := range blobEntityByPath {
		treeBlobRows = append(treeBlobRows, gormstore.TreeBlobRow{TreeID: rootTreeID, BlobID: blobID})
	}
	if len(treeBlobRows) > 0 {
		if err := m.db.WithContext(ctx).Table(m.tables.TreeBlobs).
			Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&treeBlobRows, 200).Error; err != nil {
			log.Printf("WriteFile: link tree_blobs: %v (non-fatal)", err)
		}
	}

	// ── 7. Advance branch HEAD ────────────────────────────────────────────────
	if _, err := m.advanceBranchHead(ctx, branch.ID, commitRow.ID, ""); err != nil {
		return models.Commit{}, fmt.Errorf("WriteFile: advance branch head: %w", err)
	}

	commit := gormstore.CommitFromRow(commitRow)
	commit.RepositoryID = repo.ID
	commit.ParentIDs = parentIDs
	m.publish(ctx, TopicFileWritten, FileWrittenPayload{
		Repository:    repo.Name,
		BranchName:    branch.Name,
		Path:          req.Path,
		CommitSHA:     commitSHA,
		WorkflowRunID: branch.WorkflowRunID,
	})
	return commit, nil
}

// ReadFile returns the content of path at the branch's HEAD commit.
// It first checks whether the blob row already carries cached content.
// If the content field is empty (stub blob created by FetchBranch), it reads
// the content directly from the local full clone and caches it back into the
// row.
// Returns [ErrBranchNotFound] if the branch does not exist.
// Returns [ErrFileNotFound] if the path is not present on the branch.
// Returns [ErrBlobContentUnavailable] if the blob exists as a stub but the
// local clone is unavailable; the caller should trigger [GitManager.FetchBranch]
// and retry.
func (m *gitManager) ReadFile(ctx context.Context, branchID, path string) (models.Blob, error) {
	branch, err := m.GetBranch(ctx, branchID)
	if err != nil {
		return models.Blob{}, fmt.Errorf("ReadFile: %w", err)
	}
	if branch.HeadCommitID == "" {
		return models.Blob{}, ErrFileNotFound
	}
	blob, err := m.findBlobAtCommit(ctx, branch.HeadCommitID, path)
	if err != nil {
		return models.Blob{}, fmt.Errorf("ReadFile: %w", err)
	}

	// Fast path: content is already cached.
	if blob.Content != "" {
		return blob, nil
	}

	// Lazy path: blob was written as metadata-only (no content field).
	// Hydrate from the local full clone FetchBranch made.
	content, encoding, loadErr := m.loadBlobContentFromBareClone(ctx, branch, blob)
	if loadErr != nil {
		return models.Blob{}, ErrBlobContentUnavailable
	}

	blob.Content = content
	blob.Encoding = encoding
	return blob, nil
}

// DeleteFile removes a file from the specified branch by creating a deletion
// commit (empty content, size=0).
// Returns [ErrBranchNotFound] if the branch does not exist.
// Returns [ErrFileNotFound] if the path does not exist on the branch.
func (m *gitManager) DeleteFile(ctx context.Context, req DeleteFileRequest) (models.Commit, error) {
	// Verify the file exists first and capture the current blob ID for edge cleanup.
	existingBlob, err := m.ReadFile(ctx, req.BranchID, req.Path)
	if err != nil {
		return models.Commit{}, err
	}

	// GIT-022c: Remove branch-scoped documentation edges on the deleted blob
	// before the deletion commit is written.
	m.deleteDocEdgesForBlob(ctx, existingBlob.ID, req.BranchID)

	message := req.Message
	if message == "" {
		message = "Delete " + req.Path
	}
	// A deletion commit writes empty content to the path.
	return m.WriteFile(ctx, WriteFileRequest{
		BranchID:    req.BranchID,
		Path:        req.Path,
		Content:     "",
		Encoding:    "utf-8",
		AuthorName:  req.AuthorName,
		AuthorEmail: req.AuthorEmail,
		Message:     message,
	})
}

// ListDirectory returns the immediate children (files and sub-directories)
// at the given path on the branch's HEAD commit.
// Returns [ErrBranchNotFound] if the branch does not exist.
// Returns [ErrFileNotFound] if the path does not exist on the branch.
func (m *gitManager) ListDirectory(ctx context.Context, branchID, path string) ([]FileEntry, error) {
	branch, err := m.GetBranch(ctx, branchID)
	if err != nil {
		return nil, fmt.Errorf("ListDirectory: %w", err)
	}
	if branch.HeadCommitID == "" {
		return []FileEntry{}, nil
	}

	// Find all blobs reachable from the HEAD commit.
	blobs, err := m.allBlobsAtCommit(ctx, branch.HeadCommitID)
	if err != nil {
		return nil, fmt.Errorf("ListDirectory: %w", err)
	}

	// Normalise the query path: trim leading/trailing slashes.
	queryDir := strings.Trim(path, "/")

	seen := make(map[string]bool)
	var entries []FileEntry
	for _, b := range blobs {
		bPath := b.Path
		bDir := dirPath(bPath)
		bDirClean := strings.Trim(bDir, "/")

		if queryDir == "" {
			// Root listing — show immediate children only.
			rel := bPath
			// If the file is in a subdirectory, show the directory entry.
			parts := strings.SplitN(strings.Trim(rel, "/"), "/", 2)
			if len(parts) == 1 {
				// Direct file at root.
				if !seen[parts[0]] {
					seen[parts[0]] = true
					entries = append(entries, FileEntry{
						Name:  parts[0],
						Path:  parts[0],
						IsDir: false,
						Size:  b.Size,
					})
				}
			} else {
				// Subdirectory entry.
				if !seen[parts[0]] {
					seen[parts[0]] = true
					entries = append(entries, FileEntry{
						Name:  parts[0],
						Path:  parts[0],
						IsDir: true,
					})
				}
			}
		} else {
			// Subdirectory listing.
			if bDirClean == queryDir {
				name := fileName(bPath)
				if !seen[name] {
					seen[name] = true
					entries = append(entries, FileEntry{
						Name:  name,
						Path:  bPath,
						IsDir: false,
						Size:  b.Size,
					})
				}
			} else if strings.HasPrefix(bDirClean, queryDir+"/") {
				// Deeper subdirectory — show intermediate directory.
				rel := bDirClean[len(queryDir)+1:]
				topLevel := strings.SplitN(rel, "/", 2)[0]
				if !seen[topLevel] {
					seen[topLevel] = true
					entries = append(entries, FileEntry{
						Name:  topLevel,
						Path:  queryDir + "/" + topLevel,
						IsDir: true,
					})
				}
			}
		}
	}
	if len(blobs) > 0 && len(entries) == 0 && queryDir != "" {
		return nil, ErrFileNotFound
	}
	return entries, nil
}

// ── History ───────────────────────────────────────────────────────────────────

// Log returns the commit history for the branch, newest to oldest.
// Optionally filtered to a specific file path via filter.Path.
// Returns [ErrBranchNotFound] if the branch does not exist.
func (m *gitManager) Log(ctx context.Context, branchID string, filter LogFilter) ([]CommitEntry, error) {
	branch, err := m.GetBranch(ctx, branchID)
	if err != nil {
		return nil, fmt.Errorf("Log: %w", err)
	}
	if branch.HeadCommitID == "" {
		return nil, nil
	}

	commits, err := m.walkCommitChain(ctx, branch.HeadCommitID, filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("Log: %w", err)
	}

	out := make([]CommitEntry, 0, len(commits))
	for _, c := range commits {
		if filter.Path != "" {
			// Check if this commit touched the requested path.
			if !m.commitTouchesPath(ctx, c.ID, filter.Path) {
				continue
			}
		}
		out = append(out, commitToEntry(c))
	}
	return out, nil
}

// Diff returns per-file change summaries between two refs (branch IDs or commit IDs).
// Returns [ErrRefNotFound] if either ref cannot be resolved.
func (m *gitManager) Diff(ctx context.Context, fromRef, toRef string) ([]FileDiff, error) {
	fromCommitID, err := m.resolveRef(ctx, fromRef)
	if err != nil {
		return nil, fmt.Errorf("Diff: fromRef %s: %w", fromRef, ErrRefNotFound)
	}
	toCommitID, err := m.resolveRef(ctx, toRef)
	if err != nil {
		return nil, fmt.Errorf("Diff: toRef %s: %w", toRef, ErrRefNotFound)
	}

	fromBlobs, err := m.allBlobsAtCommit(ctx, fromCommitID)
	if err != nil {
		return nil, fmt.Errorf("Diff: from blobs: %w", err)
	}
	toBlobs, err := m.allBlobsAtCommit(ctx, toCommitID)
	if err != nil {
		return nil, fmt.Errorf("Diff: to blobs: %w", err)
	}

	return diffBlobs(fromBlobs, toBlobs), nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// findBlobAtCommit traverses the commit's tree(s) to find a blob matching path.
func (m *gitManager) findBlobAtCommit(ctx context.Context, commitID, path string) (models.Blob, error) {
	blobs, err := m.allBlobsAtCommit(ctx, commitID)
	if err != nil {
		return models.Blob{}, err
	}
	for _, b := range blobs {
		if b.Path == path {
			return b, nil
		}
	}
	return models.Blob{}, ErrFileNotFound
}

// walkCommitChain returns startCommitID and its ancestors (newest-first),
// via [gormstore.CommitChainIDs], up to limit commits (0 = no limit).
func (m *gitManager) walkCommitChain(ctx context.Context, startCommitID string, limit int) ([]gormstore.CommitRow, error) {
	ids, err := gormstore.CommitChainIDs(m.db.WithContext(ctx), m.tables, startCommitID, limit)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []gormstore.CommitRow
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).
		Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	byID := make(map[string]gormstore.CommitRow, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	out := make([]gormstore.CommitRow, 0, len(ids))
	for _, id := range ids {
		if r, ok := byID[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

// commitTouchesPath reports whether the commit's tree contains a blob at path.
func (m *gitManager) commitTouchesPath(ctx context.Context, commitID, path string) bool {
	_, err := m.findBlobAtCommit(ctx, commitID, path)
	return err == nil
}

// resolveRef resolves a branchID or commitID to a commit row ID.
// It first tries GetBranch (to read HeadCommitID), then falls back to
// treating the ref as a raw commit row ID, then as a SHA.
func (m *gitManager) resolveRef(ctx context.Context, ref string) (string, error) {
	// Try as a branch ID first.
	branch, err := m.GetBranch(ctx, ref)
	if err == nil {
		if branch.HeadCommitID == "" {
			return "", fmt.Errorf("branch %s has no HEAD commit", ref)
		}
		return branch.HeadCommitID, nil
	}
	// Try as a commit row ID directly.
	var row gormstore.CommitRow
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).
		Where("id = ?", ref).First(&row).Error; err == nil {
		return ref, nil
	}
	// Try as a SHA.
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).
		Where("sha = ?", ref).First(&row).Error; err == nil {
		return row.ID, nil
	}
	return "", ErrRefNotFound
}

// ── Domain converters ─────────────────────────────────────────────────────────

// commitToEntry converts a Commit row to a [CommitEntry] for Log output.
func commitToEntry(r gormstore.CommitRow) CommitEntry {
	ts, _ := time.Parse(time.RFC3339, r.CommittedAt)
	if ts.IsZero() {
		ts, _ = time.Parse(time.RFC3339, r.AuthorAt)
	}
	return CommitEntry{
		SHA:       r.SHA,
		Author:    r.AuthorName,
		Message:   r.Message,
		Timestamp: ts,
	}
}

// diffBlobs computes added/modified/deleted file entries between two blob sets.
func diffBlobs(fromBlobs, toBlobs []models.Blob) []FileDiff {
	fromMap := make(map[string]models.Blob, len(fromBlobs))
	for _, b := range fromBlobs {
		fromMap[b.Path] = b
	}
	toMap := make(map[string]models.Blob, len(toBlobs))
	for _, b := range toBlobs {
		toMap[b.Path] = b
	}

	var diffs []FileDiff
	// Added or modified.
	for path, toBlob := range toMap {
		if fromBlob, ok := fromMap[path]; !ok {
			diffs = append(diffs, FileDiff{Path: path, Operation: "added"})
		} else if fromBlob.SHA != toBlob.SHA {
			diffs = append(diffs, FileDiff{Path: path, Operation: "modified"})
		}
	}
	// Deleted.
	for path := range fromMap {
		if _, ok := toMap[path]; !ok {
			diffs = append(diffs, FileDiff{Path: path, Operation: "deleted"})
		}
	}
	return diffs
}

// ── Path helpers ──────────────────────────────────────────────────────────────

// dirPath returns the directory component of a file path (empty for root files).
func dirPath(p string) string {
	idx := strings.LastIndex(p, "/")
	if idx < 0 {
		return ""
	}
	return p[:idx]
}

// fileName returns the base name of a file path.
func fileName(p string) string {
	idx := strings.LastIndex(p, "/")
	if idx < 0 {
		return p
	}
	return p[idx+1:]
}

// fileExtension returns the file extension without the leading dot, e.g. "txt".
// Returns an empty string for files with no extension or dotfiles (e.g. ".gitignore").
func fileExtension(p string) string {
	name := fileName(p)
	idx := strings.LastIndex(name, ".")
	if idx <= 0 {
		return ""
	}
	return name[idx+1:]
}
