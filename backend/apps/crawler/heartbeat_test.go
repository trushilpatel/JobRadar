package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/service/scan/dto"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

// fakeQueue is a test double for scansvc.CrawlerQueueService.
// Only UpdateHeartbeat is exercised by the heartbeat tests; other methods are no-ops.
type fakeQueue struct {
	calls atomic.Int64
	err   error
}

func (f *fakeQueue) ClaimNext(_ context.Context) (*dto.ScanQueueItem, error) { return nil, nil }
func (f *fakeQueue) Complete(_ context.Context, _ string) error              { return nil }
func (f *fakeQueue) ResetStuck(_ context.Context, _ time.Duration) error     { return nil }
func (f *fakeQueue) UpdateHeartbeat(_ context.Context, _ string) error {
	f.calls.Add(1)
	return f.err
}

var _ scansvc.CrawlerQueueService = (*fakeQueue)(nil)

func TestStartHeartbeat_CallsUpdatePeriodically(t *testing.T) {
	q := &fakeQueue{}
	stop := startHeartbeat(context.Background(), q, "item-1", 20*time.Millisecond, logger.NewNoop())

	time.Sleep(75 * time.Millisecond)
	stop()

	if got := q.calls.Load(); got < 2 {
		t.Errorf("expected at least 2 heartbeat calls in 75ms with 20ms interval, got %d", got)
	}
}

func TestStartHeartbeat_StopsAfterStopCalled(t *testing.T) {
	q := &fakeQueue{}
	stop := startHeartbeat(context.Background(), q, "item-1", 20*time.Millisecond, logger.NewNoop())

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
	q := &fakeQueue{}
	_ = startHeartbeat(ctx, q, "item-1", 20*time.Millisecond, logger.NewNoop())

	time.Sleep(50 * time.Millisecond)
	cancel()
	countAtCancel := q.calls.Load()

	time.Sleep(60 * time.Millisecond)
	if q.calls.Load() != countAtCancel {
		t.Errorf("heartbeat continued after context cancel: got %d calls after cancel, expected %d", q.calls.Load(), countAtCancel)
	}
}

func TestStartHeartbeat_StopIsIdempotent(t *testing.T) {
	q := &fakeQueue{}
	stop := startHeartbeat(context.Background(), q, "item-1", 20*time.Millisecond, logger.NewNoop())

	stop()
	stop() // second call must not panic
}

func TestStartHeartbeat_DoesNotBlockOnUpdateError(t *testing.T) {
	q := &fakeQueue{err: context.DeadlineExceeded}
	stop := startHeartbeat(context.Background(), q, "item-1", 10*time.Millisecond, logger.NewNoop())
	defer stop()

	time.Sleep(50 * time.Millisecond)
	if q.calls.Load() < 2 {
		t.Error("expected heartbeat to keep ticking despite UpdateHeartbeat errors")
	}
}
