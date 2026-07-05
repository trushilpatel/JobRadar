package objectstorage

import (
	"bytes"
	"context"
	"io"
	"sync"
)

// MemoryStorage is an in-process Storage implementation for tests. Objects
// live in a map for the process lifetime — no network call is ever made.
type MemoryStorage struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{objects: make(map[string][]byte)}
}

func (m *MemoryStorage) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = data
	return nil
}

func (m *MemoryStorage) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	return nil
}

func (m *MemoryStorage) Get(_ context.Context, key string) (io.ReadCloser, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.objects[key]
	if !ok {
		return nil, 0, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
}

// Has reports whether key currently exists. Test-only helper, not part of Storage.
func (m *MemoryStorage) Has(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.objects[key]
	return ok
}

var _ Storage = (*MemoryStorage)(nil)
