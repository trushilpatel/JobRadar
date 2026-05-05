package cache

import (
	"context"
	"sync"
	"time"
)

// MemoryCache is a thread-safe in-process Cache backed by a map. Used in
// tests and as a fallback when Redis is unavailable. Values are JSON-encoded
// so behaviour matches the Redis implementation byte-for-byte.
type MemoryCache struct {
	mu    sync.Mutex
	items map[string]memoryItem
	clock func() time.Time
}

type memoryItem struct {
	data      []byte
	expiresAt time.Time // zero ⇒ never expires
}

// NewMemoryCache returns an empty in-process cache.
func NewMemoryCache() *MemoryCache {
	return &MemoryCache{
		items: make(map[string]memoryItem),
		clock: time.Now,
	}
}

// Get returns false on miss or expired entry. Expired entries are evicted
// lazily — there is no background sweeper because in tests TTL accuracy
// matters more than memory pressure.
func (c *MemoryCache) Get(_ context.Context, key string, dest any) (bool, error) {
	c.mu.Lock()
	item, ok := c.items[key]
	if ok && !item.expiresAt.IsZero() && c.clock().After(item.expiresAt) {
		delete(c.items, key)
		ok = false
	}
	c.mu.Unlock()
	if !ok {
		return false, nil
	}
	if err := unmarshal(item.data, dest); err != nil {
		return false, err
	}
	return true, nil
}

// Set stores the JSON encoding of val under key. ttl <= 0 means "no
// expiry"; callers wanting an explicit "do not cache" should skip Set.
func (c *MemoryCache) Set(_ context.Context, key string, val any, ttl time.Duration) error {
	data, err := marshal(val)
	if err != nil {
		return err
	}
	var expires time.Time
	if ttl > 0 {
		expires = c.clock().Add(ttl)
	}
	c.mu.Lock()
	c.items[key] = memoryItem{data: data, expiresAt: expires}
	c.mu.Unlock()
	return nil
}

// Delete removes any of the listed keys present in the cache. Missing keys
// are silently ignored — Delete is idempotent.
func (c *MemoryCache) Delete(_ context.Context, keys ...string) error {
	c.mu.Lock()
	for _, k := range keys {
		delete(c.items, k)
	}
	c.mu.Unlock()
	return nil
}
