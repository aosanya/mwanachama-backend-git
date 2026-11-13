package mwanachamagit

import (
	"container/list"
	"errors"
	"fmt"
	"sync"
)

var ErrRepoFileCacheClosed = errors.New("repo file cache is closed")

type repoFileEntry[H any] struct {
	path   string
	handle H
	refs   int
	node   *list.Element
}

type RepoFileCache[H any] struct {
	mu          sync.Mutex
	cond        *sync.Cond
	max         int
	open        func(path string) (H, error)
	closeHandle func(H) error
	entries     map[string]*repoFileEntry[H]
	recency     *list.List
	closed      bool
}

func NewRepoFileCache[H any](max int, open func(path string) (H, error), closeHandle func(H) error) (*RepoFileCache[H], error) {
	if max < 1 {
		return nil, fmt.Errorf("repo file cache maximum must be at least 1, got %d", max)
	}
	c := &RepoFileCache[H]{
		max:         max,
		open:        open,
		closeHandle: closeHandle,
		entries:     map[string]*repoFileEntry[H]{},
		recency:     list.New(),
	}
	c.cond = sync.NewCond(&c.mu)
	return c, nil
}

func (c *RepoFileCache[H]) With(path string, use func(H) error) error {
	entry, err := c.acquire(path)
	if err != nil {
		return err
	}
	defer c.release(entry)
	return use(entry.handle)
}

func (c *RepoFileCache[H]) Open() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

func (c *RepoFileCache[H]) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	c.cond.Broadcast()
	for c.inUse() {
		c.cond.Wait()
	}
	var errs []error
	for path, entry := range c.entries {
		if err := c.closeHandle(entry.handle); err != nil {
			errs = append(errs, err)
		}
		c.recency.Remove(entry.node)
		delete(c.entries, path)
	}
	return errors.Join(errs...)
}

func (c *RepoFileCache[H]) acquire(path string) (*repoFileEntry[H], error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for {
		if c.closed {
			return nil, ErrRepoFileCacheClosed
		}
		if entry, ok := c.entries[path]; ok {
			entry.refs++
			c.recency.MoveToFront(entry.node)
			return entry, nil
		}
		if len(c.entries) >= c.max {
			victim := c.leastRecentlyUsedIdle()
			if victim == nil {
				c.cond.Wait()
				continue
			}
			c.recency.Remove(victim.node)
			delete(c.entries, victim.path)
			if err := c.closeHandle(victim.handle); err != nil {
				c.cond.Broadcast()
				return nil, fmt.Errorf("close %s: %w", victim.path, err)
			}
		}
		handle, err := c.open(path)
		if err != nil {
			c.cond.Broadcast()
			return nil, fmt.Errorf("open %s: %w", path, err)
		}
		entry := &repoFileEntry[H]{path: path, handle: handle, refs: 1}
		entry.node = c.recency.PushFront(entry)
		c.entries[path] = entry
		return entry, nil
	}
}

func (c *RepoFileCache[H]) release(entry *repoFileEntry[H]) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry.refs--
	c.cond.Broadcast()
}

func (c *RepoFileCache[H]) leastRecentlyUsedIdle() *repoFileEntry[H] {
	for node := c.recency.Back(); node != nil; node = node.Prev() {
		entry := node.Value.(*repoFileEntry[H])
		if entry.refs == 0 {
			return entry
		}
	}
	return nil
}

func (c *RepoFileCache[H]) inUse() bool {
	for _, entry := range c.entries {
		if entry.refs > 0 {
			return true
		}
	}
	return false
}
