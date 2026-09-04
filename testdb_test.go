package mwanachamagit

import (
	"context"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// newTestManager builds a *gitManager backed by a fresh in-memory sqlite
// database, migrated the same way a real deployment would via [Migrate].
// Replaces the old hand-maintained fakeDataManager: exercising real GORM/SQL
// behavior catches more than a Go map fake ever could, while staying fully
// in-process — no containers, no POSTGRES_URL, consistent with this repo's
// existing separation between fast unit tests here and the opt-in
// postgres_integration_test.go.
//
// Stays internal to package mwanachamagit (not mwanachamagit_test): the
// existing test suite reaches unexported fields (m.db, m.tables) and helpers
// directly, unlike mwanachama-backend-actor's external test package.
func newTestManager(t *testing.T) *gitManager {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	// Concurrency tests in this package run goroutines against the same
	// *gitManager. A pooled sqlite ":memory:" connection hands each
	// goroutine a different anonymous in-memory database unless pinned to a
	// single connection — mwanachama-backend-actor never hit this since its
	// tests are single-goroutine.
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	tables := DefaultTableNames("test")
	if err := Migrate(db, tables); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return &gitManager{db: db, tables: tables, locker: &mutexLocker{}}
}

// newTestManagerWithPublisher is newTestManager plus a fakePublisher for
// tests asserting on which event topics fire.
func newTestManagerWithPublisher(t *testing.T) (*gitManager, *fakePublisher) {
	t.Helper()
	m := newTestManager(t)
	pub := &fakePublisher{}
	m.publisher = pub
	return m, pub
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

func TestNewGitManager_NilDB(t *testing.T) {
	if _, err := NewGitManager(nil, DefaultTableNames("test"), nil, nil, nil); err == nil {
		t.Fatal("expected error for nil db")
	}
}
