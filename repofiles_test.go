package mwanachamagit

import (
	"context"
	"errors"
	"math/rand"
	"path/filepath"
	"sync"
	"testing"
)

type repoFilesHarness struct {
	t      *testing.T
	mu     sync.Mutex
	opened []*RepoFile
	files  *RepoFiles
	names  map[string]bool
}

func newRepoFilesHarness(t *testing.T, max int, registeredNames ...string) *repoFilesHarness {
	t.Helper()
	s, err := LoadSpec(filepath.Join("spec", "examples", "engineering.git.json"))
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	h := &repoFilesHarness{t: t, names: map[string]bool{}}
	for _, name := range registeredNames {
		h.names[name] = true
	}
	registered := func(_ context.Context, fileName string) (bool, error) { return h.names[fileName], nil }
	open := func(path string) (*RepoFile, error) {
		rf, err := OpenRepoFile(path, s, nil, nil, nil)
		if err != nil {
			return nil, err
		}
		h.mu.Lock()
		h.opened = append(h.opened, rf)
		h.mu.Unlock()
		return rf, nil
	}
	files, err := NewRepoFiles(t.TempDir(), max, registered, open)
	if err != nil {
		t.Fatalf("NewRepoFiles: %v", err)
	}
	h.files = files
	t.Cleanup(func() { _ = files.Close() })
	return h
}

func (h *repoFilesHarness) usableHandles() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	usable := 0
	for _, rf := range h.opened {
		if rf.sql.Ping() == nil {
			usable++
		}
	}
	return usable
}

func (h *repoFilesHarness) openedCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.opened)
}

func (h *repoFilesHarness) assertWithinMaximum(max int) {
	h.t.Helper()
	if open := h.files.Open(); open > max {
		h.t.Fatalf("Open() = %d, want at most %d", open, max)
	}
	if usable := h.usableHandles(); usable > max {
		h.t.Fatalf("%d file handles still usable, want at most %d", usable, max)
	}
}

func TestRepoFiles_EightRegisteredFilesThroughAMaximumOfThreeKeepTheirOwnData(t *testing.T) {
	ctx := context.Background()
	const max = 3
	fileNames := []string{"a.db", "b.db", "c.db", "d.db", "e.db", "f.db", "g.db", "h.db"}
	h := newRepoFilesHarness(t, max, fileNames...)

	for _, name := range fileNames {
		repoName := name[:1]
		err := h.files.With(ctx, name, func(rf *RepoFile) error {
			_, err := rf.InitRepo(ctx, CreateRepoRequest{Name: repoName})
			return err
		})
		if err != nil {
			t.Fatalf("InitRepo in %s: %v", name, err)
		}
		h.assertWithinMaximum(max)
	}

	checkOwnRepository := func(name string) {
		t.Helper()
		err := h.files.With(ctx, name, func(rf *RepoFile) error {
			repos, err := rf.ListRepositories(ctx)
			if err != nil {
				return err
			}
			if len(repos) != 1 || repos[0].Name != name[:1] {
				t.Fatalf("%s listed %+v, want exactly the repository %q", name, repos, name[:1])
			}
			return nil
		})
		if err != nil {
			t.Fatalf("ListRepositories in %s: %v", name, err)
		}
		h.assertWithinMaximum(max)
	}

	reversed := make([]string, len(fileNames))
	for i, name := range fileNames {
		reversed[len(fileNames)-1-i] = name
	}
	shuffled := append([]string(nil), fileNames...)
	rand.New(rand.NewSource(7)).Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })

	for _, order := range [][]string{fileNames, reversed, shuffled} {
		for _, name := range order {
			checkOwnRepository(name)
		}
	}

	if h.openedCount() <= len(fileNames) {
		t.Fatalf("expected evicted files to be reopened, only %d opens for %d files", h.openedCount(), len(fileNames))
	}
}

func TestRepoFiles_UnregisteredFileIsRefusedAndNothingIsOpened(t *testing.T) {
	ctx := context.Background()
	h := newRepoFilesHarness(t, 3, "a.db")

	called := false
	err := h.files.With(ctx, "stranger.db", func(*RepoFile) error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrRepositoryNotRegistered) {
		t.Fatalf("err = %v, want ErrRepositoryNotRegistered", err)
	}
	if called {
		t.Fatal("use ran for an unregistered file")
	}
	if h.openedCount() != 0 || h.files.Open() != 0 {
		t.Fatalf("opened %d files, cache holds %d, want none", h.openedCount(), h.files.Open())
	}
}

func TestRepoFiles_FileRegisteredThenUnregisteredIsRefusedEvenWhileCached(t *testing.T) {
	ctx := context.Background()
	h := newRepoFilesHarness(t, 3, "a.db")

	if err := h.files.With(ctx, "a.db", func(*RepoFile) error { return nil }); err != nil {
		t.Fatalf("first With: %v", err)
	}
	if h.files.Open() != 1 {
		t.Fatalf("Open() = %d, want 1", h.files.Open())
	}
	h.names["a.db"] = false

	err := h.files.With(ctx, "a.db", func(*RepoFile) error { return nil })
	if !errors.Is(err, ErrRepositoryNotRegistered) {
		t.Fatalf("err = %v, want ErrRepositoryNotRegistered", err)
	}
}

func TestRepoFiles_FileNameWithAPathSeparatorIsRefused(t *testing.T) {
	ctx := context.Background()
	h := newRepoFilesHarness(t, 3, "sub/a.db", "../a.db", `sub\a.db`)

	for _, name := range []string{"sub/a.db", "../a.db", `sub\a.db`} {
		err := h.files.With(ctx, name, func(*RepoFile) error { return nil })
		if err == nil {
			t.Fatalf("%q was accepted", name)
		}
	}
	if h.openedCount() != 0 {
		t.Fatalf("opened %d files, want none", h.openedCount())
	}
}

func TestRepoFiles_MaximumBelowOneIsRefused(t *testing.T) {
	registered := func(context.Context, string) (bool, error) { return true, nil }
	open := func(string) (*RepoFile, error) { return nil, errors.New("unused") }
	if _, err := NewRepoFiles(t.TempDir(), 0, registered, open); err == nil {
		t.Fatal("maximum 0 was accepted")
	}
}

func TestRepoFiles_CloseClosesEveryOpenFile(t *testing.T) {
	ctx := context.Background()
	h := newRepoFilesHarness(t, 3, "a.db", "b.db", "c.db")

	for _, name := range []string{"a.db", "b.db", "c.db"} {
		if err := h.files.With(ctx, name, func(*RepoFile) error { return nil }); err != nil {
			t.Fatalf("With %s: %v", name, err)
		}
	}
	if h.files.Open() != 3 || h.usableHandles() != 3 {
		t.Fatalf("Open() = %d, usable = %d, want 3 and 3", h.files.Open(), h.usableHandles())
	}
	if err := h.files.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if h.files.Open() != 0 {
		t.Fatalf("Open() = %d after Close, want 0", h.files.Open())
	}
	if h.usableHandles() != 0 {
		t.Fatalf("%d handles still usable after Close, want 0", h.usableHandles())
	}
	err := h.files.With(ctx, "a.db", func(*RepoFile) error { return nil })
	if !errors.Is(err, ErrRepoFileCacheClosed) {
		t.Fatalf("err = %v, want ErrRepoFileCacheClosed", err)
	}
}
