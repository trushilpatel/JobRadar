package cache

import (
	"container/list"
	"context"
	"sync"
	"time"
)

// LocalCache is a process-local read-through wrapper around another Cache.
// It absorbs hot reads (e.g. the per-request identity bundle resolved by the
// auth middleware) so Upstash is only consulted on the first miss or after
// the local TTL expires.
//
// All mutating operations (Set, Delete) flow through to the wrapped cache so
// invalidations performed on the same instance — including invalidations
// triggered by the user service when a profile or role changes — clear the
// local layer in-process before they reach Redis.
//
// Cross-replica invalidation still relies on TTL expiry: a write performed
// on replica A only clears its own local layer. Other replicas observe the
// stale value until their local TTL elapses, which is the same trade-off the
// upstream Redis TTL already accepts.
type LocalCache struct {
	next    Cache
	maxTTL  time.Duration
	maxSize int

	mu    sync.Mutex
	items map[string]*list.Element
	order *list.List
	clock func() time.Time
}

// localEntry is what we store in the LRU list. The data is the same JSON
// blob the wrapped Cache would return, so we never marshal twice.
type localEntry struct {
	key       string
	data      []byte
	expiresAt time.Time
}

// NewLocalCache wraps next with a bounded in-process LRU+TTL layer. A
// non-positive maxTTL or maxSize disables the local layer entirely — the
// returned Cache is equivalent to next.
//
// maxTTL caps how long a value lives locally regardless of what TTL the
// caller passes to Set; this lets the local layer bound staleness without
// touching call-site TTLs.
func NewLocalCache(next Cache, maxTTL time.Duration, maxSize int) Cache {
	if next == nil {
		panic("cache.NewLocalCache: next must not be nil")
	}
	if maxTTL <= 0 || maxSize <= 0 {
		return next
	}
	return &LocalCache{
		next:    next,
		maxTTL:  maxTTL,
		maxSize: maxSize,
		items:   make(map[string]*list.Element, maxSize),
		order:   list.New(),
		clock:   time.Now,
	}
}

// Get returns the locally cached value when present and unexpired, otherwise
// falls through to the wrapped cache and populates the local layer on hit.
func (l *LocalCache) Get(ctx context.Context, key string, dest any) (bool, error) {
	if data, ok := l.takeFresh(key); ok {
		if err := unmarshal(data, dest); err == nil {
			return true, nil
		}
		// Corrupt local entry — drop it and fall through so the source of
		// truth has a chance to repopulate.
		l.evict(key)
	}

	hit, err := l.next.Get(ctx, key, dest)
	if err != nil || !hit {
		return hit, err
	}
	if data, mErr := marshal(dest); mErr == nil {
		l.put(key, data)
	}
	return true, nil
}

// Set writes through to the wrapped cache and mirrors the value locally with
// min(ttl, maxTTL) so the local layer never serves data past the upstream
// expiry the caller requested.
func (l *LocalCache) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	if data, err := marshal(val); err == nil {
		localTTL := ttl
		if localTTL <= 0 || localTTL > l.maxTTL {
			localTTL = l.maxTTL
		}
		l.putWithTTL(key, data, localTTL)
	}
	return l.next.Set(ctx, key, val, ttl)
}

// Delete clears the keys locally and from the wrapped cache. The local clear
// runs first so a concurrent Get on this process cannot observe the stale
// value between the upstream delete and the local eviction.
func (l *LocalCache) Delete(ctx context.Context, keys ...string) error {
	l.mu.Lock()
	for _, k := range keys {
		if el, ok := l.items[k]; ok {
			l.order.Remove(el)
			delete(l.items, k)
		}
	}
	l.mu.Unlock()
	return l.next.Delete(ctx, keys...)
}

// takeFresh returns the cached blob for key when present and unexpired. It
// also promotes the entry to the front of the LRU list so eviction targets
// the least-recently-used keys.
func (l *LocalCache) takeFresh(key string) ([]byte, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	el, ok := l.items[key]
	if !ok {
		return nil, false
	}
	entry, ok := el.Value.(*localEntry)
	if !ok {
		l.order.Remove(el)
		delete(l.items, key)
		return nil, false
	}
	if l.clock().After(entry.expiresAt) {
		l.order.Remove(el)
		delete(l.items, key)
		return nil, false
	}
	l.order.MoveToFront(el)
	return entry.data, true
}

func (l *LocalCache) put(key string, data []byte) {
	l.putWithTTL(key, data, l.maxTTL)
}

func (l *LocalCache) putWithTTL(key string, data []byte, ttl time.Duration) {
	expires := l.clock().Add(ttl)
	l.mu.Lock()
	defer l.mu.Unlock()
	if el, ok := l.items[key]; ok {
		if entry, ok := el.Value.(*localEntry); ok {
			entry.data = data
			entry.expiresAt = expires
			l.order.MoveToFront(el)
			return
		}
	}
	entry := &localEntry{key: key, data: data, expiresAt: expires}
	el := l.order.PushFront(entry)
	l.items[key] = el
	for l.order.Len() > l.maxSize {
		oldest := l.order.Back()
		if oldest == nil {
			break
		}
		l.order.Remove(oldest)
		if oldEntry, ok := oldest.Value.(*localEntry); ok {
			delete(l.items, oldEntry.key)
		}
	}
}

func (l *LocalCache) evict(key string) {
	l.mu.Lock()
	if el, ok := l.items[key]; ok {
		l.order.Remove(el)
		delete(l.items, key)
	}
	l.mu.Unlock()
}
