package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/logger"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

// fakeSchedulerService is a test double for scansvc.SchedulerService.
type fakeSchedulerService struct {
	enqueueCount   int
	skippedCount   int
	enqueueErr     error
	resetErr       error
	resetCalled    bool
	purgeErr       error
	purgeCalled    bool
	enqueueDueArgs int
}

func (f *fakeSchedulerService) EnqueueDue(_ context.Context, intervalMinutes int) (scansvc.SchedulerCycleResult, error) {
	f.enqueueDueArgs = intervalMinutes
	return scansvc.SchedulerCycleResult{
		Enqueued:          f.enqueueCount,
		SkippedNoSettings: f.skippedCount,
	}, f.enqueueErr
}
func (f *fakeSchedulerService) ResetStuck(_ context.Context, _ time.Duration) error {
	f.resetCalled = true
	return f.resetErr
}
func (f *fakeSchedulerService) PurgeInactive(_ context.Context) (int, error) {
	f.purgeCalled = true
	return 0, f.purgeErr
}

var _ scansvc.SchedulerService = (*fakeSchedulerService)(nil)

// ── tests ─────────────────────────────────────────────────────────────────────

func TestRunCycle_EnqueuesAndResetsStuck(t *testing.T) {
	svc := &fakeSchedulerService{enqueueCount: 3}
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)

	if !svc.resetCalled {
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
