package ratelimit

// ratelimit_internal_test.go — white-box tests for unexported helpers and
// defensive branches that cannot be reached through the public Limiter API:
//
//  1. minFloat: the "return a" branch (cap case — when a < b)
//  2. MemoryLimiter.Allow: retryAfter <= 0 floor guard
//  3. toInt64: the int and string branches
//  4. RedisLimiter.Allow: unexpected reply branch (!ok || len(values) != 2)

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

// ---------- minFloat ----------

func TestMinFloat_ReturnsBWhenAGreaterOrEqual(t *testing.T) {
	// a >= b → returns b
	if got := minFloat(5.0, 3.0); got != 3.0 {
		t.Fatalf("minFloat(5.0, 3.0): want 3.0, got %v", got)
	}
	if got := minFloat(3.0, 3.0); got != 3.0 {
		t.Fatalf("minFloat(3.0, 3.0): want 3.0, got %v", got)
	}
}

func TestMinFloat_ReturnsAWhenALessThanB(t *testing.T) {
	// a < b → returns a (cap case: burst is the cap)
	if got := minFloat(1.0, 10.0); got != 1.0 {
		t.Fatalf("minFloat(1.0, 10.0): want 1.0 (cap), got %v", got)
	}
}

// ---------- MemoryLimiter.Allow — refill caps at burst ----------

// TestMemoryLimiter_RefillCapsAtBurst exercises the minFloat "return a" branch
// by injecting a fake clock that jumps far forward so refill would overshoot
// the burst. The bucket must be capped at burst, not at burst + overshooted.
func TestMemoryLimiter_RefillCapsAtBurst(t *testing.T) {
	ctx := context.Background()

	// burst=1, refill=100/s. Drain the bucket, then jump 10 seconds ahead so
	// the refill would add 1000 tokens — far more than burst. The bucket must
	// clamp back to exactly 1.
	now := time.Now()
	l := NewMemoryLimiter(1, 100)
	l.clock = func() time.Time { return now }

	// Drain the sole token.
	if ok, _, err := l.Allow(ctx, "k", ScopeRead); !ok || err != nil {
		t.Fatalf("first call: want allowed; ok=%v err=%v", ok, err)
	}
	if ok, _, _ := l.Allow(ctx, "k", ScopeRead); ok {
		t.Fatal("second call: want denied (bucket drained)")
	}

	// Advance clock by 10 seconds — would add 1000 tokens without the cap.
	now = now.Add(10 * time.Second)

	// Token refill path: minFloat(burst=1, 0+10*100=1000) → cap at 1.
	ok, _, err := l.Allow(ctx, "k", ScopeRead)
	if err != nil {
		t.Fatalf("post-refill Allow: unexpected err %v", err)
	}
	if !ok {
		t.Fatal("post-refill Allow: want allowed (bucket refilled to cap)")
	}

	// A second call must be denied (bucket is exactly 1 token, consumed above).
	if ok2, _, _ := l.Allow(ctx, "k", ScopeRead); ok2 {
		t.Fatal("post-refill second call: want denied (burst is 1)")
	}
}

// TestMemoryLimiter_RetryAfterFloor exercises the floor guard in Allow:
// when the computed retryAfter is <= 0 (truncated to 0 by float→Duration
// conversion), the implementation clamps it to 1ms.
//
// This happens when missing/refillPerSec * float64(time.Second) < 1 — i.e.,
// the nanosecond duration is sub-nanosecond after truncation.
// With burst=2, refill=1e12 and tokens=0: missing=1,
// retryAfter = 1/1e12 * 1e9 = 0.001 → truncates to Duration(0) → floor to 1ms.
func TestMemoryLimiter_RetryAfterFloor(t *testing.T) {
	ctx := context.Background()

	// burst=2, refill=1e12 (one trillion tokens per second).
	// Drain 2 tokens so tokens=0. Then the missing=1 token, retryAfter =
	// 1/1e12 * 1e9ns = 0.001 → Duration(0) → floor guard fires → 1ms.
	now := time.Now()
	l := NewMemoryLimiter(2, 1e12)
	l.clock = func() time.Time { return now }

	// Drain the bucket completely.
	for i := 0; i < 2; i++ {
		if ok, _, err := l.Allow(ctx, "k", ScopeRead); !ok || err != nil {
			t.Fatalf("drain call %d: want allowed; ok=%v err=%v", i, ok, err)
		}
	}

	// Next call: tokens=0, missing=1, refillPerSec=1e12.
	// raw retryAfter = Duration(1/1e12 * 1e9) = Duration(0.001) = 0 → floor to 1ms.
	ok, retryAfter, err := l.Allow(ctx, "k", ScopeRead)
	if err != nil {
		t.Fatalf("floor test Allow: unexpected err %v", err)
	}
	if ok {
		t.Fatal("floor test Allow: want denied")
	}
	if retryAfter != time.Millisecond {
		t.Fatalf("floor test: retryAfter want 1ms (floor guard), got %v", retryAfter)
	}
}

// ---------- toInt64 ----------

func TestToInt64_Int64Input(t *testing.T) {
	var v any = int64(42)
	got, ok := toInt64(v)
	if !ok || got != 42 {
		t.Fatalf("toInt64(int64(42)): want (42, true), got (%d, %v)", got, ok)
	}
}

func TestToInt64_IntInput(t *testing.T) {
	var v any = int(99)
	got, ok := toInt64(v)
	if !ok || got != 99 {
		t.Fatalf("toInt64(int(99)): want (99, true), got (%d, %v)", got, ok)
	}
}

func TestToInt64_StringInput(t *testing.T) {
	var v any = "12345"
	got, ok := toInt64(v)
	if !ok || got != 12345 {
		t.Fatalf("toInt64(%q): want (12345, true), got (%d, %v)", v, got, ok)
	}
}

func TestToInt64_InvalidStringInput(t *testing.T) {
	var v any = "not-a-number"
	got, ok := toInt64(v)
	if ok || got != 0 {
		t.Fatalf("toInt64(%q): want (0, false), got (%d, %v)", v, got, ok)
	}
}

func TestToInt64_UnknownTypeInput(t *testing.T) {
	var v any = 3.14
	got, ok := toInt64(v)
	if ok || got != 0 {
		t.Fatalf("toInt64(float64): want (0, false), got (%d, %v)", got, ok)
	}
}

// ---------- RedisLimiter.Allow — unexpected reply branch ----------

// TestRedisLimiter_UnexpectedReply covers the branch in Allow that returns an
// error when the Lua script result is not a []any of exactly 2 elements.
// We achieve this by registering a different Lua script on the RedisLimiter
// struct that returns a scalar instead of a list.
//
// Since the script field is unexported, we can replace it from inside the
// package (internal test).
func TestRedisLimiter_UnexpectedReply(t *testing.T) {
	ctx := context.Background()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = rdb.Close() }()

	l := NewRedisLimiter(rdb, 10, 1, "test")
	// Replace the Lua script with one that returns a scalar (integer) instead
	// of a two-element array — this triggers the !ok || len(values) != 2 branch.
	l.script = goredis.NewScript(`return 1`)

	ok, _, err := l.Allow(ctx, "u", ScopeRead)
	if err == nil {
		t.Fatal("unexpected reply: want error, got nil")
	}
	if ok {
		t.Fatal("unexpected reply: want denied")
	}
}
