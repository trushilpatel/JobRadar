package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/identity"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/platform/ratelimit"
)

// stubLimiter is a hand-rolled Limiter that returns canned responses so tests
// can exercise allow / deny / error paths without spinning up Redis.
type stubLimiter struct {
	allow      bool
	retryAfter time.Duration
	err        error

	gotKey   string
	gotScope ratelimit.Scope
	calls    int
}

func (s *stubLimiter) Allow(_ context.Context, key string, scope ratelimit.Scope) (bool, time.Duration, error) {
	s.calls++
	s.gotKey = key
	s.gotScope = scope
	return s.allow, s.retryAfter, s.err
}

func TestRateLimit_Allowed_PassesThrough(t *testing.T) {
	stub := &stubLimiter{allow: true}
	mw := RateLimit(stub, logger.NewNoop())
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req = req.WithContext(identity.WithTestIdentity(req.Context(), "u1"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !called {
		t.Fatal("next must be called when allowed")
	}
	if stub.gotKey != "u1" {
		t.Fatalf("limiter key=%q want u1", stub.gotKey)
	}
	if stub.gotScope != ratelimit.ScopeRead {
		t.Fatalf("scope=%q want read", stub.gotScope)
	}
}

func TestRateLimit_Denied_429_WithRetryAfter(t *testing.T) {
	stub := &stubLimiter{allow: false, retryAfter: 1500 * time.Millisecond}
	mw := RateLimit(stub, logger.NewNoop())
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/x", nil)
	req = req.WithContext(identity.WithTestIdentity(req.Context(), "u2"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if called {
		t.Fatal("next must NOT be called when denied")
	}
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d want 429", rr.Code)
	}
	if got := rr.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After=%q want 2", got)
	}
	if stub.gotScope != ratelimit.ScopeWrite {
		t.Fatalf("scope=%q want write for POST", stub.gotScope)
	}
}

func TestRateLimit_LimiterError_FailsOpen(t *testing.T) {
	stub := &stubLimiter{err: errors.New("redis down")}
	mw := RateLimit(stub, logger.NewNoop())
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req = req.WithContext(identity.WithTestIdentity(req.Context(), "u3"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !called {
		t.Fatal("next must be called when limiter errors (fail-open)")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d want 200 (fail-open)", rr.Code)
	}
}

func TestRateLimit_NoIdentity_FallsBackToRemoteAddr(t *testing.T) {
	stub := &stubLimiter{allow: true}
	mw := RateLimit(stub, logger.NewNoop())
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.RemoteAddr = "10.0.0.5:443"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if stub.gotKey != "10.0.0.5:443" {
		t.Fatalf("limiter key=%q want remote addr", stub.gotKey)
	}
}

func TestRateLimit_NilLimiter_NoOp(t *testing.T) {
	mw := RateLimit(nil, logger.NewNoop())
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if !called || rr.Code != http.StatusOK {
		t.Fatalf("nil limiter must pass through, got called=%v code=%d", called, rr.Code)
	}
}

func TestRetryAfterSeconds(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "1"},
		{-1 * time.Second, "1"},
		{500 * time.Millisecond, "1"},
		{1 * time.Second, "1"},
		{1500 * time.Millisecond, "2"},
		{30 * time.Second, "30"},
	}
	for _, tc := range cases {
		if got := retryAfterSeconds(tc.in); got != tc.want {
			t.Errorf("retryAfterSeconds(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
