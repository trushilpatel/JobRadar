package cache_test

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/trushilpatel/jobradar/platform/cache"
)

// ---------- Memory cache edge cases ----------

// Set with ttl <= 0 must store the value without expiry. The contract docs
// say "ttl <= 0 means no expiry"; verify both zero and negative TTL.
func TestMemoryCache_SetNoExpiry(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()

	for _, ttl := range []time.Duration{0, -time.Second} {
		key := "k"
		if err := c.Set(ctx, key, sampleValue{Name: "v", Count: 1}, ttl); err != nil {
			t.Fatalf("Set ttl=%v: %v", ttl, err)
		}
		var dest sampleValue
		hit, err := c.Get(ctx, key, &dest)
		if err != nil {
			t.Fatalf("Get ttl=%v: %v", ttl, err)
		}
		if !hit {
			t.Fatalf("ttl=%v: want hit, got miss", ttl)
		}
	}
}

// Set must surface a marshal failure as an error. json.Marshal rejects
// channels, functions, NaN/Inf — use NaN so the public Set signature stays.
func TestMemoryCache_SetMarshalError(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()

	err := c.Set(ctx, "k", math.NaN(), time.Minute)
	if err == nil {
		t.Fatal("Set with NaN: want marshal error, got nil")
	}
	if !strings.Contains(err.Error(), "marshal") {
		t.Fatalf("Set error: want marshal-wrapped, got %v", err)
	}
}

// Get must surface an unmarshal failure (e.g. cached value is an object, dest
// is an int). The first call writes a struct; the second tries to decode it
// into an int* — go's json package fails the decode.
func TestMemoryCache_GetUnmarshalError(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()

	if err := c.Set(ctx, "k", sampleValue{Name: "x"}, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	var dest int
	hit, err := c.Get(ctx, "k", &dest)
	if err == nil {
		t.Fatal("Get with wrong dest: want unmarshal error, got nil")
	}
	if hit {
		t.Fatal("Get on unmarshal failure: want hit=false")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Fatalf("Get error: want unmarshal-wrapped, got %v", err)
	}
}

// Delete with zero keys must succeed (idempotent no-op) and not mutate state.
func TestMemoryCache_DeleteZeroKeys(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()

	if err := c.Set(ctx, "k", sampleValue{Name: "x"}, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := c.Delete(ctx); err != nil {
		t.Fatalf("Delete with no keys: %v", err)
	}

	var dest sampleValue
	hit, _ := c.Get(ctx, "k", &dest)
	if !hit {
		t.Fatal("Delete with zero keys must not evict existing entries")
	}
}

// Delete with many keys must remove all listed entries in one call.
func TestMemoryCache_DeleteManyKeys(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()

	keys := []string{"a", "b", "c", "d"}
	for _, k := range keys {
		if err := c.Set(ctx, k, sampleValue{Name: k}, time.Minute); err != nil {
			t.Fatalf("Set %s: %v", k, err)
		}
	}
	if err := c.Delete(ctx, keys...); err != nil {
		t.Fatalf("Delete many: %v", err)
	}
	for _, k := range keys {
		var dest sampleValue
		if hit, _ := c.Get(ctx, k, &dest); hit {
			t.Fatalf("after Delete many: key %q still present", k)
		}
	}
}

// ---------- GetOrLoad edge cases ----------

// nil cache must skip the fast-path Get, call the loader once, and skip the
// write-back without panicking. This protects callers that pass a nil cache
// when the feature is disabled.
func TestGetOrLoad_NilCachePassThrough(t *testing.T) {
	ctx := context.Background()
	var calls int32

	loader := func(_ context.Context) (sampleValue, error) {
		atomic.AddInt32(&calls, 1)
		return sampleValue{Name: "loaded", Count: 9}, nil
	}

	v, err := cache.GetOrLoad[sampleValue](ctx, nil, "k", time.Minute, loader)
	if err != nil {
		t.Fatalf("GetOrLoad with nil cache: %v", err)
	}
	if v.Name != "loaded" || v.Count != 9 {
		t.Fatalf("GetOrLoad with nil cache: got %+v", v)
	}

	// A second call with nil cache must call the loader again (no cache to
	// hit). Singleflight is per-key; once Do returns, the next call starts
	// a fresh execution.
	if _, err := cache.GetOrLoad[sampleValue](ctx, nil, "k", time.Minute, loader); err != nil {
		t.Fatalf("second GetOrLoad with nil cache: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("nil cache: loader call count want 2, got %d", got)
	}
}

// A ttl <= 0 must still pass through GetOrLoad: loader runs, value is
// returned, and the value is "set" (Memory treats ttl<=0 as no-expiry so the
// next call hits the cache).
func TestGetOrLoad_ZeroTTL(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()

	var calls int32
	loader := func(_ context.Context) (sampleValue, error) {
		atomic.AddInt32(&calls, 1)
		return sampleValue{Name: "z"}, nil
	}

	v, err := cache.GetOrLoad(ctx, c, "k", 0, loader)
	if err != nil {
		t.Fatalf("GetOrLoad ttl=0: %v", err)
	}
	if v.Name != "z" {
		t.Fatalf("GetOrLoad ttl=0: got %+v", v)
	}

	// Memory cache stores ttl<=0 as no-expiry, so the next call must hit.
	if _, err := cache.GetOrLoad(ctx, c, "k", 0, loader); err != nil {
		t.Fatalf("second GetOrLoad ttl=0: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("ttl=0: loader call count want 1 (cached after first), got %d", got)
	}
}

// GetOrLoad must wrap the loader error in a "cache: load %q:" format so
// callers can errors.Is the original. Verify both the wrapping format and
// that errors.Is reaches the original sentinel.
func TestGetOrLoad_LoaderErrorWrapped(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()

	sentinel := errors.New("boom")
	_, err := cache.GetOrLoad(ctx, c, "key-xyz", time.Minute, func(_ context.Context) (sampleValue, error) {
		return sampleValue{}, sentinel
	})
	if err == nil {
		t.Fatal("GetOrLoad: want error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("GetOrLoad: want errors.Is(sentinel), got %v", err)
	}
	if !strings.Contains(err.Error(), `cache: load "key-xyz"`) {
		t.Fatalf("GetOrLoad: error not wrapped with key context, got %v", err)
	}
}

// When the cache reports a transport error on Get, GetOrLoad must treat the
// error as a miss and fall through to the loader rather than failing the
// read path. errorCache always returns an error from Get; the test checks
// the loader still ran exactly once and produced the result.
func TestGetOrLoad_CacheGetErrorFallsThroughToLoader(t *testing.T) {
	ctx := context.Background()
	ec := &errorCache{getErr: errors.New("redis unreachable")}

	var calls int32
	v, err := cache.GetOrLoad(ctx, ec, "k", time.Minute, func(_ context.Context) (sampleValue, error) {
		atomic.AddInt32(&calls, 1)
		return sampleValue{Name: "fallback"}, nil
	})
	if err != nil {
		t.Fatalf("GetOrLoad with broken cache: %v", err)
	}
	if v.Name != "fallback" {
		t.Fatalf("GetOrLoad: got %+v", v)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("broken cache: loader call count want 1, got %d", got)
	}
}

// A Set failure on the write-back must NOT surface to the caller — the loaded
// value still propagates. Use a cache that errors on Set; the test asserts
// the returned value is intact and the loader ran.
func TestGetOrLoad_CacheSetErrorDoesNotFailRead(t *testing.T) {
	ctx := context.Background()
	ec := &errorCache{setErr: errors.New("write-back failed")}

	var calls int32
	v, err := cache.GetOrLoad(ctx, ec, "k", time.Minute, func(_ context.Context) (sampleValue, error) {
		atomic.AddInt32(&calls, 1)
		return sampleValue{Name: "still-returned"}, nil
	})
	if err != nil {
		t.Fatalf("GetOrLoad with set-error cache: %v", err)
	}
	if v.Name != "still-returned" {
		t.Fatalf("GetOrLoad: got %+v", v)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("set-error cache: loader call count want 1, got %d", got)
	}
}

// ---------- Redis cache (miniredis) ----------

func newMiniRedis(t *testing.T) (*miniredis.Miniredis, *goredis.Client) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
		mr.Close()
	})
	return mr, rdb
}

func TestRedisCache_SetGetDelete(t *testing.T) {
	ctx := context.Background()
	_, rdb := newMiniRedis(t)
	c := cache.NewRedisCache(rdb)

	// Miss on empty.
	var dest sampleValue
	hit, err := c.Get(ctx, "k", &dest)
	if err != nil {
		t.Fatalf("Get miss: %v", err)
	}
	if hit {
		t.Fatal("Get miss: want hit=false")
	}

	// Round-trip.
	in := sampleValue{Name: "redis", Count: 42}
	if err := c.Set(ctx, "k", in, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	hit, err = c.Get(ctx, "k", &dest)
	if err != nil {
		t.Fatalf("Get hit: %v", err)
	}
	if !hit || dest != in {
		t.Fatalf("Get hit: hit=%v dest=%+v want %+v", hit, dest, in)
	}

	// Delete one key.
	if err := c.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	dest = sampleValue{}
	hit, _ = c.Get(ctx, "k", &dest)
	if hit {
		t.Fatal("Get after Delete: want miss")
	}
}

func TestRedisCache_DeleteZeroAndManyKeys(t *testing.T) {
	ctx := context.Background()
	_, rdb := newMiniRedis(t)
	c := cache.NewRedisCache(rdb)

	// Zero keys → no-op, returns nil without contacting Redis.
	if err := c.Delete(ctx); err != nil {
		t.Fatalf("Delete zero keys: %v", err)
	}

	// Many keys, mix of existing + missing.
	if err := c.Set(ctx, "a", sampleValue{Name: "a"}, time.Minute); err != nil {
		t.Fatalf("Set a: %v", err)
	}
	if err := c.Set(ctx, "b", sampleValue{Name: "b"}, time.Minute); err != nil {
		t.Fatalf("Set b: %v", err)
	}
	if err := c.Delete(ctx, "a", "b", "missing"); err != nil {
		t.Fatalf("Delete many: %v", err)
	}
	for _, k := range []string{"a", "b"} {
		var dest sampleValue
		hit, _ := c.Get(ctx, k, &dest)
		if hit {
			t.Fatalf("Delete many: key %q still present", k)
		}
	}
}

// TTL=0 in Redis means no expiry (matches the doc contract).
func TestRedisCache_SetNoExpiry(t *testing.T) {
	ctx := context.Background()
	mr, rdb := newMiniRedis(t)
	c := cache.NewRedisCache(rdb)

	if err := c.Set(ctx, "k", sampleValue{Name: "v"}, 0); err != nil {
		t.Fatalf("Set ttl=0: %v", err)
	}
	if ttl := mr.TTL("k"); ttl != 0 {
		t.Fatalf("TTL on key with ttl=0: want 0 (no expiry), got %v", ttl)
	}
}

// TTL>0 must set the key with the correct expiry. Use miniredis FastForward
// to advance virtual time past the TTL and confirm the key is gone.
func TestRedisCache_SetWithTTLExpires(t *testing.T) {
	ctx := context.Background()
	mr, rdb := newMiniRedis(t)
	c := cache.NewRedisCache(rdb)

	if err := c.Set(ctx, "k", sampleValue{Name: "v"}, 100*time.Millisecond); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// Immediate hit.
	var dest sampleValue
	hit, _ := c.Get(ctx, "k", &dest)
	if !hit {
		t.Fatal("immediate Get: want hit")
	}

	// FastForward past the TTL.
	mr.FastForward(200 * time.Millisecond)

	hit, _ = c.Get(ctx, "k", &dest)
	if hit {
		t.Fatal("post-TTL Get: want miss")
	}
}

// Set marshal failure must surface to the caller. NaN is the simplest
// json-unmarshalable value the public API accepts.
func TestRedisCache_SetMarshalError(t *testing.T) {
	ctx := context.Background()
	_, rdb := newMiniRedis(t)
	c := cache.NewRedisCache(rdb)

	if err := c.Set(ctx, "k", math.NaN(), time.Minute); err == nil {
		t.Fatal("Set NaN: want marshal error, got nil")
	}
}

// Stopping miniredis simulates a Redis outage. Each method must wrap the
// transport error with its operation context.
func TestRedisCache_TransportErrors(t *testing.T) {
	ctx := context.Background()
	mr, rdb := newMiniRedis(t)
	c := cache.NewRedisCache(rdb)

	// Prime + then kill miniredis so every operation hits a connection
	// failure.
	if err := c.Set(ctx, "k", sampleValue{Name: "x"}, time.Minute); err != nil {
		t.Fatalf("prime Set: %v", err)
	}
	mr.Close()

	var dest sampleValue
	if _, err := c.Get(ctx, "k", &dest); err == nil {
		t.Fatal("Get after Redis stop: want error, got nil")
	} else if !strings.Contains(err.Error(), "cache redis get") {
		t.Fatalf("Get error: not wrapped, got %v", err)
	}

	if err := c.Set(ctx, "k", sampleValue{Name: "x"}, time.Minute); err == nil {
		t.Fatal("Set after Redis stop: want error, got nil")
	} else if !strings.Contains(err.Error(), "cache redis set") {
		t.Fatalf("Set error: not wrapped, got %v", err)
	}

	if err := c.Delete(ctx, "k"); err == nil {
		t.Fatal("Delete after Redis stop: want error, got nil")
	} else if !strings.Contains(err.Error(), "cache redis del") {
		t.Fatalf("Delete error: not wrapped, got %v", err)
	}
}

// Redis cache must surface unmarshal failures from Get the same way the
// memory cache does — store a real value, then read into an incompatible
// destination type so json.Unmarshal fails.
func TestRedisCache_GetUnmarshalError(t *testing.T) {
	ctx := context.Background()
	_, rdb := newMiniRedis(t)
	c := cache.NewRedisCache(rdb)

	if err := c.Set(ctx, "k", sampleValue{Name: "x"}, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var dest int
	hit, err := c.Get(ctx, "k", &dest)
	if err == nil {
		t.Fatal("Get with wrong dest: want unmarshal error, got nil")
	}
	if hit {
		t.Fatal("Get with unmarshal failure: want hit=false")
	}
}

// NewRedisCache is a tiny constructor — explicit assertion guards against
// nil regression.
func TestNewRedisCache(t *testing.T) {
	_, rdb := newMiniRedis(t)
	c := cache.NewRedisCache(rdb)
	if c == nil {
		t.Fatal("NewRedisCache returned nil")
	}
}

// ---------- Noop edge cases ----------

// Variadic Delete with no keys must still succeed.
func TestNoopCache_DeleteZeroKeys(t *testing.T) {
	if err := cache.NewNoopCache().Delete(context.Background()); err != nil {
		t.Fatalf("Noop Delete zero keys: %v", err)
	}
}

// Variadic Delete with many keys must succeed without panic.
func TestNoopCache_DeleteManyKeys(t *testing.T) {
	if err := cache.NewNoopCache().Delete(context.Background(), "a", "b", "c"); err != nil {
		t.Fatalf("Noop Delete many: %v", err)
	}
}

// ---------- Concurrency under -race ----------

// Memory cache must allow interleaved Get/Set/Delete from many goroutines
// without race or panic. Single asserted invariant: no panic, no race.
func TestMemoryCache_ConcurrentReadWriteDelete(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()

	const goroutines = 16
	const iterations = 200

	var wg sync.WaitGroup
	wg.Add(goroutines * 3)

	// Writers.
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = c.Set(ctx, "k", sampleValue{Count: i*1000 + j}, time.Minute)
			}
		}(i)
	}
	// Readers.
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				var dest sampleValue
				_, _ = c.Get(ctx, "k", &dest)
			}
		}()
	}
	// Deleters.
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = c.Delete(ctx, "k")
			}
		}()
	}
	wg.Wait()
}

// ---------- helpers ----------

// errorCache is a hand-rolled cache.Cache that returns the configured error
// from each method. It is used to exercise GetOrLoad's defensive paths
// without standing up a real Redis instance.
type errorCache struct {
	getErr error
	setErr error
	delErr error
}

func (e *errorCache) Get(_ context.Context, _ string, _ any) (bool, error) {
	if e.getErr != nil {
		return false, e.getErr
	}
	return false, nil
}

func (e *errorCache) Set(_ context.Context, _ string, _ any, _ time.Duration) error {
	return e.setErr
}

func (e *errorCache) Delete(_ context.Context, _ ...string) error {
	return e.delErr
}
