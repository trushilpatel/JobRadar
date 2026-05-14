package ratelimit_test

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/trushilpatel/jobradar/platform/ratelimit"
)

func TestScopeFromMethod(t *testing.T) {
	cases := []struct {
		method string
		want   ratelimit.Scope
	}{
		{http.MethodGet, ratelimit.ScopeRead},
		{http.MethodHead, ratelimit.ScopeRead},
		{http.MethodOptions, ratelimit.ScopeRead},
		{http.MethodPost, ratelimit.ScopeWrite},
		{http.MethodPut, ratelimit.ScopeWrite},
		{http.MethodPatch, ratelimit.ScopeWrite},
		{http.MethodDelete, ratelimit.ScopeWrite},
		{"WEIRD", ratelimit.ScopeWrite},
		{"", ratelimit.ScopeWrite},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			if got := ratelimit.ScopeFromMethod(tc.method); got != tc.want {
				t.Fatalf("ScopeFromMethod(%q) = %q, want %q", tc.method, got, tc.want)
			}
		})
	}
}

func TestMemoryLimiter_AllowsThenBlocks(t *testing.T) {
	ctx := context.Background()
	// burst=3 tokens, refill=1 per second.
	l := ratelimit.NewMemoryLimiter(3, 1)

	for i := 0; i < 3; i++ {
		ok, retry, err := l.Allow(ctx, "user-1", ratelimit.ScopeRead)
		if err != nil {
			t.Fatalf("call %d: unexpected err %v", i, err)
		}
		if !ok {
			t.Fatalf("call %d: want allowed", i)
		}
		if retry != 0 {
			t.Fatalf("call %d: want retryAfter=0 on allow, got %v", i, retry)
		}
	}

	// 4th call exhausts the bucket.
	ok, retry, err := l.Allow(ctx, "user-1", ratelimit.ScopeRead)
	if err != nil {
		t.Fatalf("exhaust call: unexpected err %v", err)
	}
	if ok {
		t.Fatal("exhaust call: want denied")
	}
	if retry <= 0 {
		t.Fatalf("exhaust call: want retryAfter > 0, got %v", retry)
	}
	// Refill is 1/s so retryAfter should be roughly 1 second.
	if retry > 2*time.Second {
		t.Fatalf("exhaust call: retryAfter %v unexpectedly large", retry)
	}
}

func TestMemoryLimiter_KeysAreIndependent(t *testing.T) {
	ctx := context.Background()
	l := ratelimit.NewMemoryLimiter(1, 1)

	if ok, _, _ := l.Allow(ctx, "a", ratelimit.ScopeRead); !ok {
		t.Fatal("a: first call should be allowed")
	}
	if ok, _, _ := l.Allow(ctx, "a", ratelimit.ScopeRead); ok {
		t.Fatal("a: second call should be denied (bucket=1)")
	}
	// Different key has its own bucket.
	if ok, _, _ := l.Allow(ctx, "b", ratelimit.ScopeRead); !ok {
		t.Fatal("b: first call should be allowed despite a being exhausted")
	}
}

func TestMemoryLimiter_ScopesAreIndependent(t *testing.T) {
	ctx := context.Background()
	l := ratelimit.NewMemoryLimiter(1, 1)

	if ok, _, _ := l.Allow(ctx, "u", ratelimit.ScopeRead); !ok {
		t.Fatal("read: first call should be allowed")
	}
	if ok, _, _ := l.Allow(ctx, "u", ratelimit.ScopeRead); ok {
		t.Fatal("read: second call should be denied")
	}
	// Same key, different scope ⇒ different bucket.
	if ok, _, _ := l.Allow(ctx, "u", ratelimit.ScopeWrite); !ok {
		t.Fatal("write: first call should be allowed despite read being exhausted")
	}
}

func TestMemoryLimiter_ZeroConfigDeniesAlways(t *testing.T) {
	ctx := context.Background()
	for _, l := range []*ratelimit.MemoryLimiter{
		ratelimit.NewMemoryLimiter(0, 1),
		ratelimit.NewMemoryLimiter(1, 0),
		ratelimit.NewMemoryLimiter(-1, -1),
	} {
		ok, retry, err := l.Allow(ctx, "k", ratelimit.ScopeRead)
		if err != nil {
			t.Fatalf("zero-config: unexpected err %v", err)
		}
		if ok {
			t.Fatal("zero-config: want denied")
		}
		if retry != time.Second {
			t.Fatalf("zero-config: want retryAfter=1s, got %v", retry)
		}
	}
}

// TestMemoryLimiter_NewBucketStartsFull verifies the contract: a fresh
// bucket can absorb a full burst before any refill.
func TestMemoryLimiter_NewBucketStartsFull(t *testing.T) {
	ctx := context.Background()
	l := ratelimit.NewMemoryLimiter(5, 1)
	for i := 0; i < 5; i++ {
		if ok, _, _ := l.Allow(ctx, "fresh", ratelimit.ScopeRead); !ok {
			t.Fatalf("call %d should be allowed on a fresh bucket of size 5", i)
		}
	}
	if ok, _, _ := l.Allow(ctx, "fresh", ratelimit.ScopeRead); ok {
		t.Fatal("6th call should be denied")
	}
}

// TestMemoryLimiter_ConcurrentAllowSameKey runs N parallel Allow calls on
// the same (key, scope) pair and asserts the cumulative allow count never
// exceeds the burst. Race-safety is asserted via -race.
func TestMemoryLimiter_ConcurrentAllowSameKey(t *testing.T) {
	ctx := context.Background()
	const burst = 10
	const callers = 100
	// refill is small enough that no token regenerates within the test window.
	l := ratelimit.NewMemoryLimiter(burst, 0.0001)

	var allowed int64
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			if ok, _, err := l.Allow(ctx, "shared", ratelimit.ScopeRead); err == nil && ok {
				atomic.AddInt64(&allowed, 1)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt64(&allowed); got != burst {
		t.Fatalf("concurrent allow: got %d allowed, want exactly burst=%d", got, burst)
	}
}

// --- Redis-backed tests via miniredis ---

func newRedisLimiter(t *testing.T, burst, refill float64, prefix string) (*ratelimit.RedisLimiter, *miniredis.Miniredis, func()) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	l := ratelimit.NewRedisLimiter(rdb, burst, refill, prefix)
	return l, mr, func() {
		_ = rdb.Close()
		mr.Close()
	}
}

func TestRedisLimiter_DefaultKeyPrefix(t *testing.T) {
	ctx := context.Background()
	_, mr, cleanup := newRedisLimiter(t, 1, 1, "")
	defer cleanup()

	// trigger a write to inspect the namespacing
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = rdb.Close() }()
	l := ratelimit.NewRedisLimiter(rdb, 1, 1, "")
	if ok, _, err := l.Allow(ctx, "u", ratelimit.ScopeRead); err != nil || !ok {
		t.Fatalf("first call: ok=%v err=%v", ok, err)
	}
	keys := mr.Keys()
	found := false
	for _, k := range keys {
		if k == "ratelimit:read:u" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected default-prefixed key 'ratelimit:read:u' in %v", keys)
	}
}

func TestRedisLimiter_CustomKeyPrefix(t *testing.T) {
	ctx := context.Background()
	l, mr, cleanup := newRedisLimiter(t, 1, 1, "myprefix")
	defer cleanup()

	if ok, _, err := l.Allow(ctx, "u", ratelimit.ScopeAdmin); err != nil || !ok {
		t.Fatalf("first call: ok=%v err=%v", ok, err)
	}
	keys := mr.Keys()
	found := false
	for _, k := range keys {
		if k == "myprefix:admin:u" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected custom-prefixed key 'myprefix:admin:u' in %v", keys)
	}
}

func TestRedisLimiter_AllowsThenBlocks(t *testing.T) {
	ctx := context.Background()
	l, _, cleanup := newRedisLimiter(t, 3, 1, "rl")
	defer cleanup()

	for i := 0; i < 3; i++ {
		ok, retry, err := l.Allow(ctx, "u", ratelimit.ScopeRead)
		if err != nil {
			t.Fatalf("call %d: unexpected err %v", i, err)
		}
		if !ok {
			t.Fatalf("call %d: want allowed", i)
		}
		if retry != 0 {
			t.Fatalf("call %d: want retryAfter=0, got %v", i, retry)
		}
	}
	ok, retry, err := l.Allow(ctx, "u", ratelimit.ScopeRead)
	if err != nil {
		t.Fatalf("exhaust call: unexpected err %v", err)
	}
	if ok {
		t.Fatal("exhaust call: want denied")
	}
	if retry <= 0 {
		t.Fatalf("exhaust call: want retryAfter > 0, got %v", retry)
	}
	// refill=1/s so retryAfter should be around 1s and never larger than 2s.
	if retry > 2*time.Second {
		t.Fatalf("exhaust call: retryAfter %v unexpectedly large", retry)
	}
}

func TestRedisLimiter_RefillsAfterTimeElapses(t *testing.T) {
	ctx := context.Background()
	l, mr, cleanup := newRedisLimiter(t, 1, 1, "rl")
	defer cleanup()

	if ok, _, err := l.Allow(ctx, "u", ratelimit.ScopeRead); err != nil || !ok {
		t.Fatalf("first call: ok=%v err=%v", ok, err)
	}
	if ok, _, _ := l.Allow(ctx, "u", ratelimit.ScopeRead); ok {
		t.Fatal("second call: want denied")
	}

	// Fast-forward miniredis time; the limiter's nowMs is host-time-based
	// while the bucket's last_ms is the previously recorded host time. To
	// simulate elapsed time we manually rewrite last_ms in the hash so the
	// next Lua run computes a positive elapsed.
	mr.HSet("rl:read:u", "last_ms", "0")

	if ok, _, err := l.Allow(ctx, "u", ratelimit.ScopeRead); err != nil || !ok {
		t.Fatalf("after refill: ok=%v err=%v", ok, err)
	}
}

func TestRedisLimiter_ZeroConfigDenies(t *testing.T) {
	ctx := context.Background()
	for _, cfg := range []struct{ burst, refill float64 }{
		{0, 1},
		{1, 0},
		{-1, -1},
	} {
		l, _, cleanup := newRedisLimiter(t, cfg.burst, cfg.refill, "rl")
		ok, retry, err := l.Allow(ctx, "u", ratelimit.ScopeRead)
		cleanup()
		if err != nil {
			t.Fatalf("zero-config: unexpected err %v", err)
		}
		if ok {
			t.Fatalf("zero-config (burst=%v refill=%v): want denied", cfg.burst, cfg.refill)
		}
		if retry != time.Second {
			t.Fatalf("zero-config (burst=%v refill=%v): want retry=1s, got %v", cfg.burst, cfg.refill, retry)
		}
	}
}

func TestRedisLimiter_ScopesAreIndependent(t *testing.T) {
	ctx := context.Background()
	l, _, cleanup := newRedisLimiter(t, 1, 1, "rl")
	defer cleanup()

	if ok, _, _ := l.Allow(ctx, "u", ratelimit.ScopeRead); !ok {
		t.Fatal("read: first call should be allowed")
	}
	if ok, _, _ := l.Allow(ctx, "u", ratelimit.ScopeRead); ok {
		t.Fatal("read: second call should be denied")
	}
	if ok, _, _ := l.Allow(ctx, "u", ratelimit.ScopeWrite); !ok {
		t.Fatal("write: first call should be allowed despite read being exhausted")
	}
}

// TestRedisLimiter_TTLSet asserts the Lua script applies an EXPIRE on the
// bucket key so idle entries do not accumulate forever.
func TestRedisLimiter_TTLSet(t *testing.T) {
	ctx := context.Background()
	l, mr, cleanup := newRedisLimiter(t, 2, 1, "rl")
	defer cleanup()

	if ok, _, err := l.Allow(ctx, "u", ratelimit.ScopeRead); err != nil || !ok {
		t.Fatalf("first call: ok=%v err=%v", ok, err)
	}
	ttl := mr.TTL("rl:read:u")
	if ttl <= 0 {
		t.Fatalf("want positive TTL on bucket key, got %v", ttl)
	}
}

// TestRedisLimiter_TransportError closes the client before Allow so the
// Lua run returns an error; the limiter must wrap it.
func TestRedisLimiter_TransportError(t *testing.T) {
	ctx := context.Background()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	_ = rdb.Close() // force every subsequent op to fail

	l := ratelimit.NewRedisLimiter(rdb, 1, 1, "rl")
	ok, retry, err := l.Allow(ctx, "u", ratelimit.ScopeRead)
	if err == nil {
		t.Fatal("want transport error, got nil")
	}
	if ok {
		t.Fatal("transport error: want denied")
	}
	if retry != 0 {
		t.Fatalf("transport error: want retry=0, got %v", retry)
	}
}

// TestRedisLimiter_ContextCancelled — a cancelled context should surface
// the cancellation as an error from the underlying Redis driver.
func TestRedisLimiter_ContextCancelled(t *testing.T) {
	l, _, cleanup := newRedisLimiter(t, 1, 1, "rl")
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := l.Allow(ctx, "u", ratelimit.ScopeRead); err == nil {
		t.Fatal("want error from cancelled context, got nil")
	}
}

// TestRedisLimiter_ConcurrentAllowSameKey asserts the Lua atomicity holds
// under concurrent callers — never more than burst should be allowed.
func TestRedisLimiter_ConcurrentAllowSameKey(t *testing.T) {
	ctx := context.Background()
	const burst = 5
	const callers = 50
	l, _, cleanup := newRedisLimiter(t, burst, 0.0001, "rl")
	defer cleanup()

	var allowed int64
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			if ok, _, err := l.Allow(ctx, "shared", ratelimit.ScopeRead); err == nil && ok {
				atomic.AddInt64(&allowed, 1)
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt64(&allowed); got != burst {
		t.Fatalf("concurrent allow: got %d allowed, want exactly burst=%d", got, burst)
	}
}
