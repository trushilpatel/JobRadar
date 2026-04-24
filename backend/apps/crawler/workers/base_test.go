package workers_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/workers"
	"github.com/trushilpatel/jobradar/common/logger"
)

// --- Test doubles ---

// mockQueue records calls to ClaimNext, Complete and UpdateHeartbeat.
type mockQueue struct {
	claimItem  *workers.ScanQueueItem
	claimErr   error
	claimCalls atomic.Int64

	completeErr   error
	completeCalls atomic.Int64

	heartbeatCalls atomic.Int64
	heartbeatErr   error
}

func (m *mockQueue) ClaimNext(_ context.Context, _ string) (*workers.ScanQueueItem, error) {
	m.claimCalls.Add(1)
	return m.claimItem, m.claimErr
}

func (m *mockQueue) Complete(_ context.Context, _ string) error {
	m.completeCalls.Add(1)
	return m.completeErr
}

func (m *mockQueue) UpdateHeartbeat(_ context.Context, _ string) error {
	m.heartbeatCalls.Add(1)
	return m.heartbeatErr
}

var _ workers.CrawlerQueueService = (*mockQueue)(nil)

// mockScanner records RunForCompany calls.
type mockScanner struct {
	calls atomic.Int64
}

func (m *mockScanner) RunForCompany(_ context.Context, _ string) {
	m.calls.Add(1)
}

var _ workers.Scanner = (*mockScanner)(nil)

// --- RunLoop tests ---

func TestRunLoop_ReturnsWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	q := &mockQueue{} // ClaimNext returns nil, nil → worker sleeps
	s := &mockScanner{}

	w := workers.NewBaseWorkerWithHeartbeat("test", q, s, logger.NewNoop(), 10*time.Millisecond, time.Minute)

	done := make(chan struct{})
	go func() {
		w.RunLoop(ctx)
		close(done)
	}()

	// Give the loop time to start sleeping.
	time.Sleep(25 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Good — loop exited.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("RunLoop did not return after context cancellation")
	}
}

func TestRunLoop_SleepsWhenQueueEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := &mockQueue{} // ClaimNext returns nil → sleep
	s := &mockScanner{}

	// Very short poll so we can measure multiple ticks.
	w := workers.NewBaseWorkerWithHeartbeat("test", q, s, logger.NewNoop(), 10*time.Millisecond, time.Minute)

	go func() { w.RunLoop(ctx) }()

	time.Sleep(60 * time.Millisecond)
	claims := q.claimCalls.Load()
	if claims < 3 {
		t.Errorf("expected at least 3 ClaimNext calls with 10ms poll, got %d", claims)
	}
	if s.calls.Load() != 0 {
		t.Errorf("scanner should not be called when queue is empty, got %d calls", s.calls.Load())
	}
}

func TestRunLoop_ClaimScanComplete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	item := &workers.ScanQueueItem{ID: "q-1", CompanyID: "c-1", AdapterType: "test", Source: "scheduler"}

	// We need custom behaviour: first call returns item, rest return nil.
	custom := &firstOnceQueue{first: item}

	s := &mockScanner{}

	w := workers.NewBaseWorkerWithHeartbeat("test", custom, s, logger.NewNoop(), 10*time.Millisecond, time.Minute)
	go func() { w.RunLoop(ctx) }()

	// Wait for scanner to be called.
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

	// Give Complete a moment to be called.
	time.Sleep(20 * time.Millisecond)
	if custom.completeCalls.Load() < 1 {
		t.Fatal("queue.Complete was not called after scan")
	}
}

// firstOnceQueue returns the first item on the first ClaimNext call, then nil.
type firstOnceQueue struct {
	first         *workers.ScanQueueItem
	returned      atomic.Bool
	completeCalls atomic.Int64
}

func (f *firstOnceQueue) ClaimNext(ctx context.Context, adapterType string) (*workers.ScanQueueItem, error) {
	if f.returned.CompareAndSwap(false, true) {
		return f.first, nil
	}
	return nil, nil
}

func (f *firstOnceQueue) Complete(_ context.Context, _ string) error {
	f.completeCalls.Add(1)
	return nil
}

func (f *firstOnceQueue) UpdateHeartbeat(_ context.Context, _ string) error { return nil }

func TestRunLoop_HeartbeatStartedAndStopped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	item := &workers.ScanQueueItem{ID: "q-hb", CompanyID: "c-hb", AdapterType: "test", Source: "scheduler"}
	q := &slowScanQueue{item: item}
	s := &slowScanner{dur: 80 * time.Millisecond}

	w := workers.NewBaseWorkerWithHeartbeat("test", q, s, logger.NewNoop(), 10*time.Millisecond, 20*time.Millisecond)
	go func() { w.RunLoop(ctx) }()

	// Wait for scan to complete.
	time.Sleep(200 * time.Millisecond)

	hb := q.heartbeatCalls.Load()
	if hb < 2 {
		t.Errorf("expected at least 2 heartbeat ticks during 80ms scan with 20ms interval, got %d", hb)
	}
}

// slowScanQueue returns item once then nil. Records heartbeat calls.
type slowScanQueue struct {
	item           *workers.ScanQueueItem
	returned       atomic.Bool
	heartbeatCalls atomic.Int64
}

func (q *slowScanQueue) ClaimNext(_ context.Context, _ string) (*workers.ScanQueueItem, error) {
	if q.returned.CompareAndSwap(false, true) {
		return q.item, nil
	}
	return nil, nil
}

func (q *slowScanQueue) Complete(_ context.Context, _ string) error { return nil }

func (q *slowScanQueue) UpdateHeartbeat(_ context.Context, _ string) error {
	q.heartbeatCalls.Add(1)
	return nil
}

// slowScanner sleeps for dur before returning.
type slowScanner struct{ dur time.Duration }

func (s *slowScanner) RunForCompany(_ context.Context, _ string) {
	time.Sleep(s.dur)
}
