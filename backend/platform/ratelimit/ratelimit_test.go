package ratelimit_test

import (
	"context"
	"net/http"
	"testing"
	"time"

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
	} {
		ok, retry, err := l.Allow(ctx, "k", ratelimit.ScopeRead)
		if err != nil {
			t.Fatalf("zero-config: unexpected err %v", err)
		}
		if ok {
			t.Fatal("zero-config: want denied")
		}
		if retry <= 0 {
			t.Fatal("zero-config: want retryAfter > 0")
		}
	}
}
