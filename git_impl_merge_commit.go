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

const (
	mergeAuthorName  = "mwanachama-bot"
	mergeAuthorEmail = "bot@mwanachama.local"
)

func (m *gitManager) writeMergeCommit(ctx context.Context, repositoryID string, merged mergeTree, parentIDs []string, message string) (models.Commit, error) {
	commitTime := time.Now().UTC()
	now := commitTime.Format(time.RFC3339)

	parentHashes := make([]plumbing.Hash, 0, len(parentIDs))
	for _, parentID := range parentIDs {
		var parentRow models.Commit
		if err := m.db.WithContext(ctx).Table(m.tables.Commits).
			Where("id = ?", parentID).First(&parentRow).Error; err != nil {
			return models.Commit{}, fmt.Errorf("merge commit: get parent commit %s: %w", parentID, err)
		}
		parentHashes = append(parentHashes, plumbing.NewHash(parentRow.SHA))
	}

	fileMap := make(map[string]plumbing.Hash, len(merged))
	blobIDs := make([]string, 0, len(merged))
	for path, entry := range merged {
		fileMap[path] = plumbing.NewHash(entry.SHA)
		blobIDs = append(blobIDs, entry.BlobID)
	}

	rootTreeHash, treeRecords, err := buildNestedTrees(fileMap)
	if err != nil {
		return models.Commit{}, fmt.Errorf("merge commit: %w", err)
	}

	var rootTreeID string
	for _, tr := range treeRecords {
		treeRow := models.Tree{SHA: tr.sha, Path: tr.path, CreatedAt: now}
		treeRow.Entries = tr.entries
		treeRow.Data = base64.StdEncoding.EncodeToString(tr.rawData)
		treeRow.Size = tr.size
		ensureID(&treeRow)
		if err := m.db.WithContext(ctx).Table(m.tables.Trees).Create(&treeRow).Error; err != nil {
			return models.Commit{}, fmt.Errorf("merge commit: create tree row path=%q: %w", tr.path, err)
		}
		if tr.sha == rootTreeHash.String() {
			rootTreeID = treeRow.ID
		}
	}

	signature := object.Signature{Name: mergeAuthorName, Email: mergeAuthorEmail, When: commitTime}
	gitCommitObj := &object.Commit{
		TreeHash:     rootTreeHash,
		ParentHashes: parentHashes,
		Author:       signature,
		Committer:    signature,
		Message:      message,
	}
	commitMemObj := &plumbing.MemoryObject{}
	if err := gitCommitObj.Encode(commitMemObj); err != nil {
		return models.Commit{}, fmt.Errorf("merge commit: encode commit: %w", err)
	}
	commitR, _ := commitMemObj.Reader()
	commitRaw, _ := io.ReadAll(commitR)
	_ = commitR.Close()

	commitRow := models.Commit{
		SHA:            commitMemObj.Hash().String(),
		Message:        message,
		AuthorName:     mergeAuthorName,
		AuthorEmail:    mergeAuthorEmail,
		AuthorAt:       now,
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
		return models.Commit{}, fmt.Errorf("merge commit: create commit: %w", err)
	}

	parentRows := commitParentsOf(commitRow.ID, parentIDs)
	ensureID(&parentRows)
	if err := m.db.WithContext(ctx).Table(m.tables.CommitParents).Create(&parentRows).Error; err != nil {
		return models.Commit{}, fmt.Errorf("merge commit: link parents: %w", err)
	}

	treeBlobRows := make([]models.TreeBlob, 0, len(blobIDs))
	for _, blobID := range blobIDs {
		treeBlobRows = append(treeBlobRows, models.TreeBlob{TreeID: rootTreeID, BlobID: blobID})
	}
	if len(treeBlobRows) > 0 {
		if err := m.db.WithContext(ctx).Table(m.tables.TreeBlobs).
			Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&treeBlobRows, 200).Error; err != nil {
			log.Printf("merge commit: link tree_blobs: %v (non-fatal)", err)
		}
	}

	commit := commitRow
	commit.RepositoryID = repositoryID
	commit.ParentIDs = parentIDs
	return commit, nil
}
