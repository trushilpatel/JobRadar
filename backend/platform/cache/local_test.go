package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingCache wraps another Cache and counts the calls that reach it. Used
// to assert that the local layer is actually serving hits from memory and not
// forwarding every read down the chain.
type countingCache struct {
	inner    Cache
	gets     atomic.Int64
	sets     atomic.Int64
	deletes  atomic.Int64
	getErr   error
	setErr   error
	delErr   error
	onceErr  error
	onceOnly atomic.Bool
}

func newCountingCache(inner Cache) *countingCache { return &countingCache{inner: inner} }

func (c *countingCache) Get(ctx context.Context, key string, dest any) (bool, error) {
	c.gets.Add(1)
	if c.onceErr != nil && !c.onceOnly.Swap(true) {
		return false, c.onceErr
	}
	if c.getErr != nil {
		return false, c.getErr
	}
	return c.inner.Get(ctx, key, dest)
}

func (c *countingCache) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	c.sets.Add(1)
	if c.setErr != nil {
		return c.setErr
	}
	return c.inner.Set(ctx, key, val, ttl)
}

func (c *countingCache) Delete(ctx context.Context, keys ...string) error {
	c.deletes.Add(1)
	if c.delErr != nil {
		return c.delErr
	}
	return c.inner.Delete(ctx, keys...)
}

func TestLocalCache_PassThroughWhenDisabled(t *testing.T) {
	mem := NewMemoryCache()
	if got := NewLocalCache(mem, 0, 100); got != Cache(mem) {
		t.Fatalf("zero TTL must return the wrapped cache unchanged, got %T", got)
	}
	if got := NewLocalCache(mem, time.Second, 0); got != Cache(mem) {
		t.Fatalf("zero size must return the wrapped cache unchanged, got %T", got)
	}
}

func TestLocalCache_NilNextPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil next")
		}
	}()
	NewLocalCache(nil, time.Second, 1)
}

func TestLocalCache_HitAvoidsWrappedGet(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryCache()
	counting := newCountingCache(mem)
	c := NewLocalCache(counting, time.Minute, 10)

	if err := c.Set(ctx, "k", "v", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	if counting.sets.Load() != 1 {
		t.Fatalf("set must reach wrapped cache, got %d", counting.sets.Load())
	}

	for i := range 5 {
		var out string
		hit, err := c.Get(ctx, "k", &out)
		if err != nil || !hit || out != "v" {
			t.Fatalf("get %d: hit=%v err=%v out=%q", i, hit, err, out)
		}
	}
	if counting.gets.Load() != 0 {
		t.Fatalf("local hits must not forward Get, forwarded %d", counting.gets.Load())
	}
}

func TestLocalCache_MissPopulatesLocal(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryCache()
	if err := mem.Set(ctx, "k", "v", time.Minute); err != nil {
		t.Fatalf("seed: %v", err)
	}
	counting := newCountingCache(mem)
	c := NewLocalCache(counting, time.Minute, 10)

	var out string
	hit, err := c.Get(ctx, "k", &out)
	if err != nil || !hit || out != "v" {
		t.Fatalf("first get: hit=%v err=%v out=%q", hit, err, out)
	}
	if counting.gets.Load() != 1 {
		t.Fatalf("first get must reach wrapped cache once, got %d", counting.gets.Load())
	}

	for i := range 3 {
		var out string
		hit, err := c.Get(ctx, "k", &out)
		if err != nil || !hit || out != "v" {
			t.Fatalf("subsequent get %d: hit=%v err=%v out=%q", i, hit, err, out)
		}
	}
	if counting.gets.Load() != 1 {
		t.Fatalf("subsequent gets must be served locally, forwarded %d", counting.gets.Load())
	}
}

func TestLocalCache_TTLExpiryFallsThrough(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryCache()
	counting := newCountingCache(mem)
	c, ok := NewLocalCache(counting, 50*time.Millisecond, 10).(*LocalCache)
	if !ok {
		t.Fatal("local cache must be a *LocalCache when TTL+size are positive")
	}

	// Inject a deterministic clock so the test does not race real time.
	now := time.Now()
	c.clock = func() time.Time { return now }

	if err := c.Set(ctx, "k", "v", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	var out string
	if _, err := c.Get(ctx, "k", &out); err != nil {
		t.Fatalf("warm: %v", err)
	}

	now = now.Add(200 * time.Millisecond)
	beforeGets := counting.gets.Load()
	if _, err := c.Get(ctx, "k", &out); err != nil {
		t.Fatalf("post-expiry get: %v", err)
	}
	if counting.gets.Load() != beforeGets+1 {
		t.Fatalf("expired entry must forward to wrapped cache, gets=%d", counting.gets.Load())
	}
}

func TestLocalCache_SetCapsLocalTTLAtMax(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryCache()
	counting := newCountingCache(mem)
	c, ok := NewLocalCache(counting, 10*time.Millisecond, 10).(*LocalCache)
	if !ok {
		t.Fatal("local cache must be a *LocalCache when TTL+size are positive")
	}

	now := time.Now()
	c.clock = func() time.Time { return now }

	// Caller asks for a 1-hour TTL — local layer must cap at 10ms so a stale
	// reader cannot keep using the local copy past its cap even though Redis
	// would still serve it.
	if err := c.Set(ctx, "k", "v", time.Hour); err != nil {
		t.Fatalf("set: %v", err)
	}
	now = now.Add(50 * time.Millisecond)
	var out string
	beforeGets := counting.gets.Load()
	if _, err := c.Get(ctx, "k", &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if counting.gets.Load() != beforeGets+1 {
		t.Fatalf("local entry must expire at maxTTL; got %d wrapped gets", counting.gets.Load())
	}
}

func TestLocalCache_DeleteClearsLocalAndForwards(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryCache()
	counting := newCountingCache(mem)
	c := NewLocalCache(counting, time.Minute, 10)

	if err := c.Set(ctx, "k", "v", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	// Warm the local layer.
	var out string
	if _, err := c.Get(ctx, "k", &out); err != nil {
		t.Fatalf("warm: %v", err)
	}
	if err := c.Delete(ctx, "k"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if counting.deletes.Load() != 1 {
		t.Fatalf("delete must forward, got %d", counting.deletes.Load())
	}
	// Next read should miss locally and reach the wrapped cache.
	beforeGets := counting.gets.Load()
	hit, err := c.Get(ctx, "k", &out)
	if err != nil || hit {
		t.Fatalf("post-delete get: hit=%v err=%v", hit, err)
	}
	if counting.gets.Load() != beforeGets+1 {
		t.Fatalf("post-delete get must forward, forwarded %d", counting.gets.Load()-beforeGets)
	}
}

func TestLocalCache_LRUEvictsOldest(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryCache()
	c, ok := NewLocalCache(mem, time.Minute, 2).(*LocalCache)
	if !ok {
		t.Fatal("local cache must be a *LocalCache when TTL+size are positive")
	}

	if err := c.Set(ctx, "a", "1", time.Minute); err != nil {
		t.Fatalf("set a: %v", err)
	}
	if err := c.Set(ctx, "b", "2", time.Minute); err != nil {
		t.Fatalf("set b: %v", err)
	}
	// Touch "a" so "b" becomes the LRU candidate.
	var out string
	if _, err := c.Get(ctx, "a", &out); err != nil {
		t.Fatalf("get a: %v", err)
	}
	if err := c.Set(ctx, "c", "3", time.Minute); err != nil {
		t.Fatalf("set c: %v", err)
	}

	c.mu.Lock()
	_, hasB := c.items["b"]
	hasA := c.items["a"] != nil
	hasC := c.items["c"] != nil
	c.mu.Unlock()
	if hasB || !hasA || !hasC {
		t.Fatalf("expected eviction of 'b'; got items=a:%v b:%v c:%v", hasA, hasB, hasC)
	}
}

func TestLocalCache_GetErrorFromWrappedSurfaces(t *testing.T) {
	ctx := context.Background()
	counting := newCountingCache(NewMemoryCache())
	counting.getErr = errors.New("redis down")
	c := NewLocalCache(counting, time.Minute, 4)
	var out string
	hit, err := c.Get(ctx, "k", &out)
	if hit {
		t.Fatal("hit must be false on transport error")
	}
	if err == nil {
		t.Fatal("transport error must surface to caller")
	}
}

func TestLocalCache_SetErrorFromWrappedSurfaces(t *testing.T) {
	ctx := context.Background()
	counting := newCountingCache(NewMemoryCache())
	counting.setErr = errors.New("redis down")
	c := NewLocalCache(counting, time.Minute, 4)
	if err := c.Set(ctx, "k", "v", time.Minute); err == nil {
		t.Fatal("Set must surface wrapped error")
	}
}

func TestLocalCache_ConcurrentReadsWriteOnce(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryCache()
	if err := mem.Set(ctx, "k", "v", time.Minute); err != nil {
		t.Fatalf("seed: %v", err)
	}
	counting := newCountingCache(mem)
	c := NewLocalCache(counting, time.Minute, 10)

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			var out string
			if hit, err := c.Get(ctx, "k", &out); err != nil || !hit || out != "v" {
				t.Errorf("concurrent get: hit=%v err=%v out=%q", hit, err, out)
			}
		})
	}
	wg.Wait()
	// At least one Get reaches the wrapped cache to populate, but the rest
	// should be served locally. The exact count depends on goroutine
	// scheduling; we only enforce that it is not 50 (i.e., the local layer
	// is actually doing work).
	if counting.gets.Load() == 50 {
		t.Fatalf("local layer did not absorb any concurrent reads")
	}
}

func TestLocalCache_DeleteIgnoresMissingKeys(t *testing.T) {
	ctx := context.Background()
	c := NewLocalCache(NewMemoryCache(), time.Minute, 4)
	if err := c.Delete(ctx, "nope-1", "nope-2"); err != nil {
		t.Fatalf("delete of missing keys must be a no-op, got %v", err)
	}
}

func TestLocalCache_OverwriteUpdatesEntry(t *testing.T) {
	ctx := context.Background()
	c := NewLocalCache(NewMemoryCache(), time.Minute, 4)
	if err := c.Set(ctx, "k", "v1", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := c.Set(ctx, "k", "v2", time.Minute); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	var out string
	hit, err := c.Get(ctx, "k", &out)
	if err != nil || !hit || out != "v2" {
		t.Fatalf("after overwrite: hit=%v err=%v out=%q", hit, err, out)
	}
}

// TestLocalCache_NestedStructRoundTrips guards against a subtle bug where the
// local layer holds a different JSON encoding than the wrapped cache would
// have produced — keeping marshal/unmarshal symmetrical matters because the
// identity bundle is a struct, not a scalar.
func TestLocalCache_NestedStructRoundTrips(t *testing.T) {
	ctx := context.Background()
	c := NewLocalCache(NewMemoryCache(), time.Minute, 4)
	type bundle struct {
		ID    string   `json:"id"`
		Roles []string `json:"roles"`
	}
	in := bundle{ID: "u1", Roles: []string{"user", "admin"}}
	if err := c.Set(ctx, "k", in, time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	var out bundle
	hit, err := c.Get(ctx, "k", &out)
	if err != nil || !hit {
		t.Fatalf("get: hit=%v err=%v", hit, err)
	}
	if out.ID != in.ID || fmt.Sprint(out.Roles) != fmt.Sprint(in.Roles) {
		t.Fatalf("round-trip mismatch: in=%+v out=%+v", in, out)
	}
}
