package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/workers"
	"github.com/trushilpatel/jobradar/common/logger"
)

// fakeWorkerQueue is a test double for workers.CrawlerQueueService.
// Only UpdateHeartbeat is exercised by the heartbeat tests; other methods are no-ops.
type fakeWorkerQueue struct {
	calls atomic.Int64
	err   error
}

func (f *fakeWorkerQueue) ClaimNext(_ context.Context, _ string) (*workers.ScanQueueItem, error) {
	return nil, nil
}
func (f *fakeWorkerQueue) Complete(_ context.Context, _ string) error { return nil }
func (f *fakeWorkerQueue) UpdateHeartbeat(_ context.Context, _ string) error {
	f.calls.Add(1)
	return f.err
}

var _ workers.CrawlerQueueService = (*fakeWorkerQueue)(nil)

// startHeartbeatViaWorker is a test helper that creates a BaseWorker and exercises
// its heartbeat goroutine. Heartbeat is now owned by workers.BaseWorker.
func startHeartbeatViaWorker(ctx context.Context, q workers.CrawlerQueueService, id string, interval time.Duration, log logger.Logger) func() {
	w := workers.NewBaseWorkerWithHeartbeat("test", q, nil, log, time.Second, interval)
	return w.StartHeartbeatForTest(ctx, id)
}

func TestStartHeartbeat_CallsUpdatePeriodically(t *testing.T) {
	q := &fakeWorkerQueue{}
	stop := startHeartbeatViaWorker(context.Background(), q, "item-1", 20*time.Millisecond, logger.NewNoop())

	time.Sleep(75 * time.Millisecond)
	stop()

	if got := q.calls.Load(); got < 2 {
		t.Errorf("expected at least 2 heartbeat calls in 75ms with 20ms interval, got %d", got)
	}
}

func TestStartHeartbeat_StopsAfterStopCalled(t *testing.T) {
	q := &fakeWorkerQueue{}
	stop := startHeartbeatViaWorker(context.Background(), q, "item-1", 20*time.Millisecond, logger.NewNoop())

	time.Sleep(50 * time.Millisecond)
	stop()
	countAtStop := q.calls.Load()

	time.Sleep(60 * time.Millisecond)
	if q.calls.Load() != countAtStop {
		t.Errorf("heartbeat continued after stop: got %d calls after stop, expected %d", q.calls.Load(), countAtStop)
	}
}

func TestStartHeartbeat_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	q := &fakeWorkerQueue{}
	_ = startHeartbeatViaWorker(ctx, q, "item-1", 20*time.Millisecond, logger.NewNoop())

	time.Sleep(50 * time.Millisecond)
	cancel()
	countAtCancel := q.calls.Load()

	time.Sleep(60 * time.Millisecond)
	if q.calls.Load() != countAtCancel {
		t.Errorf("heartbeat continued after context cancel: got %d calls after cancel, expected %d", q.calls.Load(), countAtCancel)
	}
}

func TestStartHeartbeat_StopIsIdempotent(t *testing.T) {
	q := &fakeWorkerQueue{}
	stop := startHeartbeatViaWorker(context.Background(), q, "item-1", 20*time.Millisecond, logger.NewNoop())

	stop()
	stop() // second call must not panic
}

func TestStartHeartbeat_DoesNotBlockOnUpdateError(t *testing.T) {
	q := &fakeWorkerQueue{err: context.DeadlineExceeded}
	stop := startHeartbeatViaWorker(context.Background(), q, "item-1", 10*time.Millisecond, logger.NewNoop())
	defer stop()

	time.Sleep(200 * time.Millisecond)
	if q.calls.Load() < 2 {
		t.Error("expected heartbeat to keep ticking despite UpdateHeartbeat errors")
	}
}
