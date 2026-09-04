package mwanachamagit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/entitygraph"
)

// fakeDataManager is a minimal in-process entitygraph.DataManager used to
// unit-test G4's ported business logic without a real Postgres backend.
// Real Postgres integration testing is G9's job (mwanachama-backend-shared#S4);
// this fake only needs to be correct enough to exercise the ported
// git_impl_*.go control flow.
type fakeDataManager struct {
	mu            sync.Mutex
	nextID        int
	entities      map[string]entitygraph.Entity
	relationships map[string]entitygraph.Relationship
	// relOrder preserves relationship creation order so ListRelationships is
	// deterministic (Go map iteration is randomized per-process), which
	// traversal tests over multi-edge frontiers rely on for a reproducible
	// BFS visiting order.
	relOrder []string
}

func newFakeDataManager() *fakeDataManager {
	return &fakeDataManager{
		entities:      make(map[string]entitygraph.Entity),
		relationships: make(map[string]entitygraph.Relationship),
	}
}

func (f *fakeDataManager) newID(prefix string) string {
	f.nextID++
	return fmt.Sprintf("%s%d", prefix, f.nextID)
}

func (f *fakeDataManager) CreateEntity(ctx context.Context, req entitygraph.CreateEntityRequest) (entitygraph.Entity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now().UTC()
	props := make(map[string]any, len(req.Properties))
	for k, v := range req.Properties {
		props[k] = v
	}
	e := entitygraph.Entity{
		ID:         f.newID("e"),
		TypeID:     req.TypeID,
		Properties: props,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	f.entities[e.ID] = e

	for _, rel := range req.Relationships {
		rID := f.newID("r")
		f.relationships[rID] = entitygraph.Relationship{
			ID:        rID,
			Name:      rel.Name,
			FromID:    e.ID,
			ToID:      rel.ToID,
			CreatedAt: now,
		}
		f.relOrder = append(f.relOrder, rID)
	}
	return e, nil
}

func (f *fakeDataManager) GetEntity(ctx context.Context, entityID string) (entitygraph.Entity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entities[entityID]
	if !ok || e.Deleted {
		return entitygraph.Entity{}, entitygraph.ErrEntityNotFound
	}
	return e, nil
}

func (f *fakeDataManager) UpdateEntity(ctx context.Context, entityID string, req entitygraph.UpdateEntityRequest) (entitygraph.Entity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entities[entityID]
	if !ok || e.Deleted {
		return entitygraph.Entity{}, entitygraph.ErrEntityNotFound
	}
	if e.Properties == nil {
		e.Properties = make(map[string]any, len(req.Properties))
	}
	for k, v := range req.Properties {
		e.Properties[k] = v
	}
	e.UpdatedAt = time.Now().UTC()
	f.entities[entityID] = e
	return e, nil
}

func (f *fakeDataManager) DeleteEntity(ctx context.Context, entityID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entities[entityID]
	if !ok || e.Deleted {
		return entitygraph.ErrEntityNotFound
	}
	now := time.Now().UTC()
	e.Deleted = true
	e.DeletedAt = &now
	f.entities[entityID] = e
	return nil
}

func (f *fakeDataManager) ListEntities(ctx context.Context, filter entitygraph.EntityFilter) ([]entitygraph.Entity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []entitygraph.Entity
	for _, e := range f.entities {
		if e.Deleted {
			continue
		}
		if filter.TypeID != "" && e.TypeID != filter.TypeID {
			continue
		}
		if !propsMatch(e.Properties, filter.Properties) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func (f *fakeDataManager) UpsertEntity(ctx context.Context, req entitygraph.CreateEntityRequest) (entitygraph.Entity, error) {
	return f.CreateEntity(ctx, req)
}

func (f *fakeDataManager) CreateRelationship(ctx context.Context, req entitygraph.CreateRelationshipRequest) (entitygraph.Relationship, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := entitygraph.Relationship{
		ID:         f.newID("r"),
		Name:       req.Name,
		FromID:     req.FromID,
		ToID:       req.ToID,
		Properties: req.Properties,
		CreatedAt:  time.Now().UTC(),
	}
	f.relationships[r.ID] = r
	f.relOrder = append(f.relOrder, r.ID)
	return r, nil
}

func (f *fakeDataManager) GetRelationship(ctx context.Context, relationshipID string) (entitygraph.Relationship, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.relationships[relationshipID]
	if !ok {
		return entitygraph.Relationship{}, entitygraph.ErrRelationshipNotFound
	}
	return r, nil
}

func (f *fakeDataManager) DeleteRelationship(ctx context.Context, relationshipID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.relationships[relationshipID]
	if !ok {
		return entitygraph.ErrRelationshipNotFound
	}
	delete(f.relationships, relationshipID)
	return nil
}

func (f *fakeDataManager) ListRelationships(ctx context.Context, filter entitygraph.RelationshipFilter) ([]entitygraph.Relationship, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []entitygraph.Relationship
	for _, id := range f.relOrder {
		r, ok := f.relationships[id]
		if !ok {
			continue // deleted since creation
		}
		if filter.FromID != "" && r.FromID != filter.FromID {
			continue
		}
		if filter.ToID != "" && r.ToID != filter.ToID {
			continue
		}
		if filter.Name != "" && r.Name != filter.Name {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// propsMatch reports whether entity properties p contain every key/value in
// want (exact match). A nil or empty want always matches.
func propsMatch(p, want map[string]any) bool {
	for k, v := range want {
		if p[k] != v {
			return false
		}
	}
	return true
}

// publishedEvent is one recorded [fakePublisher.Publish] call.
type publishedEvent struct {
	topic   string
	payload any
}

// fakePublisher is an in-process events.Publisher that records every call,
// for tests asserting on which topics fire and in what order.
type fakePublisher struct {
	mu     sync.Mutex
	events []publishedEvent
}

func (p *fakePublisher) Publish(_ context.Context, topic string, payload any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, publishedEvent{topic: topic, payload: payload})
	return nil
}

// published returns a snapshot of all recorded events.
func (p *fakePublisher) published() []publishedEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]publishedEvent(nil), p.events...)
}

// countByTopic returns how many recorded events have the given topic.
func countByTopic(events []publishedEvent, topic string) int {
	n := 0
	for _, e := range events {
		if e.topic == topic {
			n++
		}
	}
	return n
}

// hasTopic reports whether at least one event with the given topic appears
// in events.
func hasTopic(events []publishedEvent, topic string) bool {
	return countByTopic(events, topic) > 0
}

// updateEntityReq is a tiny convenience wrapper for tests that side-load
// state directly into the fakeDataManager via UpdateEntity.
func updateEntityReq(props map[string]any) entitygraph.UpdateEntityRequest {
	return entitygraph.UpdateEntityRequest{Properties: props}
}
