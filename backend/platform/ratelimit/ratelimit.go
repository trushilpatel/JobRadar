// Package ratelimit provides a small Limiter abstraction shared by the API
// rate-limit middleware. Two implementations ship with the package:
//
//   - RedisLimiter (production) — atomic token bucket via Lua, shared state
//     across API replicas.
//   - MemoryLimiter (tests)     — single-process token bucket, identical
//     semantics so tests exercise the same contract as production.
//
// Scope encodes the family of operations a key is being throttled for so a
// single Redis namespace can host independent buckets per scope.
package ratelimit

import (
	"context"
	"net/http"
	"time"
)

// Scope names a throttling family. Keys are unique per (key, scope) tuple
// — a user may have a permissive Read budget alongside a tight Write one.
type Scope string

const (
	// ScopeRead covers safe, non-mutating requests (GET / HEAD / OPTIONS).
	ScopeRead Scope = "read"
	// ScopeWrite covers mutating requests (POST / PATCH / PUT / DELETE).
	ScopeWrite Scope = "write"
	// ScopeAdmin covers admin-only operations; the middleware applies it
	// after RequireRole gates an admin handler.
	ScopeAdmin Scope = "admin"
)

// Limiter is the contract every implementation honours. A returned
// retryAfter is meaningful only when allowed is false.
//
// Implementations must be safe for concurrent use. Implementations must
// never panic on a closed transport — return the error so the middleware
// can fail open or closed per its policy.
type Limiter interface {
	Allow(ctx context.Context, key string, scope Scope) (allowed bool, retryAfter time.Duration, err error)
}

// ScopeFromMethod maps an HTTP method to the appropriate Scope.
//
//   - GET / HEAD / OPTIONS  → ScopeRead
//   - everything else       → ScopeWrite
//
// ScopeAdmin is intentionally never returned from this helper — admin
// gating is a per-route decision, not a method-level one. Middleware that
// needs ScopeAdmin must opt in explicitly.
func ScopeFromMethod(method string) Scope {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return ScopeRead
	default:
		return ScopeWrite
	}
}
