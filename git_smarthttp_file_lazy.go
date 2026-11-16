package mwanachamagit

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	gogitplumbing "github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
)

const smallObjectCacheBytes = 8 << 20

var errStoredObjectCorrupt = errors.New("stored object does not match its hash")

type lazyStorage struct {
	*memory.Storage
	m     *gitManager
	ctx   context.Context
	mu    sync.Mutex
	small map[gogitplumbing.Hash]*gogitplumbing.MemoryObject
	held  int
}

func newLazyStorage(ctx context.Context, m *gitManager) *lazyStorage {
	return &lazyStorage{
		Storage: memory.NewStorage(),
		m:       m,
		ctx:     ctx,
		small:   map[gogitplumbing.Hash]*gogitplumbing.MemoryObject{},
	}
}

func (s *lazyStorage) EncodedObject(kind gogitplumbing.ObjectType, hash gogitplumbing.Hash) (gogitplumbing.EncodedObject, error) {
	if obj, err := s.Storage.EncodedObject(kind, hash); err == nil {
		return obj, nil
	} else if !errors.Is(err, gogitplumbing.ErrObjectNotFound) {
		return nil, err
	}
	return s.stored(kind, hash)
}

func (s *lazyStorage) HasEncodedObject(hash gogitplumbing.Hash) error {
	if s.Storage.HasEncodedObject(hash) == nil {
		return nil
	}
	_, err := s.stored(gogitplumbing.AnyObject, hash)
	return err
}

func (s *lazyStorage) EncodedObjectSize(hash gogitplumbing.Hash) (int64, error) {
	if size, err := s.Storage.EncodedObjectSize(hash); err == nil {
		return size, nil
	}
	obj, err := s.stored(gogitplumbing.AnyObject, hash)
	if err != nil {
		return 0, err
	}
	return obj.Size(), nil
}

func (s *lazyStorage) stored(kind gogitplumbing.ObjectType, hash gogitplumbing.Hash) (gogitplumbing.EncodedObject, error) {
	if kind != gogitplumbing.AnyObject && kind != gogitplumbing.CommitObject && kind != gogitplumbing.TreeObject && kind != gogitplumbing.BlobObject {
		return nil, gogitplumbing.ErrObjectNotFound
	}
	if kind == gogitplumbing.AnyObject || kind == gogitplumbing.CommitObject {
		if obj, err := s.loadSmall(gogitplumbing.CommitObject, s.m.tables.Commits, hash); !errors.Is(err, gogitplumbing.ErrObjectNotFound) {
			return obj, err
		}
	}
	if kind == gogitplumbing.AnyObject || kind == gogitplumbing.TreeObject {
		if obj, err := s.loadSmall(gogitplumbing.TreeObject, s.m.tables.Trees, hash); !errors.Is(err, gogitplumbing.ErrObjectNotFound) {
			return obj, err
		}
	}
	if kind == gogitplumbing.AnyObject || kind == gogitplumbing.BlobObject {
		return s.blobOnDemand(hash)
	}
	return nil, gogitplumbing.ErrObjectNotFound
}

func (s *lazyStorage) loadSmall(kind gogitplumbing.ObjectType, table string, hash gogitplumbing.Hash) (gogitplumbing.EncodedObject, error) {
	s.mu.Lock()
	cached, ok := s.small[hash]
	s.mu.Unlock()
	if ok && cached.Type() == kind {
		return cached, nil
	}
	var rows []string
	err := s.m.db.WithContext(s.ctx).Table(table).
		Where("sha = ? AND data <> ''", hash.String()).Limit(1).Pluck("data", &rows).Error
	if err != nil {
		return nil, fmt.Errorf("read %s %s: %w", kind, shortSHA(hash.String()), err)
	}
	if len(rows) == 0 {
		return nil, gogitplumbing.ErrObjectNotFound
	}
	raw, err := base64.StdEncoding.DecodeString(rows[0])
	if err != nil || gogitplumbing.ComputeHash(kind, raw) != hash {
		return nil, gogitplumbing.ErrObjectNotFound
	}
	obj := &gogitplumbing.MemoryObject{}
	obj.SetType(kind)
	_, _ = obj.Write(raw)
	s.remember(hash, obj, len(raw))
	return obj, nil
}

func (s *lazyStorage) remember(hash gogitplumbing.Hash, obj *gogitplumbing.MemoryObject, size int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held+size > smallObjectCacheBytes {
		s.small = map[gogitplumbing.Hash]*gogitplumbing.MemoryObject{}
		s.held = 0
	}
	s.small[hash] = obj
	s.held += size
}

func (s *lazyStorage) blobOnDemand(hash gogitplumbing.Hash) (gogitplumbing.EncodedObject, error) {
	var rows []struct {
		N    int64
		Tail string
	}
	err := s.m.db.WithContext(s.ctx).Table(s.m.tables.Blobs).
		Select("length(data) AS n, substr(data, -2) AS tail").
		Where("sha = ? AND (data <> '' OR size = 0)", hash.String()).Limit(1).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("read blob %s: %w", shortSHA(hash.String()), err)
	}
	if len(rows) == 0 {
		return nil, gogitplumbing.ErrObjectNotFound
	}
	size, ok := decodedBase64Size(rows[0].N, rows[0].Tail)
	if !ok {
		return nil, gogitplumbing.ErrObjectNotFound
	}
	if size == 0 && hash != gogitplumbing.ComputeHash(gogitplumbing.BlobObject, nil) {
		return nil, gogitplumbing.ErrObjectNotFound
	}
	return &lazyBlob{storage: s, hash: hash, size: size}, nil
}

func decodedBase64Size(encodedLength int64, tail string) (int64, bool) {
	if encodedLength%4 != 0 {
		return 0, false
	}
	padding := int64(strings.Count(tail, "="))
	return encodedLength/4*3 - padding, true
}

type lazyBlob struct {
	storage *lazyStorage
	hash    gogitplumbing.Hash
	size    int64
}

func (b *lazyBlob) Hash() gogitplumbing.Hash         { return b.hash }
func (b *lazyBlob) Type() gogitplumbing.ObjectType   { return gogitplumbing.BlobObject }
func (b *lazyBlob) SetType(gogitplumbing.ObjectType) {}
func (b *lazyBlob) Size() int64                      { return b.size }
func (b *lazyBlob) SetSize(int64)                    {}
func (b *lazyBlob) Writer() (io.WriteCloser, error) {
	return nil, errors.New("stored blob is read only")
}
func (b *lazyBlob) Reader() (io.ReadCloser, error) {
	var rows []string
	err := b.storage.m.db.WithContext(b.storage.ctx).Table(b.storage.m.tables.Blobs).
		Where("sha = ?", b.hash.String()).Limit(1).Pluck("data", &rows).Error
	if err != nil {
		return nil, fmt.Errorf("read blob %s: %w", shortSHA(b.hash.String()), err)
	}
	if len(rows) == 0 {
		return nil, gogitplumbing.ErrObjectNotFound
	}
	decoded := base64.NewDecoder(base64.StdEncoding, strings.NewReader(rows[0]))
	return &verifiedBlobReader{
		source: decoded,
		hasher: gogitplumbing.NewHasher(gogitplumbing.BlobObject, b.size),
		want:   b.hash,
		size:   b.size,
	}, nil
}

type verifiedBlobReader struct {
	source io.Reader
	hasher gogitplumbing.Hasher
	want   gogitplumbing.Hash
	size   int64
	read   int64
}

func (v *verifiedBlobReader) Read(p []byte) (int, error) {
	n, err := v.source.Read(p)
	_, _ = v.hasher.Write(p[:n])
	v.read += int64(n)
	if errors.Is(err, io.EOF) && (v.read != v.size || v.hasher.Sum() != v.want) {
		return n, fmt.Errorf("blob %s: %w", shortSHA(v.want.String()), errStoredObjectCorrupt)
	}
	return n, err
}

func (v *verifiedBlobReader) Close() error { return nil }
