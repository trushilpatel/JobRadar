package cache

import (
	"context"
	"time"
)

// NoopCache is a Cache that never stores anything. Get always misses;
// Set and Delete are no-ops. Wire this in when caching needs to be
// disabled without changing call sites — for instance in single-replica
// debug builds where staleness would obscure live behaviour.
type NoopCache struct{}

// NewNoopCache returns a cache that holds nothing.
func NewNoopCache() *NoopCache { return &NoopCache{} }

// Get always reports a clean miss.
func (NoopCache) Get(_ context.Context, _ string, _ any) (bool, error) {
	return false, nil
}

// Set is a no-op. Returns nil so callers cannot distinguish it from a
// successful write — important so cache-aside code paths remain identical
// regardless of which Cache implementation is wired in.
func (NoopCache) Set(_ context.Context, _ string, _ any, _ time.Duration) error {
	return nil
}

// Delete is a no-op.
func (NoopCache) Delete(_ context.Context, _ ...string) error { return nil }
