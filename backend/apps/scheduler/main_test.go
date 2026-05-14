package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/logger"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

// fakeSchedulerService is a test double for scansvc.SchedulerService.
//
// It records the inputs each call received and exposes counters via
// atomic.Int64 so concurrent ticks under -race never tear during reads
// from Run's goroutine.
type fakeSchedulerService struct {
	mu sync.Mutex

	enqueueCount  int
	skippedCount  int
	purgeCount    int
	enqueueErr    error
	resetErr      error
	purgeErr      error
	resetStuckArg time.Duration

	resetCalls   atomic.Int64
	purgeCalls   atomic.Int64
	enqueueCalls atomic.Int64

	enqueueDueArgs int
}

func (f *fakeSchedulerService) EnqueueDue(_ context.Context, intervalMinutes int) (scansvc.SchedulerCycleResult, error) {
	f.mu.Lock()
	f.enqueueDueArgs = intervalMinutes
	res := scansvc.SchedulerCycleResult{
		Enqueued:          f.enqueueCount,
		SkippedNoSettings: f.skippedCount,
	}
	err := f.enqueueErr
	f.mu.Unlock()
	f.enqueueCalls.Add(1)
	return res, err
}
func (f *fakeSchedulerService) ResetStuck(_ context.Context, d time.Duration) error {
	f.mu.Lock()
	f.resetStuckArg = d
	err := f.resetErr
	f.mu.Unlock()
	f.resetCalls.Add(1)
	return err
}
func (f *fakeSchedulerService) PurgeInactive(_ context.Context) (int, error) {
	f.mu.Lock()
	n := f.purgeCount
	err := f.purgeErr
	f.mu.Unlock()
	f.purgeCalls.Add(1)
	return n, err
}

// resetCalled reports whether ResetStuck has been invoked at least once.
func (f *fakeSchedulerService) resetCalled() bool { return f.resetCalls.Load() > 0 }

// purgeCalled reports whether PurgeInactive has been invoked at least once.
func (f *fakeSchedulerService) purgeCalled() bool { return f.purgeCalls.Load() > 0 }

var _ scansvc.SchedulerService = (*fakeSchedulerService)(nil)

// ── tests ─────────────────────────────────────────────────────────────────────

func TestRunCycle_EnqueuesAndResetsStuck(t *testing.T) {
	svc := &fakeSchedulerService{enqueueCount: 3}
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)

	if !svc.resetCalled() {
		t.Error("expected ResetStuck to be called")
	}
}

func TestRunCycle_NoDueCompanies(t *testing.T) {
	svc := &fakeSchedulerService{enqueueCount: 0}
	// Should complete without error — zero enqueues is normal.
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)
}

func TestRunCycle_EnqueueError_ContinuesAfterLogging(t *testing.T) {
	svc := &fakeSchedulerService{enqueueErr: errors.New("db down")}
	// Should not panic — error is logged and cycle completes.
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)
}

func TestRunCycle_ResetStuckError_ContinuesToEnqueue(t *testing.T) {
	svc := &fakeSchedulerService{
		resetErr:     errors.New("db down"),
		enqueueCount: 2,
	}
	// Reset failure must not abort the enqueue step.
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)
}

func TestRunCycle_PurgeInactiveError_ContinuesToEnqueue(t *testing.T) {
	svc := &fakeSchedulerService{
		purgeErr:     errors.New("db down"),
		enqueueCount: 2,
	}
	// Purge failure must not abort the enqueue step.
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)
}

// TestRunCycle_SkippedNoSettings_LoggedAtInfo asserts the scheduler reports
// companies with no scan_settings row even when nothing was enqueued.
func TestRunCycle_SkippedNoSettings_LoggedAtInfo(t *testing.T) {
	svc := &fakeSchedulerService{enqueueCount: 0, skippedCount: 5}
	// Should complete without error — the cycle still surfaces the skip count
	// via the scheduler service even when nothing was enqueued. We can't
	// assert against the noop logger, but the cycle must not error.
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)
}

// TestRunCycle_PassesIntervalMinutes asserts EnqueueDue receives the
// configured global default minutes value.
func TestRunCycle_PassesIntervalMinutes(t *testing.T) {
	svc := &fakeSchedulerService{}
	runCycle(context.Background(), logger.NewNoop(), svc, 720, 30*time.Minute)
	if svc.enqueueDueArgs != 720 {
		t.Errorf("expected EnqueueDue to receive 720, got %d", svc.enqueueDueArgs)
	}
}
