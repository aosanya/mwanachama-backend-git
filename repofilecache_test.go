package mwanachamagit

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type openFileTracker struct {
	mu       sync.Mutex
	current  int64
	peak     int64
	opens    map[string]int
	closed   []string
	paths    map[*gorm.DB]string
	failOpen map[string]error
}

func newOpenFileTracker() *openFileTracker {
	return &openFileTracker{
		opens:    map[string]int{},
		paths:    map[*gorm.DB]string{},
		failOpen: map[string]error{},
	}
}

func (k *openFileTracker) open(path string) (*gorm.DB, error) {
	k.mu.Lock()
	failure := k.failOpen[path]
	k.mu.Unlock()
	if failure != nil {
		return nil, failure
	}
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(10000)"), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	now := atomic.AddInt64(&k.current, 1)
	k.mu.Lock()
	if now > k.peak {
		k.peak = now
	}
	k.opens[path]++
	k.paths[db] = path
	k.mu.Unlock()
	return db, nil
}

func (k *openFileTracker) closeHandle(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	if err := sqlDB.Close(); err != nil {
		return err
	}
	atomic.AddInt64(&k.current, -1)
	k.mu.Lock()
	k.closed = append(k.closed, k.paths[db])
	k.mu.Unlock()
	return nil
}

func (k *openFileTracker) currentlyOpen() int {
	return int(atomic.LoadInt64(&k.current))
}

func (k *openFileTracker) peakOpen() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return int(k.peak)
}

func (k *openFileTracker) openCount(path string) int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.opens[path]
}

func (k *openFileTracker) closedPaths() []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]string(nil), k.closed...)
}

func (k *openFileTracker) failOpening(path string, err error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if err == nil {
		delete(k.failOpen, path)
		return
	}
	k.failOpen[path] = err
}

func newTrackedRepoFileCache(t *testing.T, max int) (*RepoFileCache[*gorm.DB], *openFileTracker) {
	t.Helper()
	tracker := newOpenFileTracker()
	cache, err := NewRepoFileCache(max, tracker.open, tracker.closeHandle)
	if err != nil {
		t.Fatalf("NewRepoFileCache(%d): %v", max, err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	return cache, tracker
}

func repoFilePaths(t *testing.T, n int) []string {
	t.Helper()
	dir := t.TempDir()
	paths := make([]string, n)
	for i := range paths {
		paths[i] = filepath.Join(dir, fmt.Sprintf("repo-%02d.db", i))
	}
	return paths
}

func writeRepoRow(db *gorm.DB, value string) error {
	if err := db.Exec("CREATE TABLE IF NOT EXISTS items (id INTEGER PRIMARY KEY AUTOINCREMENT, v TEXT)").Error; err != nil {
		return err
	}
	return db.Exec("INSERT INTO items (v) VALUES (?)", value).Error
}

func readRepoRows(db *gorm.DB) ([]string, error) {
	var values []string
	err := db.Raw("SELECT v FROM items ORDER BY id").Scan(&values).Error
	return values, err
}

func TestRepoFileCache_MoreRepositoriesThanTheMaximumStayWithinTheMaximum(t *testing.T) {
	const max = 3
	cache, tracker := newTrackedRepoFileCache(t, max)
	paths := repoFilePaths(t, 10)

	within := func(step string) {
		t.Helper()
		if got := cache.Open(); got > max {
			t.Fatalf("%s: Open() = %d, want <= %d", step, got, max)
		}
	}

	for i, path := range paths {
		want := fmt.Sprintf("row-%d", i)
		if err := cache.With(path, func(db *gorm.DB) error { return writeRepoRow(db, want) }); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		within("write pass")
	}

	check := func(pass string, order []int) {
		for _, i := range order {
			want := fmt.Sprintf("row-%d", i)
			err := cache.With(paths[i], func(db *gorm.DB) error {
				got, err := readRepoRows(db)
				if err != nil {
					return err
				}
				if len(got) != 1 || got[0] != want {
					return fmt.Errorf("rows = %v, want [%s]", got, want)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("%s %s: %v", pass, paths[i], err)
			}
			within(pass)
		}
	}

	forward := make([]int, len(paths))
	reversed := make([]int, len(paths))
	for i := range paths {
		forward[i] = i
		reversed[len(paths)-1-i] = i
	}
	check("second pass", forward)
	check("reversed pass", reversed)

	if peak := tracker.peakOpen(); peak > max {
		t.Fatalf("peak open handles = %d, want <= %d", peak, max)
	}
	if got := tracker.currentlyOpen(); got != cache.Open() {
		t.Fatalf("tracker sees %d open handles, cache reports %d", got, cache.Open())
	}
}

func TestRepoFileCache_LeastRecentlyUsedIsTheOneClosed(t *testing.T) {
	cache, tracker := newTrackedRepoFileCache(t, 2)
	paths := repoFilePaths(t, 3)
	a, b, c := paths[0], paths[1], paths[2]

	touch := func(path string) {
		t.Helper()
		if err := cache.With(path, func(db *gorm.DB) error { return writeRepoRow(db, path) }); err != nil {
			t.Fatalf("With(%s): %v", path, err)
		}
	}

	touch(a)
	touch(b)
	touch(a)
	touch(c)

	closed := tracker.closedPaths()
	if len(closed) != 1 || closed[0] != b {
		t.Fatalf("closed paths = %v, want only %s", closed, b)
	}
	if got := tracker.openCount(a); got != 1 {
		t.Fatalf("%s was opened %d times, want 1 (touching it again must not reopen it)", a, got)
	}
	if got := cache.Open(); got != 2 {
		t.Fatalf("Open() = %d, want 2", got)
	}
}

func TestRepoFileCache_AFileInUseIsNeverClosedAndTheNextCallWaits(t *testing.T) {
	cache, tracker := newTrackedRepoFileCache(t, 1)
	paths := repoFilePaths(t, 2)
	first, second := paths[0], paths[1]

	holding := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- cache.With(first, func(db *gorm.DB) error {
			if err := writeRepoRow(db, "held"); err != nil {
				return err
			}
			close(holding)
			<-release
			rows, err := readRepoRows(db)
			if err != nil {
				return err
			}
			if len(rows) != 1 || rows[0] != "held" {
				return fmt.Errorf("handle was disturbed while in use: rows = %v", rows)
			}
			return nil
		})
	}()
	<-holding

	var secondFinished atomic.Bool
	secondDone := make(chan error, 1)
	go func() {
		err := cache.With(second, func(db *gorm.DB) error { return writeRepoRow(db, "second") })
		secondFinished.Store(true)
		secondDone <- err
	}()

	time.Sleep(300 * time.Millisecond)
	if secondFinished.Load() {
		t.Fatalf("second call finished while the only slot was still in use")
	}
	if closed := tracker.closedPaths(); len(closed) != 0 {
		t.Fatalf("closed %v while %s was in use", closed, first)
	}

	close(release)
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first call: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("first call never returned")
	}
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("second call: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("second call never got the slot after the first returned")
	}

	if peak := tracker.peakOpen(); peak != 1 {
		t.Fatalf("peak open handles = %d, want 1", peak)
	}
}

func TestRepoFileCache_ConcurrentReadersAndWritersAcrossMoreFilesThanTheMaximum(t *testing.T) {
	const (
		max        = 3
		files      = 8
		goroutines = 24
		rounds     = 15
	)
	cache, tracker := newTrackedRepoFileCache(t, max)
	paths := repoFilePaths(t, files)

	var wg sync.WaitGroup
	failures := make(chan error, goroutines*rounds)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for r := 0; r < rounds; r++ {
				path := paths[(g+r*3)%files]
				err := cache.With(path, func(db *gorm.DB) error {
					if (g+r)%2 == 0 {
						return writeRepoRow(db, fmt.Sprintf("g%d-r%d", g, r))
					}
					if err := db.Exec("CREATE TABLE IF NOT EXISTS items (id INTEGER PRIMARY KEY AUTOINCREMENT, v TEXT)").Error; err != nil {
						return err
					}
					_, err := readRepoRows(db)
					return err
				})
				if err != nil {
					failures <- fmt.Errorf("g%d r%d %s: %w", g, r, path, err)
					return
				}
				if got := cache.Open(); got > max {
					failures <- fmt.Errorf("Open() = %d, want <= %d", got, max)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}

	if peak := tracker.peakOpen(); peak > max {
		t.Fatalf("peak open handles = %d, want <= %d", peak, max)
	}
}

func TestRepoFileCache_AMaximumBelowOneIsRefused(t *testing.T) {
	tracker := newOpenFileTracker()
	for _, max := range []int{0, -1} {
		cache, err := NewRepoFileCache(max, tracker.open, tracker.closeHandle)
		if err == nil {
			t.Fatalf("NewRepoFileCache(%d) succeeded, want an error", max)
		}
		if cache != nil {
			t.Fatalf("NewRepoFileCache(%d) returned a cache alongside its error", max)
		}
	}
}

func TestRepoFileCache_CloseClosesEveryOpenHandle(t *testing.T) {
	cache, tracker := newTrackedRepoFileCache(t, 4)
	for _, path := range repoFilePaths(t, 4) {
		if err := cache.With(path, func(db *gorm.DB) error { return writeRepoRow(db, "x") }); err != nil {
			t.Fatalf("With(%s): %v", path, err)
		}
	}
	if got := tracker.currentlyOpen(); got != 4 {
		t.Fatalf("open handles before Close = %d, want 4", got)
	}

	if err := cache.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := tracker.currentlyOpen(); got != 0 {
		t.Fatalf("open handles after Close = %d, want 0", got)
	}
	if got := cache.Open(); got != 0 {
		t.Fatalf("Open() after Close = %d, want 0", got)
	}
}

func TestRepoFileCache_AnOpenerErrorIsReturnedAndLeaksNoSlot(t *testing.T) {
	cache, tracker := newTrackedRepoFileCache(t, 1)
	paths := repoFilePaths(t, 2)
	broken, healthy := paths[0], paths[1]
	openFailure := errors.New("disk on fire")
	tracker.failOpening(broken, openFailure)

	before := cache.Open()
	called := false
	err := cache.With(broken, func(db *gorm.DB) error {
		called = true
		return nil
	})
	if !errors.Is(err, openFailure) {
		t.Fatalf("With on a failing opener = %v, want it to wrap %v", err, openFailure)
	}
	if called {
		t.Fatalf("use ran although the file never opened")
	}
	if got := cache.Open(); got != before {
		t.Fatalf("Open() = %d after a failed open, want %d", got, before)
	}

	finished := make(chan error, 1)
	go func() {
		finished <- cache.With(healthy, func(db *gorm.DB) error { return writeRepoRow(db, "ok") })
	}()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("With on a healthy file after a failed open: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("a failed open leaked the only slot, the next call is blocked")
	}

	tracker.failOpening(broken, nil)
	if err := cache.With(broken, func(db *gorm.DB) error { return writeRepoRow(db, "recovered") }); err != nil {
		t.Fatalf("With once the opener works again: %v", err)
	}
}
