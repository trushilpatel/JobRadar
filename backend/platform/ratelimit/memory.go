package ratelimit

import (
	"context"
	"sync"
	"time"
)

// MemoryLimiter is a single-process token-bucket Limiter. It is the
// authoritative reference implementation: the Redis variant must produce
// identical decisions for the same input sequence.
//
// Each (key, scope) tuple has an independent bucket initialised lazily on
// first Allow. Tokens refill linearly at refillPerSec until the bucket is
// full. A request consumes one token; if none remain Allow returns
// (false, retryAfter, nil) where retryAfter is the time until one token
// becomes available.
type MemoryLimiter struct {
	burst        float64
	refillPerSec float64
	mu           sync.Mutex
	buckets      map[bucketKey]*memoryBucket
	clock        func() time.Time
}

type bucketKey struct {
	key   string
	scope Scope
}

type memoryBucket struct {
	tokens float64
	last   time.Time
}

// NewMemoryLimiter returns a Limiter with the given bucket size and refill
// rate. burst caps the maximum tokens; refillPerSec is the steady-state
// allowance.
//
// burst <= 0 or refillPerSec <= 0 yield a Limiter that always denies — a
// configuration error rather than a silent permissive default.
func NewMemoryLimiter(burst, refillPerSec float64) *MemoryLimiter {
	return &MemoryLimiter{
		burst:        burst,
		refillPerSec: refillPerSec,
		buckets:      make(map[bucketKey]*memoryBucket),
		clock:        time.Now,
	}
}

// Allow consumes one token from the (key, scope) bucket. Returns
// (true, 0, nil) on success and (false, retryAfter, nil) when the bucket
// is empty.
func (l *MemoryLimiter) Allow(_ context.Context, key string, scope Scope) (bool, time.Duration, error) {
	if l.burst <= 0 || l.refillPerSec <= 0 {
		return false, time.Second, nil
	}

	now := l.clock()
	bk := bucketKey{key: key, scope: scope}

	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[bk]
	if !ok {
		// New bucket starts full — first request never blocks.
		b = &memoryBucket{tokens: l.burst, last: now}
		l.buckets[bk] = b
	} else {
		// Refill proportionally to elapsed time.
		elapsed := now.Sub(b.last).Seconds()
		if elapsed > 0 {
			b.tokens = minFloat(l.burst, b.tokens+elapsed*l.refillPerSec)
			b.last = now
		}
	}

	if b.tokens >= 1 {
		b.tokens--
		return true, 0, nil
	}

	// Compute how long until one token regenerates.
	missing := 1 - b.tokens
	retryAfter := time.Duration(missing / l.refillPerSec * float64(time.Second))
	if retryAfter <= 0 {
		retryAfter = time.Millisecond // floor — never advertise zero on a deny
	}
	return false, retryAfter, nil
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
