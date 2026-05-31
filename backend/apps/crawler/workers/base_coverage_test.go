package workers_test

import (
	"context"
	stderrors "errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/workers"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
)

// TestNewBaseWorker_ConstructorSetsFields verifies that NewBaseWorker (the
// production constructor that uses the default heartbeat interval) returns a
// non-zero worker that can be started and stopped cleanly.
func TestNewBaseWorker_ConstructorSetsFields(t *testing.T) {
	q := &mockQueue{}
	s := &mockScanner{}
	log := logger.NewNoop()

	// NewBaseWorker uses the default heartbeat interval (not overridable).
	w := workers.NewBaseWorker("test-adapter", q, s, log, 10*time.Millisecond)

	// Verify the worker is usable by running it briefly and cancelling.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.RunLoop(ctx)
		close(done)
	}()

	// Give the loop time to start.
	time.Sleep(25 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Good — loop exited as expected.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("RunLoop did not return after context cancellation")
	}
}

// warnHeartbeatQueue returns one item on the first ClaimNext call, then nil.
// UpdateHeartbeat returns a generic error (not ErrQueueRowMissing) to exercise
// the Warn-and-continue branch in startHeartbeat.
type warnHeartbeatQueue struct {
	item           *workers.ScanQueueItem
	returned       atomic.Bool
	heartbeatCalls atomic.Int64
	heartbeatErr   error
}

func (q *warnHeartbeatQueue) ClaimNext(_ context.Context, _ string) (*workers.ScanQueueItem, error) {
	if q.returned.CompareAndSwap(false, true) {
		return q.item, nil
	}
	return nil, nil
}

func (q *warnHeartbeatQueue) Complete(_ context.Context, _ string) error { return nil }

func (q *warnHeartbeatQueue) UpdateHeartbeat(_ context.Context, _ string) error {
	q.heartbeatCalls.Add(1)
	return q.heartbeatErr
}

// TestStartHeartbeat_NonMissingRowError verifies that when UpdateHeartbeat
// returns a generic (non-ErrQueueRowMissing) error the heartbeat logs Warn and
// continues — the scan is not aborted and the worker keeps running.
func TestStartHeartbeat_NonMissingRowError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	item := &workers.ScanQueueItem{ID: "q-warn", CompanyID: "c-warn", AdapterType: "test", Source: "scheduler"}
	q := &warnHeartbeatQueue{
		item:         item,
		heartbeatErr: stderrors.New("transient heartbeat error"),
	}
	// Slow scanner so we get multiple heartbeat ticks.
	s := &slowScanner{dur: 100 * time.Millisecond}

	// Use a short heartbeat interval so we can observe multiple warn ticks quickly.
	w := workers.NewBaseWorkerWithHeartbeat("test", q, s, logger.NewNoop(), 10*time.Millisecond, 20*time.Millisecond)

	go func() { w.RunLoop(ctx) }()

	// Wait long enough for at least 2 heartbeat ticks to fire while the slow
	// scanner is running.
	time.Sleep(200 * time.Millisecond)

	hbCalls := q.heartbeatCalls.Load()
	if hbCalls < 2 {
		t.Errorf("expected at least 2 heartbeat calls while scan ran; got %d", hbCalls)
	}
}

// reassignQueue returns one item on the first ClaimNext call, then nil.
// UpdateHeartbeat returns ErrQueueRowMissing on the first heartbeat tick,
// which exercises the reassignment abort path in startHeartbeat.
type reassignQueue struct {
	item           *workers.ScanQueueItem
	returned       atomic.Bool
	heartbeatCalls atomic.Int64
	completeCalls  atomic.Int64
}

func (q *reassignQueue) ClaimNext(_ context.Context, _ string) (*workers.ScanQueueItem, error) {
	if q.returned.CompareAndSwap(false, true) {
		return q.item, nil
	}
	return nil, nil
}

func (q *reassignQueue) Complete(_ context.Context, _ string) error {
	q.completeCalls.Add(1)
	return nil
}

func (q *reassignQueue) UpdateHeartbeat(_ context.Context, _ string) error {
	q.heartbeatCalls.Add(1)
	return domainerrs.ErrQueueRowMissing
}

// TestRunLoop_HeartbeatReassignment exercises the ErrQueueRowMissing path in
// startHeartbeat: the scan is aborted mid-run and Complete is skipped.
func TestRunLoop_HeartbeatReassignment(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	item := &workers.ScanQueueItem{ID: "q-reassign", CompanyID: "c-reassign", AdapterType: "test", Source: "scheduler"}
	q := &reassignQueue{item: item}
	// Slow scanner so the heartbeat fires while the scan is still in progress.
	s := &slowScanner{dur: 200 * time.Millisecond}

	w := workers.NewBaseWorkerWithHeartbeat("test", q, s, logger.NewNoop(), 10*time.Millisecond, 20*time.Millisecond)

	go func() { w.RunLoop(ctx) }()

	// Wait long enough for the heartbeat to fire and abort the scan.
	time.Sleep(300 * time.Millisecond)

	if q.heartbeatCalls.Load() < 1 {
		t.Error("expected at least one heartbeat call")
	}
	// Complete must NOT have been called — reassignment means we skip it.
	if q.completeCalls.Load() != 0 {
		t.Errorf("Complete should not be called on reassignment, got %d calls", q.completeCalls.Load())
	}
}

// TestStartHeartbeatForTest_ExposedHelper verifies that StartHeartbeatForTest
// works correctly: it starts a heartbeat goroutine and the stop func stops it.
func TestStartHeartbeatForTest_ExposedHelper(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := &mockQueue{}
	s := &mockScanner{}
	w := workers.NewBaseWorkerWithHeartbeat("test", q, s, logger.NewNoop(), time.Second, 20*time.Millisecond)

	scanCtx, cancelScan := context.WithCancel(ctx)
	defer cancelScan()

	stop, reassigned := w.StartHeartbeatForTest(scanCtx, cancelScan, "q-hbt")

	// Wait for a heartbeat tick.
	time.Sleep(50 * time.Millisecond)
	stop()

	if reassigned.Load() {
		t.Error("reassigned should be false when heartbeat returns nil")
	}
}

// TestRegistry_Len verifies that Len returns the correct count.
func TestRegistry_Len(t *testing.T) {
	r := workers.NewRegistry()
	if r.Len() != 0 {
		t.Fatalf("expected 0, got %d", r.Len())
	}
	r.Register(&stubService{adapterType: "a"})
	r.Register(&stubService{adapterType: "b"})
	if r.Len() != 2 {
		t.Fatalf("expected 2, got %d", r.Len())
	}
}
