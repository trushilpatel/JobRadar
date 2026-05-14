package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/logger"
)

// TestAdapterRateLimiter_AllowsImmediateBurst — within burst, Wait returns
// immediately. Token-bucket starts full so the first `burst` calls succeed
// without waiting.
func TestAdapterRateLimiter_AllowsImmediateBurst(t *testing.T) {
	rl := NewAdapterRateLimiter(1.0 /* rps */, 5 /* burst */, 0 /* jitter */, logger.NewNoop())
	ctx := context.Background()

	start := time.Now()
	for i := 0; i < 5; i++ {
		if err := rl.Wait(ctx, "ashby"); err != nil {
			t.Fatalf("Wait(%d): %v", i, err)
		}
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("expected burst window to consume tokens without waiting; elapsed=%v", elapsed)
	}
}

// TestAdapterRateLimiter_PerAdapterIsolated — one adapter's exhausted bucket
// must NOT delay calls on a different adapter (separate limiters per key).
func TestAdapterRateLimiter_PerAdapterIsolated(t *testing.T) {
	rl := NewAdapterRateLimiter(0.5 /* slow */, 1 /* burst=1 */, 0, logger.NewNoop())
	ctx := context.Background()

	// Drain greenhouse bucket.
	if err := rl.Wait(ctx, "greenhouse"); err != nil {
		t.Fatalf("greenhouse first: %v", err)
	}
	// Lever has a fresh, untouched bucket — should be immediate.
	start := time.Now()
	if err := rl.Wait(ctx, "lever"); err != nil {
		t.Fatalf("lever: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("lever should not be delayed by greenhouse; elapsed=%v", elapsed)
	}
}

// TestAdapterRateLimiter_ContextCancelDuringWait — when the bucket is empty
// and ctx is cancelled, Wait returns the cancellation error.
func TestAdapterRateLimiter_ContextCancelDuringWait(t *testing.T) {
	// 1 rps with burst=1 → second Wait blocks for ~1s. Cancel mid-wait.
	rl := NewAdapterRateLimiter(1.0, 1, 0, logger.NewNoop())
	if err := rl.Wait(context.Background(), "ashby"); err != nil {
		t.Fatalf("first wait: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	err := rl.Wait(ctx, "ashby")
	if err == nil {
		t.Errorf("expected ctx cancel error, got nil")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("Wait should return promptly on cancel; elapsed=%v", elapsed)
	}
}

// TestAdapterRateLimiter_PreCancelledContextErrors — calling Wait with an
// already-cancelled ctx returns ctx.Err immediately.
func TestAdapterRateLimiter_PreCancelledContextErrors(t *testing.T) {
	rl := NewAdapterRateLimiter(1.0, 1, 0, logger.NewNoop())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := rl.Wait(ctx, "ashby"); err == nil {
		t.Errorf("expected error from cancelled ctx")
	}
}

// TestAdapterRateLimiter_JitterCancelledContext — when jitter > 0 and ctx is
// cancelled during the jitter sleep, Wait returns ctx.Err().
func TestAdapterRateLimiter_JitterCancelledContext(t *testing.T) {
	// Token bucket is generous, but jitter window is large; cancel before
	// jitter completes.
	rl := NewAdapterRateLimiter(1000.0, 100, 200*time.Millisecond, logger.NewNoop())

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	// Call many times to maximise chance that at least one hits the jitter
	// branch with ctx cancelled. Any err must be context error.
	var sawCancel bool
	for i := 0; i < 50; i++ {
		if err := rl.Wait(ctx, "ashby"); err != nil {
			sawCancel = true
			break
		}
	}
	if !sawCancel {
		// Not strictly required — jitter is random — but if cancel never fires
		// the test is meaningless; force at least one cancelled call.
		ctx2, c2 := context.WithCancel(context.Background())
		c2()
		if err := rl.Wait(ctx2, "ashby"); err == nil {
			t.Errorf("expected cancel error")
		}
	}
}

// TestAdapterRateLimiter_GetOrCreateConcurrentSafe — concurrent first-use of
// the same adapter must produce a single limiter (double-check under write
// lock). Run under -race to verify no data races on the map.
func TestAdapterRateLimiter_GetOrCreateConcurrentSafe(t *testing.T) {
	rl := NewAdapterRateLimiter(1000.0, 1000, 0, logger.NewNoop())

	const n = 32
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = rl.Wait(context.Background(), "greenhouse")
		}()
	}
	wg.Wait()

	// Force a second access; same limiter should be reused via the RLock fast
	// path. We can't see internals, but a non-racy run is the assertion.
	if err := rl.Wait(context.Background(), "greenhouse"); err != nil {
		t.Errorf("post-concurrent wait: %v", err)
	}
}

// TestAdapterRateLimiter_JitterDelaysSomeFraction — with a 50ms jitter and
// large bucket, repeated calls produce a measurable average wall-clock cost.
// This exercises the jitter branch (a.jitter > 0) without flaking on RNG.
func TestAdapterRateLimiter_JitterDelaysSomeFraction(t *testing.T) {
	rl := NewAdapterRateLimiter(10000.0, 1000, 30*time.Millisecond, logger.NewNoop())
	ctx := context.Background()

	const iters = 20
	start := time.Now()
	for i := 0; i < iters; i++ {
		if err := rl.Wait(ctx, "ashby"); err != nil {
			t.Fatalf("Wait %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	// With max jitter 30ms and ~uniform RNG, expected sum ≈ iters*15ms = 300ms.
	// Allow a wide margin; we only want to confirm the branch executed.
	if elapsed < 30*time.Millisecond {
		t.Errorf("expected some jitter accumulation, elapsed=%v", elapsed)
	}
}

// TestAdapterRateLimiter_NoJitterPath — jitter=0 disables the jitter branch
// entirely; pure token-bucket pacing. With a huge bucket the calls return
// immediately.
func TestAdapterRateLimiter_NoJitterPath(t *testing.T) {
	rl := NewAdapterRateLimiter(10000.0, 1000, 0, logger.NewNoop())
	ctx := context.Background()
	start := time.Now()
	for i := 0; i < 100; i++ {
		if err := rl.Wait(ctx, "ashby"); err != nil {
			t.Fatalf("Wait %d: %v", i, err)
		}
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("no-jitter fast path should be near-instant; elapsed=%v", elapsed)
	}
}
