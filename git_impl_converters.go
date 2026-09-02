// git_impl_converters.go — entity→domain converters and shared graph lookup
// utilities for [gitManager].
//
// Property helpers (StringProp, BoolProp, Int64Prop) live in
// [github.com/aosanya/mwanachama-go-shared/entitygraph] and are used directly.
package mwanachamagit

import (
	"context"

	"github.com/aosanya/mwanachama-go-shared/entitygraph"
)

// ── Entity → domain converters ────────────────────────────────────────────────

// entityToRepository maps an entitygraph.Entity of type "Repository" to [Repository].
func entityToRepository(e entitygraph.Entity, agencyID string) Repository {
	p := e.Properties
	return Repository{
		ID:            e.ID,
		AgencyID:      agencyID,
		Name:          entitygraph.StringProp(p, "name"),
		Description:   entitygraph.StringProp(p, "description"),
		DefaultBranch: entitygraph.StringProp(p, "default_branch"),
		CreatedAt:     entitygraph.StringProp(p, "created_at"),
		UpdatedAt:     entitygraph.StringProp(p, "updated_at"),
		SourceURL:     entitygraph.StringProp(p, "source_url"),
	}
}

// entityToBranch maps an entitygraph.Entity of type "Branch" to [Branch].
func entityToBranch(e entitygraph.Entity, repositoryID string) Branch {
	p := e.Properties
	return Branch{
		ID:            e.ID,
		RepositoryID:  repositoryID,
		Name:          entitygraph.StringProp(p, "name"),
		IsDefault:     entitygraph.BoolProp(p, "is_default"),
		HeadCommitID:  entitygraph.StringProp(p, "head_commit_id"),
		SHA:           entitygraph.StringProp(p, "sha"),
		WorkflowRunID: entitygraph.StringProp(p, "workflow_run_id"),
		CreatedAt:     entitygraph.StringProp(p, "created_at"),
		UpdatedAt:     entitygraph.StringProp(p, "updated_at"),
	}
}

// entityToTag maps an entitygraph.Entity of type "Tag" to [Tag].
func entityToTag(e entitygraph.Entity, repositoryID string) Tag {
	p := e.Properties
	return Tag{
		ID:           e.ID,
		RepositoryID: repositoryID,
		Name:         entitygraph.StringProp(p, "name"),
		SHA:          entitygraph.StringProp(p, "sha"),
		Message:      entitygraph.StringProp(p, "message"),
		TaggerName:   entitygraph.StringProp(p, "tagger_name"),
		TaggerAt:     entitygraph.StringProp(p, "tagger_at"),
		CreatedAt:    entitygraph.StringProp(p, "created_at"),
	}
}

// entityToBlob maps an entitygraph.Entity of type "Blob" to [Blob].
//
// Pulled forward from what will become git_impl_fileops.go (G5) because
// [gitManager.allBlobsAtCommit] below — needed by G4's edgelifecycle
// implementation — depends on it and neither function touches go-git; both
// are pure entitygraph.DataManager calls despite living in the
// go-git-touching source file upstream.
func entityToBlob(e entitygraph.Entity) Blob {
	p := e.Properties
	return Blob{
		ID:        e.ID,
		SHA:       entitygraph.StringProp(p, "sha"),
		Path:      entitygraph.StringProp(p, "path"),
		Name:      entitygraph.StringProp(p, "name"),
		Extension: entitygraph.StringProp(p, "extension"),
		Size:      entitygraph.Int64Prop(p, "size"),
		Encoding:  entitygraph.StringProp(p, "encoding"),
		Content:   entitygraph.StringProp(p, "content"),
		CreatedAt: entitygraph.StringProp(p, "created_at"),
	}
}

// allBlobsAtCommit returns all Blob entities reachable from the commit's tree.
func (m *gitManager) allBlobsAtCommit(ctx context.Context, commitID string) ([]Blob, error) {
	// Traverse outbound from commit: has_tree → has_blob / has_subtree.
	result, err := m.dm.TraverseGraph(ctx, entitygraph.TraverseGraphRequest{
		AgencyID:  m.agencyID,
		StartID:   commitID,
		Direction: "outbound",
		Depth:     5,
		Names:     []string{"has_tree", "has_blob", "has_subtree"},
	})
	if err != nil {
		return nil, err
	}
	var blobs []Blob
	for _, v := range result.Vertices {
		if v.TypeID == "Blob" {
			blobs = append(blobs, entityToBlob(v))
		}
	}
	return blobs, nil
}

// ── Shared graph helpers ──────────────────────────────────────────────────────

// resolveParentID returns the first ToID for an outbound relationship with the
// given name from the entity identified by entityID. Returns "" on any error.
func (m *gitManager) resolveParentID(ctx context.Context, entityID, relName string) string {
	rels, err := m.dm.ListRelationships(ctx, entitygraph.RelationshipFilter{
		AgencyID: m.agencyID,
		Name:     relName,
		FromID:   entityID,
	})
	if err != nil || len(rels) == 0 {
		return ""
	}
	return rels[0].ToID
}
