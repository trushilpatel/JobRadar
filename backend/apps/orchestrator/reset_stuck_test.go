package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/logger"
)

// fakeResetStucker is a test double for the resetStucker interface.
//
// It records the number of calls made and can be configured to return a fixed
// error. Counters use atomic.Int64 so concurrent ticker fires under -race
// never tear during reads.
type fakeResetStucker struct {
	calls atomic.Int64
	err   error // returned on every call when non-nil
	n     int   // returned count on success
}

func (f *fakeResetStucker) ResetStuckForNodes(_ context.Context, _ time.Duration) (int, error) {
	f.calls.Add(1)
	if f.err != nil {
		return 0, f.err
	}
	return f.n, nil
}

// compile-time interface check
var _ resetStucker = (*fakeResetStucker)(nil)

// ── Happy path ────────────────────────────────────────────────────────────────

// TestRunResetStuck_CallsStuckerAtLeastOnce asserts that runResetStuck invokes
// ResetStuckForNodes at least once within a short interval before the context
// is cancelled.
func TestRunResetStuck_CallsStuckerAtLeastOnce(t *testing.T) {
	store := &fakeResetStucker{n: 2}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		runResetStuck(ctx, store, 50*time.Millisecond, logger.NewNoop())
		close(done)
	}()

	// Wait until at least one call has been made, then cancel.
	deadline := time.After(2 * time.Second)
	for store.calls.Load() < 1 {
		select {
		case <-deadline:
			t.Fatal("ResetStuckForNodes was not called within 2s")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runResetStuck did not return within 2s of context cancellation")
	}

	if store.calls.Load() < 1 {
		t.Errorf("expected at least 1 call to ResetStuckForNodes, got %d", store.calls.Load())
	}
}

// ── Context cancellation ──────────────────────────────────────────────────────

// TestRunResetStuck_ContextCancellation_ReturnsWithoutCalling asserts that
// when the context is cancelled before the ticker fires, runResetStuck returns
// promptly and calls ResetStuckForNodes at most once (in case the first tick
// and the cancellation race).
func TestRunResetStuck_ContextCancellation_ReturnsWithoutCalling(t *testing.T) {
	store := &fakeResetStucker{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately before the goroutine starts

	done := make(chan struct{})
	go func() {
		// Use a 1-second interval so the ticker cannot fire before ctx.Done is
		// selected — the cancellation wins the race.
		runResetStuck(ctx, store, 1*time.Second, logger.NewNoop())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runResetStuck did not return within 2s after immediate context cancellation")
	}

	// At most 1 call is acceptable if the first tick and cancellation race; 0 is
	// expected in practice with a 1-second interval and an already-cancelled ctx.
	if got := store.calls.Load(); got > 1 {
		t.Errorf("expected at most 1 call to ResetStuckForNodes after immediate cancellation, got %d", got)
	}
}

// ── Error path ────────────────────────────────────────────────────────────────

// TestRunResetStuck_ErrorFromStucker_LogsAndKeepsRunning asserts that when
// ResetStuckForNodes returns an error, runResetStuck logs the error and
// continues running — it does not return or panic.
func TestRunResetStuck_ErrorFromStucker_LogsAndKeepsRunning(t *testing.T) {
	store := &fakeResetStucker{err: errors.New("db down")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		runResetStuck(ctx, store, 50*time.Millisecond, logger.NewNoop())
		close(done)
	}()

	// Wait for at least 2 calls so we know the loop continued after the first error.
	deadline := time.After(2 * time.Second)
	for store.calls.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("expected at least 2 calls after errors, got %d within 2s", store.calls.Load())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runResetStuck did not return within 2s of context cancellation")
	}

	if got := store.calls.Load(); got < 2 {
		t.Errorf("expected at least 2 calls (loop must continue on error), got %d", got)
	}
}

// TestRunResetStuck_TicksMultipleTimes asserts the ticker branch fires more
// than once by using a very short interval and waiting for ≥2 total calls.
func TestRunResetStuck_TicksMultipleTimes(t *testing.T) {
	store := &fakeResetStucker{n: 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		runResetStuck(ctx, store, 50*time.Millisecond, logger.NewNoop())
		close(done)
	}()

	deadline := time.After(2 * time.Second)
	for store.calls.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("expected ≥2 ticker-driven calls, got %d within 2s", store.calls.Load())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runResetStuck did not return within 2s of context cancellation")
	}
}
