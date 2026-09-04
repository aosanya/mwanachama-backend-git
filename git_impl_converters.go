// git_impl_converters.go — entity→domain converters and shared graph lookup
// utilities for [gitManager].
//
// Property helpers (StringProp, BoolProp, Int64Prop) live in
// [github.com/aosanya/mwanachama-backend-shared/entitygraph] and are used directly.
package mwanachamagit

import (
	"context"
	"errors"
	"fmt"

	"github.com/aosanya/mwanachama-backend-shared/entitygraph"
)

// ── Entity → domain converters ────────────────────────────────────────────────

// entityToRepository maps an entitygraph.Entity of type "Repository" to [Repository].
func entityToRepository(e entitygraph.Entity) Repository {
	p := e.Properties
	return Repository{
		ID:            e.ID,
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

// allBlobsAtCommitMaxDepth bounds the outbound walk in [allBlobsAtCommit]:
// commit → tree → subtree* → blob. Five hops comfortably covers realistic
// directory nesting; deeper trees simply stop contributing blobs past that
// point rather than erroring.
const allBlobsAtCommitMaxDepth = 5

// allBlobsAtCommitEdgeNames are the only edge labels [allBlobsAtCommit]
// follows outbound from the commit: has_tree (commit → root Tree),
// has_subtree (Tree → child Tree), and has_blob (Tree → Blob).
var allBlobsAtCommitEdgeNames = map[string]bool{
	"has_tree":    true,
	"has_blob":    true,
	"has_subtree": true,
}

// allBlobsAtCommit returns all Blob entities reachable from the commit's
// tree, walking only has_tree/has_subtree/has_blob edges outbound from
// commitID.
//
// Ported off entitygraph.TraverseGraph (a single recursive-CTE query in the
// old multi-tenant Postgres DataManager) onto a manual outbound BFS using
// only ListRelationships/GetEntity, since TraverseGraph was removed from the
// DataManager interface entirely. Cost is one ListRelationships call per
// visited Tree (Blob and Commit nodes are leaves/roots and are never
// expanded further) plus one GetEntity per visited vertex — bounded by
// [allBlobsAtCommitMaxDepth] hops, not by a node cap (unlike
// GetNeighborhood, every blob under the commit's tree is expected to come
// back, not just the first N).
func (m *gitManager) allBlobsAtCommit(ctx context.Context, commitID string) ([]Blob, error) {
	visited := map[string]bool{commitID: true}
	frontier := []string{commitID}

	for level := 0; level < allBlobsAtCommitMaxDepth && len(frontier) > 0; level++ {
		var next []string
		for _, id := range frontier {
			rels, err := m.dm.ListRelationships(ctx, entitygraph.RelationshipFilter{FromID: id})
			if err != nil {
				return nil, fmt.Errorf("allBlobsAtCommit %s: list relationships from %s: %w", commitID, id, err)
			}
			for _, rel := range rels {
				if !allBlobsAtCommitEdgeNames[rel.Name] || visited[rel.ToID] {
					continue
				}
				visited[rel.ToID] = true
				next = append(next, rel.ToID)
			}
		}
		frontier = next
	}

	var blobs []Blob
	for id := range visited {
		if id == commitID {
			continue
		}
		e, err := m.dm.GetEntity(ctx, id)
		if err != nil {
			if errors.Is(err, entitygraph.ErrEntityNotFound) {
				continue // deleted after the edge referencing it was created
			}
			return nil, fmt.Errorf("allBlobsAtCommit %s: get entity %s: %w", commitID, id, err)
		}
		if e.TypeID == "Blob" {
			blobs = append(blobs, entityToBlob(e))
		}
	}
	return blobs, nil
}

// ── Shared graph helpers ──────────────────────────────────────────────────────

// resolveParentID returns the first ToID for an outbound relationship with the
// given name from the entity identified by entityID. Returns "" on any error.
func (m *gitManager) resolveParentID(ctx context.Context, entityID, relName string) string {
	rels, err := m.dm.ListRelationships(ctx, entitygraph.RelationshipFilter{
		Name:   relName,
		FromID: entityID,
	})
	if err != nil || len(rels) == 0 {
		return ""
	}
	return rels[0].ToID
}
