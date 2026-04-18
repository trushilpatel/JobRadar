package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/infra/logger"
	scansvc "github.com/trushilpatel/jobradar/service/scan"
)

// fakeScanQueueStore is a test double for scansvc.ScanQueueStore.
// EnqueueDue returns company IDs one at a time, then "" to signal no more due.
type fakeScanQueueStore struct {
	companyIDs []string
	idx        int
	err        error
	enqueued   []string
}

func (f *fakeScanQueueStore) EnqueueDue(_ context.Context, _ int, _ string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if f.idx >= len(f.companyIDs) {
		return "", nil
	}
	id := f.companyIDs[f.idx]
	f.idx++
	f.enqueued = append(f.enqueued, id)
	return id, nil
}
func (f *fakeScanQueueStore) Enqueue(_ context.Context, _, _ string) error { return f.err }
func (f *fakeScanQueueStore) ClaimNext(_ context.Context) (*scansvc.ScanQueueItem, error) {
	return nil, f.err
}
func (f *fakeScanQueueStore) Complete(_ context.Context, _ string) error          { return f.err }
func (f *fakeScanQueueStore) Remove(_ context.Context, _ string) error            { return f.err }
func (f *fakeScanQueueStore) MoveUp(_ context.Context, _ string) error            { return f.err }
func (f *fakeScanQueueStore) MoveDown(_ context.Context, _ string) error          { return f.err }
func (f *fakeScanQueueStore) ResetStuck(_ context.Context, _ time.Duration) error { return f.err }
func (f *fakeScanQueueStore) UpdateHeartbeat(_ context.Context, _ string) error   { return f.err }
func (f *fakeScanQueueStore) ListQueue(_ context.Context) ([]scansvc.ScanQueueItem, error) {
	return nil, f.err
}

var _ scansvc.ScanQueueStore = (*fakeScanQueueStore)(nil)

// ── tests ─────────────────────────────────────────────────────────────────────

func TestEnqueueDue_NoDueCompanies(t *testing.T) {
	q := &fakeScanQueueStore{}

	enqueuedue(context.Background(), logger.NewNoop(), q, 1440)

	if len(q.enqueued) != 0 {
		t.Errorf("expected no enqueues, got %d", len(q.enqueued))
	}
}

func TestEnqueueDue_EnqueuesAllDueCompanies(t *testing.T) {
	q := &fakeScanQueueStore{
		companyIDs: []string{"company-1", "company-2", "company-3"},
	}

	enqueuedue(context.Background(), logger.NewNoop(), q, 1440)

	if len(q.enqueued) != 3 {
		t.Fatalf("expected 3 enqueues, got %d", len(q.enqueued))
	}
	want := map[string]bool{"company-1": true, "company-2": true, "company-3": true}
	for _, id := range q.enqueued {
		if !want[id] {
			t.Errorf("unexpected company enqueued: %q", id)
		}
	}
}

func TestEnqueueDue_StoreError_StopsImmediately(t *testing.T) {
	q := &fakeScanQueueStore{err: errors.New("db down")}

	enqueuedue(context.Background(), logger.NewNoop(), q, 1440)

	if len(q.enqueued) != 0 {
		t.Errorf("expected no enqueues on store error, got %d", len(q.enqueued))
	}
}

func TestEnqueueDue_ReturnsCount(t *testing.T) {
	q := &fakeScanQueueStore{
		companyIDs: []string{"a", "b"},
	}

	n := enqueuedue(context.Background(), logger.NewNoop(), q, 1440)

	if n != 2 {
		t.Errorf("expected count=2, got %d", n)
	}
}

func TestEnqueueDue_CancelledContext_StopsMidLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Fake store with many IDs — cancel after the first one to verify early exit.
	q := &fakeScanQueueStore{
		companyIDs: []string{"c1", "c2", "c3", "c4", "c5"},
	}

	// Wrap EnqueueDue to cancel the context after the first successful call.
	original := q

	wrapping := &cancelAfterNStore{inner: original, cancelAfter: 1, cancel: cancel}

	n := enqueuedue(ctx, logger.NewNoop(), wrapping, 1440)

	// Should stop after 1 (the cancelled call) or 2 (cancel races the check).
	// The important invariant: must not process all 5.
	if n >= len(q.companyIDs) {
		t.Errorf("expected early exit on cancelled context, but got count=%d (all companies enqueued)", n)
	}
}

func TestResetStuck_StoreError_LogsAndContinues(t *testing.T) {
	q := &fakeScanQueueStore{err: errors.New("db down")}
	// Should not panic — error is logged and swallowed.
	resetStuck(context.Background(), logger.NewNoop(), q, 30*time.Minute)
}

// cancelAfterNStore wraps a fakeScanQueueStore and cancels the context after N enqueues.
type cancelAfterNStore struct {
	inner       *fakeScanQueueStore
	cancelAfter int
	cancel      context.CancelFunc
	count       int
}

func (c *cancelAfterNStore) EnqueueDue(ctx context.Context, interval int, source string) (string, error) {
	id, err := c.inner.EnqueueDue(ctx, interval, source)
	if id != "" {
		c.count++
		if c.count >= c.cancelAfter {
			c.cancel()
		}
	}
	return id, err
}
func (c *cancelAfterNStore) Enqueue(_ context.Context, _, _ string) error { return nil }
func (c *cancelAfterNStore) ClaimNext(_ context.Context) (*scansvc.ScanQueueItem, error) {
	return nil, nil
}
func (c *cancelAfterNStore) Complete(_ context.Context, _ string) error          { return nil }
func (c *cancelAfterNStore) Remove(_ context.Context, _ string) error            { return nil }
func (c *cancelAfterNStore) MoveUp(_ context.Context, _ string) error            { return nil }
func (c *cancelAfterNStore) MoveDown(_ context.Context, _ string) error          { return nil }
func (c *cancelAfterNStore) ResetStuck(_ context.Context, _ time.Duration) error { return nil }
func (c *cancelAfterNStore) UpdateHeartbeat(_ context.Context, _ string) error   { return nil }
func (c *cancelAfterNStore) ListQueue(_ context.Context) ([]scansvc.ScanQueueItem, error) {
	return nil, nil
}

var _ scansvc.ScanQueueStore = (*cancelAfterNStore)(nil)
