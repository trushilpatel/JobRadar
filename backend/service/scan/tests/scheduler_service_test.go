package scan

import (
	"context"
	"errors"
	"testing"
	"time"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/scan/dto"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// schedulerFakeQueue is a focused queue store double for scheduler tests.
// It records arguments to confirm the scheduler binds the correct source
// constant and forwards interval/duration unchanged.
type schedulerFakeQueue struct {
	enqueueDueRes svc.EnqueueDueResult
	purgeCount    int
	resetCount    int
	err           error

	enqueueDueArgs struct {
		intervalMinutes int
		source          string
	}
	resetStuckArg time.Duration
}

func (f *schedulerFakeQueue) Enqueue(context.Context, string, string) error { return nil }
func (f *schedulerFakeQueue) EnqueueDue(_ context.Context, m int, src string) (svc.EnqueueDueResult, error) {
	f.enqueueDueArgs.intervalMinutes = m
	f.enqueueDueArgs.source = src
	return f.enqueueDueRes, f.err
}
func (f *schedulerFakeQueue) EnqueueAll(context.Context, string) (int, error) { return 0, nil }
func (f *schedulerFakeQueue) ClaimNext(context.Context, string) (*dto.ScanQueueItem, error) {
	return nil, nil
}
func (f *schedulerFakeQueue) Complete(context.Context, string) error { return nil }
func (f *schedulerFakeQueue) Remove(context.Context, string) error   { return nil }
func (f *schedulerFakeQueue) ResetStuck(_ context.Context, d time.Duration) (int, error) {
	f.resetStuckArg = d
	return f.resetCount, f.err
}
func (f *schedulerFakeQueue) PurgeInactive(context.Context) (int, error) {
	return f.purgeCount, f.err
}
func (f *schedulerFakeQueue) UpdateHeartbeat(context.Context, string) error { return nil }
func (f *schedulerFakeQueue) ListQueue(context.Context, dto.QueueFilter) ([]dto.ScanQueueItem, error) {
	return nil, nil
}

var _ svc.ScanQueueStore = (*schedulerFakeQueue)(nil)

// TestSchedulerService_EnqueueDue_OK asserts the scheduler service forwards
// the interval and emits QueueSourceScheduler — never QueueSourceManual.
// Confirms result mapping copies both fields.
func TestSchedulerService_EnqueueDue_OK(t *testing.T) {
	q := &schedulerFakeQueue{enqueueDueRes: svc.EnqueueDueResult{Enqueued: 4, SkippedNoSettings: 1}}
	s := svc.NewSchedulerService(q)

	res, err := s.EnqueueDue(context.Background(), 30)
	if err != nil {
		t.Fatalf("EnqueueDue() error = %v", err)
	}
	if res.Enqueued != 4 || res.SkippedNoSettings != 1 {
		t.Errorf("unexpected result: %+v", res)
	}
	if q.enqueueDueArgs.intervalMinutes != 30 {
		t.Errorf("expected interval 30, got %d", q.enqueueDueArgs.intervalMinutes)
	}
	if q.enqueueDueArgs.source != "scheduler" {
		t.Errorf("expected source 'scheduler', got %q", q.enqueueDueArgs.source)
	}
}

// TestSchedulerService_EnqueueDue_StoreError asserts raw store errors are
// wrapped as ErrInternal so the operator never sees the underlying DB
// driver text.
func TestSchedulerService_EnqueueDue_StoreError(t *testing.T) {
	q := &schedulerFakeQueue{err: errors.New("db down")}
	s := svc.NewSchedulerService(q)

	_, err := s.EnqueueDue(context.Background(), 60)
	assertInternal(t, err)
}

func TestSchedulerService_ResetStuck_OK(t *testing.T) {
	q := &schedulerFakeQueue{resetCount: 2}
	s := svc.NewSchedulerService(q)

	n, err := s.ResetStuck(context.Background(), 90*time.Second)
	if err != nil {
		t.Fatalf("ResetStuck() error = %v", err)
	}
	if n != 2 {
		t.Errorf("expected reset count 2, got %d", n)
	}
	if q.resetStuckArg != 90*time.Second {
		t.Errorf("expected 90s, got %v", q.resetStuckArg)
	}
}

func TestSchedulerService_ResetStuck_ZeroCount_OK(t *testing.T) {
	q := &schedulerFakeQueue{resetCount: 0}
	s := svc.NewSchedulerService(q)

	n, err := s.ResetStuck(context.Background(), 30*time.Second)
	if err != nil {
		t.Fatalf("ResetStuck() zero-count error = %v", err)
	}
	if n != 0 {
		t.Errorf("expected reset count 0, got %d", n)
	}
}

func TestSchedulerService_ResetStuck_StoreError(t *testing.T) {
	q := &schedulerFakeQueue{err: errors.New("db down")}
	s := svc.NewSchedulerService(q)

	_, err := s.ResetStuck(context.Background(), 30*time.Second)
	assertInternal(t, err)
}

func TestSchedulerService_PurgeInactive_OK(t *testing.T) {
	q := &schedulerFakeQueue{purgeCount: 7}
	s := svc.NewSchedulerService(q)

	n, err := s.PurgeInactive(context.Background())
	if err != nil {
		t.Fatalf("PurgeInactive() error = %v", err)
	}
	if n != 7 {
		t.Errorf("expected 7 purged, got %d", n)
	}
}

func TestSchedulerService_PurgeInactive_StoreError(t *testing.T) {
	q := &schedulerFakeQueue{err: errors.New("db down")}
	s := svc.NewSchedulerService(q)

	_, err := s.PurgeInactive(context.Background())
	assertInternal(t, err)
}

// assertInternal fails the test if err is not a wrapped *ServiceError with
// code INTERNAL. Used by both scheduler and crawler-queue tests below.
func assertInternal(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL, got %v", err)
	}
}
