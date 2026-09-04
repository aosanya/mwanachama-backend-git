package gormstore

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-git/models"
)

// CommitRow is the GORM row for a [models.Commit]. TreeID replaces the old
// has_tree edge. SHA is intentionally NOT unique — entitygraph never
// enforced SHA uniqueness for Commit either (every write went through
// CreateEntity, not UpsertEntity, so the declared UniqueKey was never
// applied); adding a unique index here would be a behavior change, not a
// mechanical port. ParentIDs has no column — see [CommitParentRow].
type CommitRow struct {
	ID             string  `gorm:"primaryKey"`
	RepositoryID   *string `gorm:"index"`
	SHA            string  `gorm:"index"`
	Message        string
	AuthorName     string
	AuthorEmail    string
	AuthorAt       string
	CommitterName  string
	CommitterEmail string
	CommittedAt    string
	TreeID         *string `gorm:"index"`
	// Data is the base64-encoded raw git commit object, written by
	// WriteFile only (empty for commits materialised by FetchBranch/Import,
	// which never re-encode the original git object).
	Data      string
	Size      int64
	CreatedAt string
	Deleted   bool `gorm:"index"`
}

func (r *CommitRow) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

// CommitToRow converts a domain Commit to its row shape. ParentIDs is
// ignored here — see [CommitParentsToRows].
func CommitToRow(c models.Commit) CommitRow {
	return CommitRow{
		ID:             c.ID,
		RepositoryID:   StringToNullable(c.RepositoryID),
		SHA:            c.SHA,
		Message:        c.Message,
		AuthorName:     c.AuthorName,
		AuthorEmail:    c.AuthorEmail,
		AuthorAt:       c.AuthorAt,
		CommitterName:  c.CommitterName,
		CommitterEmail: c.CommitterEmail,
		CommittedAt:    c.CommittedAt,
		TreeID:         StringToNullable(c.TreeID),
		CreatedAt:      c.CreatedAt,
	}
}

// CommitFromRow converts a row back to the domain Commit. ParentIDs is left
// nil — the caller attaches it separately from [CommitParentRow] rows (see
// e.g. the root package's entityToCommit-equivalent), since that requires a
// second query this pure function has no db handle to make.
func CommitFromRow(r CommitRow) models.Commit {
	return models.Commit{
		ID:             r.ID,
		RepositoryID:   NullableToString(r.RepositoryID),
		SHA:            r.SHA,
		Message:        r.Message,
		AuthorName:     r.AuthorName,
		AuthorEmail:    r.AuthorEmail,
		AuthorAt:       r.AuthorAt,
		CommitterName:  r.CommitterName,
		CommitterEmail: r.CommitterEmail,
		CommittedAt:    r.CommittedAt,
		TreeID:         NullableToString(r.TreeID),
		CreatedAt:      r.CreatedAt,
	}
}

// CommitParentRow is the GORM row for one (commit, parent) edge, replacing
// the old has_parent relationship. ParentIndex preserves parent order (0 =
// first parent, 1+ = merge parents) — order that used to fall out of edge
// creation order and is now an explicit column.
type CommitParentRow struct {
	CommitID    string `gorm:"primaryKey"`
	ParentID    string `gorm:"primaryKey;index"`
	ParentIndex int
}

// CommitParentsToRows builds the join rows for a commit's ParentIDs, in order.
func CommitParentsToRows(commitID string, parentIDs []string) []CommitParentRow {
	rows := make([]CommitParentRow, len(parentIDs))
	for i, pid := range parentIDs {
		rows[i] = CommitParentRow{CommitID: commitID, ParentID: pid, ParentIndex: i}
	}
	return rows
}
