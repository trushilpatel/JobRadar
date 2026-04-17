package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/infra/logger"
)

type fakeHeartbeater struct {
	calls atomic.Int64
	err   error
}

func (f *fakeHeartbeater) UpdateHeartbeat(_ context.Context, _ string) error {
	f.calls.Add(1)
	return f.err
}

func TestStartHeartbeat_CallsUpdatePeriodically(t *testing.T) {
	q := &fakeHeartbeater{}
	stop := startHeartbeat(context.Background(), q, "item-1", 20*time.Millisecond, logger.NewNoop())

	time.Sleep(75 * time.Millisecond)
	stop()

	got := q.calls.Load()
	if got < 2 {
		t.Errorf("expected at least 2 heartbeat calls in 75ms with 20ms interval, got %d", got)
	}
}

func TestStartHeartbeat_StopsAfterStopCalled(t *testing.T) {
	q := &fakeHeartbeater{}
	stop := startHeartbeat(context.Background(), q, "item-1", 20*time.Millisecond, logger.NewNoop())

	time.Sleep(50 * time.Millisecond)
	stop()
	countAtStop := q.calls.Load()

	// Wait longer and confirm no more calls fire after stop.
	time.Sleep(60 * time.Millisecond)
	if q.calls.Load() != countAtStop {
		t.Errorf("heartbeat continued after stop: got %d calls after stop, expected %d", q.calls.Load(), countAtStop)
	}
}

func TestStartHeartbeat_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	q := &fakeHeartbeater{}
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
	q := &fakeHeartbeater{}
	stop := startHeartbeat(context.Background(), q, "item-1", 20*time.Millisecond, logger.NewNoop())

	stop()
	stop() // second call must not panic
}

func TestStartHeartbeat_DoesNotBlockOnUpdateError(t *testing.T) {
	q := &fakeHeartbeater{err: context.DeadlineExceeded}
	stop := startHeartbeat(context.Background(), q, "item-1", 10*time.Millisecond, logger.NewNoop())
	defer stop()

	// Heartbeat should keep ticking even when UpdateHeartbeat returns an error.
	time.Sleep(50 * time.Millisecond)
	if q.calls.Load() < 2 {
		t.Error("expected heartbeat to keep ticking despite UpdateHeartbeat errors")
	}
}
