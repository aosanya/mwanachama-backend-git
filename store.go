package mwanachamagit

import (
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"
	"github.com/aosanya/mwanachama-backend-shared/specstore"

	"github.com/aosanya/mwanachama-backend-git/models"
)

const (
	roleRepository     = "repository"
	roleBranch         = "branch"
	roleCommit         = "commit"
	roleCommitParent   = "commit_parent"
	roleTree           = "tree"
	roleTreeBlob       = "tree_blob"
	roleTreeSubtree    = "tree_subtree"
	roleBlob           = "blob"
	roleTag            = "tag"
	roleMergeRequest   = "merge_request"
	roleKeyword        = "keyword"
	roleBlobKeywordTag = "blob_keyword_tag"
	roleBlobReference  = "blob_reference"
	roleImportJob      = "import_job"
	roleFetchBranchJob = "fetch_branch_job"
)

type store = specstore.Store

func carriers() map[string]any {
	return map[string]any{
		roleRepository:     models.Repository{},
		roleBranch:         models.Branch{},
		roleCommit:         models.Commit{},
		roleCommitParent:   models.CommitParent{},
		roleTree:           models.Tree{},
		roleTreeBlob:       models.TreeBlob{},
		roleTreeSubtree:    models.TreeSubtree{},
		roleBlob:           models.Blob{},
		roleTag:            models.Tag{},
		roleMergeRequest:   models.MergeRequest{},
		roleKeyword:        models.Keyword{},
		roleBlobKeywordTag: models.BlobKeywordTag{},
		roleBlobReference:  models.BlobReference{},
		roleImportJob:      models.ImportJob{},
		roleFetchBranchJob: models.FetchBranchJob{},
	}
}

func newStore(db *gorm.DB, s *spec.Spec) (*store, error) {
	return specstore.New(db, s, carriers())
}

func newID() string { return specstore.NewID() }

func columnName(field string) string { return specstore.ColumnName(field) }

func encode(o spec.Object, v any) (map[string]any, error) { return specstore.Encode(o, v) }

func decode(o spec.Object, row map[string]any, out any) error { return specstore.Decode(o, row, out) }

type tableSet struct {
	Repositories    string
	Branches        string
	MergeRequests   string
	Tags            string
	Commits         string
	CommitParents   string
	Trees           string
	TreeBlobs       string
	TreeSubtrees    string
	Blobs           string
	Keywords        string
	BlobKeywordTags string
	BlobReferences  string
	ImportJobs      string
	FetchBranchJobs string
}

func tablesOf(st *store) tableSet {
	return tableSet{
		Repositories:    st.Table(roleRepository),
		Branches:        st.Table(roleBranch),
		MergeRequests:   st.Table(roleMergeRequest),
		Tags:            st.Table(roleTag),
		Commits:         st.Table(roleCommit),
		CommitParents:   st.Table(roleCommitParent),
		Trees:           st.Table(roleTree),
		TreeBlobs:       st.Table(roleTreeBlob),
		TreeSubtrees:    st.Table(roleTreeSubtree),
		Blobs:           st.Table(roleBlob),
		Keywords:        st.Table(roleKeyword),
		BlobKeywordTags: st.Table(roleBlobKeywordTag),
		BlobReferences:  st.Table(roleBlobReference),
		ImportJobs:      st.Table(roleImportJob),
		FetchBranchJobs: st.Table(roleFetchBranchJob),
	}
}
