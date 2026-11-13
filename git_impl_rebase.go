package mwanachamagit

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"gorm.io/gorm/clause"

	"github.com/aosanya/mwanachama-backend-git/models"
)

type RebaseBranchRequest struct {
	BranchID     string `json:"branch_id"`
	OntoBranchID string `json:"onto_branch_id,omitempty"`
}

type replayedCommit struct {
	original models.Commit
	merged   mergeTree
}

func (m *gitManager) RebaseBranch(ctx context.Context, req RebaseBranchRequest) (models.Branch, error) {
	var result models.Branch
	lockErr := m.locker.WithMergeLock(ctx, func() error {
		branch, err := m.GetBranch(ctx, req.BranchID)
		if err != nil {
			return fmt.Errorf("get branch: %w", err)
		}
		target, err := m.rebaseTarget(ctx, branch, req.OntoBranchID)
		if err != nil {
			return err
		}
		result, err = m.rebaseOnto(ctx, branch, target)
		return err
	})
	if lockErr != nil {
		return models.Branch{}, fmt.Errorf("RebaseBranch: %w", lockErr)
	}
	return result, nil
}

func (m *gitManager) rebaseTarget(ctx context.Context, branch models.Branch, ontoBranchID string) (models.Branch, error) {
	if ontoBranchID == "" {
		target, err := m.defaultBranch(ctx, branch.RepositoryID)
		if err != nil {
			return models.Branch{}, fmt.Errorf("default branch: %w", err)
		}
		return target, nil
	}
	target, err := m.GetBranch(ctx, ontoBranchID)
	if err != nil {
		return models.Branch{}, fmt.Errorf("get target branch: %w", err)
	}
	if target.RepositoryID != branch.RepositoryID {
		return models.Branch{}, fmt.Errorf("target branch %s belongs to another repository", target.ID)
	}
	return target, nil
}

func (m *gitManager) rebaseOnto(ctx context.Context, branch, target models.Branch) (models.Branch, error) {
	if branch.HeadCommitID == "" || target.HeadCommitID == "" || branch.HeadCommitID == target.HeadCommitID {
		return branch, nil
	}
	base, err := m.mergeBase(ctx, target.HeadCommitID, branch.HeadCommitID)
	if err != nil {
		return models.Branch{}, fmt.Errorf("merge base: %w", err)
	}
	if base == target.HeadCommitID {
		return branch, nil
	}
	if base == branch.HeadCommitID {
		return m.advanceBranchHead(ctx, branch.ID, target.HeadCommitID, branch.HeadCommitID)
	}

	own, err := m.ownCommits(ctx, branch.HeadCommitID, base)
	if err != nil {
		return models.Branch{}, err
	}
	replay, err := m.replayTrees(ctx, branch, target, own)
	if err != nil {
		return models.Branch{}, err
	}

	parentID := target.HeadCommitID
	for _, step := range replay {
		written, err := m.writeReplayedCommit(ctx, branch.RepositoryID, step, parentID)
		if err != nil {
			return models.Branch{}, err
		}
		parentID = written.ID
	}
	return m.advanceBranchHead(ctx, branch.ID, parentID, branch.HeadCommitID)
}

func (m *gitManager) ownCommits(ctx context.Context, headID, baseID string) ([]models.Commit, error) {
	var chain []models.Commit
	for current := headID; current != baseID; {
		var row models.Commit
		if err := m.db.WithContext(ctx).Table(m.tables.Commits).
			Where("id = ?", current).First(&row).Error; err != nil {
			return nil, fmt.Errorf("get commit %s: %w", current, err)
		}
		var links []models.CommitParent
		if err := m.db.WithContext(ctx).Table(m.tables.CommitParents).
			Where("commit_id = ?", current).Order("parent_index").Find(&links).Error; err != nil {
			return nil, fmt.Errorf("get parents of commit %s: %w", current, err)
		}
		if len(links) > 1 {
			return nil, fmt.Errorf("rebase of merge commits is not supported (commit %s)", row.SHA)
		}
		if len(links) == 0 {
			return nil, fmt.Errorf("commit %s does not descend from merge base %s", headID, baseID)
		}
		row.ParentIDs = []string{links[0].ParentID}
		chain = append(chain, row)
		current = links[0].ParentID
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

func (m *gitManager) replayTrees(ctx context.Context, branch, target models.Branch, own []models.Commit) ([]replayedCommit, error) {
	tip, err := m.mergeTreeAt(ctx, target.HeadCommitID)
	if err != nil {
		return nil, err
	}
	replay := make([]replayedCommit, 0, len(own))
	for _, commit := range own {
		parentTree, err := m.mergeTreeAt(ctx, commit.ParentIDs[0])
		if err != nil {
			return nil, err
		}
		commitTree, err := m.mergeTreeAt(ctx, commit.ID)
		if err != nil {
			return nil, err
		}
		merged, overlapping := mergeTrees(parentTree, tip, commitTree)
		if len(overlapping) > 0 {
			return nil, &ErrMergeConflict{TaskID: branch.Name, ConflictingFiles: overlapping}
		}
		tip = merged
		replay = append(replay, replayedCommit{original: commit, merged: merged})
	}
	return replay, nil
}

func (m *gitManager) writeReplayedCommit(ctx context.Context, repositoryID string, step replayedCommit, parentID string) (models.Commit, error) {
	commitTime := time.Now().UTC()
	now := commitTime.Format(time.RFC3339)

	var parentRow models.Commit
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).
		Where("id = ?", parentID).First(&parentRow).Error; err != nil {
		return models.Commit{}, fmt.Errorf("get parent commit %s: %w", parentID, err)
	}

	fileMap := make(map[string]plumbing.Hash, len(step.merged))
	blobIDs := make([]string, 0, len(step.merged))
	for path, entry := range step.merged {
		fileMap[path] = plumbing.NewHash(entry.SHA)
		blobIDs = append(blobIDs, entry.BlobID)
	}
	rootTreeHash, treeRecords, err := buildNestedTrees(fileMap)
	if err != nil {
		return models.Commit{}, fmt.Errorf("build trees: %w", err)
	}

	var rootTreeID string
	for _, tr := range treeRecords {
		treeRow := models.Tree{SHA: tr.sha, Path: tr.path, CreatedAt: now}
		treeRow.Entries = tr.entries
		treeRow.Data = base64.StdEncoding.EncodeToString(tr.rawData)
		treeRow.Size = tr.size
		ensureID(&treeRow)
		if err := m.db.WithContext(ctx).Table(m.tables.Trees).Create(&treeRow).Error; err != nil {
			return models.Commit{}, fmt.Errorf("create tree row path=%q: %w", tr.path, err)
		}
		if tr.sha == rootTreeHash.String() {
			rootTreeID = treeRow.ID
		}
	}

	authoredAt, parseErr := time.Parse(time.RFC3339, step.original.AuthorAt)
	if parseErr != nil {
		authoredAt = commitTime
	}
	author := object.Signature{Name: step.original.AuthorName, Email: step.original.AuthorEmail, When: authoredAt}
	committer := object.Signature{Name: mergeAuthorName, Email: mergeAuthorEmail, When: commitTime}
	gitCommitObj := &object.Commit{
		TreeHash:     rootTreeHash,
		ParentHashes: []plumbing.Hash{plumbing.NewHash(parentRow.SHA)},
		Author:       author,
		Committer:    committer,
		Message:      step.original.Message,
	}
	commitMemObj := &plumbing.MemoryObject{}
	if err := gitCommitObj.Encode(commitMemObj); err != nil {
		return models.Commit{}, fmt.Errorf("encode commit: %w", err)
	}
	commitReader, err := commitMemObj.Reader()
	if err != nil {
		return models.Commit{}, fmt.Errorf("read encoded commit: %w", err)
	}
	commitRaw, err := io.ReadAll(commitReader)
	_ = commitReader.Close()
	if err != nil {
		return models.Commit{}, fmt.Errorf("read encoded commit: %w", err)
	}

	commitRow := models.Commit{
		SHA:            commitMemObj.Hash().String(),
		Message:        step.original.Message,
		AuthorName:     step.original.AuthorName,
		AuthorEmail:    step.original.AuthorEmail,
		AuthorAt:       step.original.AuthorAt,
		CommitterName:  mergeAuthorName,
		CommitterEmail: mergeAuthorEmail,
		CommittedAt:    now,
		TreeID:         rootTreeID,
		CreatedAt:      now,
	}
	commitRow.RepositoryID = repositoryID
	commitRow.Data = base64.StdEncoding.EncodeToString(commitRaw)
	commitRow.Size = commitMemObj.Size()
	ensureID(&commitRow)
	if err := m.db.WithContext(ctx).Table(m.tables.Commits).Create(&commitRow).Error; err != nil {
		return models.Commit{}, fmt.Errorf("create commit: %w", err)
	}

	parentRows := commitParentsOf(commitRow.ID, []string{parentID})
	ensureID(&parentRows)
	if err := m.db.WithContext(ctx).Table(m.tables.CommitParents).Create(&parentRows).Error; err != nil {
		return models.Commit{}, fmt.Errorf("link parent: %w", err)
	}

	treeBlobRows := make([]models.TreeBlob, 0, len(blobIDs))
	for _, blobID := range blobIDs {
		treeBlobRows = append(treeBlobRows, models.TreeBlob{TreeID: rootTreeID, BlobID: blobID})
	}
	if len(treeBlobRows) > 0 {
		if err := m.db.WithContext(ctx).Table(m.tables.TreeBlobs).
			Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&treeBlobRows, 200).Error; err != nil {
			log.Printf("rebase commit: link tree_blobs: %v (non-fatal)", err)
		}
	}

	commitRow.ParentIDs = []string{parentID}
	return commitRow, nil
}
