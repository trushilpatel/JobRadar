package workers_test

import (
	"context"
	stderrors "errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/workers"
	"github.com/trushilpatel/jobradar/common/logger"
)

// errOnceQueue returns an error on the first ClaimNext call, then nil items.
// Exercises the "claim failed" log + sleep + continue branch in RunLoop.
type errOnceQueue struct {
	returned atomic.Bool
}

func (q *errOnceQueue) ClaimNext(_ context.Context, _ string) (*workers.ScanQueueItem, error) {
	if q.returned.CompareAndSwap(false, true) {
		return nil, stderrors.New("transient claim failure")
	}
	return nil, nil
}

func (q *errOnceQueue) Complete(_ context.Context, _ string) error        { return nil }
func (q *errOnceQueue) UpdateHeartbeat(_ context.Context, _ string) error { return nil }

func TestRunLoop_RecoversFromClaimError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := &errOnceQueue{}
	s := &mockScanner{}

	w := workers.NewBaseWorkerWithHeartbeat("test", q, s, logger.NewNoop(), 10*time.Millisecond, time.Minute)
	go func() { w.RunLoop(ctx) }()

	// Give the loop ~60ms to: err out once, sleep, retry, get nil, sleep again.
	time.Sleep(60 * time.Millisecond)
	cancel()

	// Scanner must NOT have been called — every ClaimNext after the first returns nil.
	if s.calls.Load() != 0 {
		t.Errorf("scanner should not have run after claim errors; got %d calls", s.calls.Load())
	}
}

// completeErrQueue returns an item once then nil; Complete always errors.
// Exercises the Complete-error log branch in RunLoop.
type completeErrQueue struct {
	item     *workers.ScanQueueItem
	returned atomic.Bool
}

func (q *completeErrQueue) ClaimNext(_ context.Context, _ string) (*workers.ScanQueueItem, error) {
	if q.returned.CompareAndSwap(false, true) {
		return q.item, nil
	}
	return nil, nil
}

func (q *completeErrQueue) Complete(_ context.Context, _ string) error {
	return stderrors.New("complete-failed")
}

func (q *completeErrQueue) UpdateHeartbeat(_ context.Context, _ string) error { return nil }

func TestRunLoop_LogsCompleteError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	item := &workers.ScanQueueItem{ID: "q-c", CompanyID: "c", AdapterType: "test", Source: "scheduler"}
	q := &completeErrQueue{item: item}
	s := &mockScanner{}

	w := workers.NewBaseWorkerWithHeartbeat("test", q, s, logger.NewNoop(), 10*time.Millisecond, time.Minute)
	go func() { w.RunLoop(ctx) }()

	// Wait for the one scan + complete attempt.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if s.calls.Load() >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if s.calls.Load() < 1 {
		t.Fatal("scanner.RunForCompany was not called")
	}
	// Loop must continue past Complete-failed without dying.
	time.Sleep(30 * time.Millisecond)
}

// panicScanner panics inside RunForCompany — exercises the recover() branch.
type panicScanner struct{ called atomic.Int64 }

func (p *panicScanner) RunForCompany(_ context.Context, _ string) {
	p.called.Add(1)
	panic("boom")
}

func TestRunLoop_RecoversFromScannerPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	item := &workers.ScanQueueItem{ID: "q-p", CompanyID: "c", AdapterType: "test", Source: "scheduler"}
	q := &firstOnceQueue{first: item}
	s := &panicScanner{}

	w := workers.NewBaseWorkerWithHeartbeat("test", q, s, logger.NewNoop(), 10*time.Millisecond, time.Minute)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("RunLoop must recover from scanner panic, got: %v", r)
			}
		}()
		w.RunLoop(ctx)
	}()

	// Wait for the panicking call + Complete to occur, then cancel.
	time.Sleep(60 * time.Millisecond)
	if s.called.Load() < 1 {
		t.Fatal("panicScanner was never invoked")
	}
}

func TestSleep_ReturnsImmediatelyOnContextCancel(t *testing.T) {
	w := workers.NewBaseWorkerWithHeartbeat("t", &mockQueue{}, &mockScanner{}, logger.NewNoop(), time.Second, time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled

	start := time.Now()
	if err := w.Sleep(ctx, 10*time.Second); err == nil {
		t.Fatal("expected ctx.Err() from cancelled-context Sleep")
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("Sleep on cancelled ctx should return immediately, took %v", elapsed)
	}
}

func TestSleep_RunsFullDurationWhenCtxLives(t *testing.T) {
	w := workers.NewBaseWorkerWithHeartbeat("t", &mockQueue{}, &mockScanner{}, logger.NewNoop(), time.Second, time.Minute)

	start := time.Now()
	if err := w.Sleep(context.Background(), 30*time.Millisecond); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 25*time.Millisecond {
		t.Fatalf("Sleep returned too early: %v", elapsed)
	}
}

// cancelAfterClaimErrQueue returns an error on the first ClaimNext call, then
// cancels the provided context so the subsequent Sleep receives a cancelled ctx.
// This exercises the `return` inside the claim-error sleep path (line 79 of
// base.go: `if err := w.Sleep(ctx, ...); err != nil { return }`).
type cancelAfterClaimErrQueue struct {
	returned atomic.Bool
	cancel   context.CancelFunc
}

func (q *cancelAfterClaimErrQueue) ClaimNext(ctx context.Context, _ string) (*workers.ScanQueueItem, error) {
	if q.returned.CompareAndSwap(false, true) {
		// Cancel the context immediately before returning — the caller's
		// Sleep will see a cancelled ctx and return an error, triggering the
		// `return` branch in RunLoop.
		q.cancel()
		return nil, stderrors.New("transient claim failure")
	}
	return nil, nil
}

func (q *cancelAfterClaimErrQueue) Complete(_ context.Context, _ string) error        { return nil }
func (q *cancelAfterClaimErrQueue) UpdateHeartbeat(_ context.Context, _ string) error { return nil }

// TestRunLoop_ExitsOnCancelledContextDuringClaimErrorSleep verifies that
// RunLoop exits cleanly when the context is cancelled during the Sleep that
// follows a ClaimNext error.
func TestRunLoop_ExitsOnCancelledContextDuringClaimErrorSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	q := &cancelAfterClaimErrQueue{cancel: cancel}
	s := &mockScanner{}

	// Use a long poll interval; the cancel fires before the sleep can complete.
	w := workers.NewBaseWorkerWithHeartbeat("test", q, s, logger.NewNoop(), 10*time.Second, time.Minute)

	done := make(chan struct{})
	go func() {
		w.RunLoop(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Good — RunLoop returned after the cancelled sleep.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("RunLoop did not return after context cancelled during claim-error sleep")
	}
	if s.calls.Load() != 0 {
		t.Errorf("scanner should not run when RunLoop exits via claim-error path")
	}
}

// cancelDuringRunScanner cancels the supplied context while RunForCompany is
// executing — simulating the parent context being cancelled mid-scan.
type cancelDuringRunScanner struct {
	cancel context.CancelFunc
	called atomic.Int64
}

func (s *cancelDuringRunScanner) RunForCompany(_ context.Context, _ string) {
	s.called.Add(1)
	s.cancel()
}

// itemOnceQueue returns one item on the first ClaimNext call, then nil.
// Heartbeat always succeeds so the reassignment path is not taken.
type itemOnceQueue struct {
	item          *workers.ScanQueueItem
	returned      atomic.Bool
	completeCalls atomic.Int64
}

func (q *itemOnceQueue) ClaimNext(_ context.Context, _ string) (*workers.ScanQueueItem, error) {
	if q.returned.CompareAndSwap(false, true) {
		return q.item, nil
	}
	return nil, nil
}
func (q *itemOnceQueue) Complete(_ context.Context, _ string) error {
	q.completeCalls.Add(1)
	return nil
}
func (q *itemOnceQueue) UpdateHeartbeat(_ context.Context, _ string) error { return nil }

// TestRunLoop_ExitsAfterScanWhenContextCancelled verifies that RunLoop takes
// the `if ctx.Err() != nil { return }` branch after a scan completes when the
// context was cancelled mid-scan (line 128-131 of base.go).
func TestRunLoop_ExitsAfterScanWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	item := &workers.ScanQueueItem{ID: "q-cancel", CompanyID: "c-cancel", AdapterType: "test", Source: "scheduler"}
	q := &itemOnceQueue{item: item}
	s := &cancelDuringRunScanner{cancel: cancel}

	// Long poll interval so we don't loop — the context cancel will exit.
	w := workers.NewBaseWorkerWithHeartbeat("test", q, s, logger.NewNoop(), 10*time.Second, time.Minute)

	done := make(chan struct{})
	go func() {
		w.RunLoop(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Good — RunLoop returned after context was cancelled mid-scan.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("RunLoop did not exit after context cancelled during scan")
	}
	if s.called.Load() < 1 {
		t.Fatal("scanner was not invoked")
	}
	// Complete must NOT be called — ctx.Err() != nil causes early return.
	if q.completeCalls.Load() != 0 {
		t.Errorf("Complete should not be called when ctx is cancelled after scan, got %d calls", q.completeCalls.Load())
	}
}
