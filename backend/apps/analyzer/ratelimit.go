package main

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/trushilpatel/jobradar/common/logger"
)

// AdapterRateLimiter enforces a separate token-bucket rate limit per ATS
// adapter. Each adapter (greenhouse, lever, ashby, netflix, ...) gets its own
// limiter so that hammering one platform never starves another. Limiters are
// created lazily on first use with the configured default RPS and burst.
//
// A small random jitter is added after each Wait so concurrent fetches don't
// all fire on identical bucket boundaries — looks less like a bot, costs us
// nothing.
type AdapterRateLimiter struct {
	mu           sync.RWMutex
	limiters     map[string]*rate.Limiter
	defaultRPS   float64
	defaultBurst int
	jitter       time.Duration
	log          logger.Logger
}

// NewAdapterRateLimiter constructs a limiter pool with the given defaults.
// rps and burst must be > 0 (caller validates). jitter is the maximum random
// delay added after each token grant — set to 0 to disable.
func NewAdapterRateLimiter(rps float64, burst int, jitter time.Duration, log logger.Logger) *AdapterRateLimiter {
	return &AdapterRateLimiter{
		limiters:     make(map[string]*rate.Limiter),
		defaultRPS:   rps,
		defaultBurst: burst,
		jitter:       jitter,
		log:          log,
	}
}

// Wait blocks until the adapter's rate limiter grants a token, then sleeps an
// additional random jitter. Returns the context's error if cancelled while
// waiting.
//
// Logs a debug line when the wait exceeds 100ms so slow adapters are visible
// without spamming the log on every call.
func (a *AdapterRateLimiter) Wait(ctx context.Context, adapter string) error {
	limiter := a.getOrCreate(adapter)
	start := time.Now()
	if err := limiter.Wait(ctx); err != nil {
		return err
	}
	if waited := time.Since(start); waited > 100*time.Millisecond {
		a.log.Debug("rate limit wait",
			logger.Str("adapter", adapter),
			logger.Dur("waited", waited),
		)
	}
	if a.jitter > 0 {
		// nolint:gosec — math/rand is fine here; this is jitter, not security
		delay := time.Duration(rand.Int63n(int64(a.jitter)))
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (a *AdapterRateLimiter) getOrCreate(adapter string) *rate.Limiter {
	a.mu.RLock()
	l, ok := a.limiters[adapter]
	a.mu.RUnlock()
	if ok {
		return l
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	// Double-check under write lock — another goroutine may have raced us.
	if l, ok := a.limiters[adapter]; ok {
		return l
	}
	l = rate.NewLimiter(rate.Limit(a.defaultRPS), a.defaultBurst)
	a.limiters[adapter] = l
	return l
}
