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
	enqueueCount int
	enqueueErr   error
	resetErr     error
	resetCalled  bool
}

func (f *fakeSchedulerService) EnqueueDue(_ context.Context, _ int) (int, error) {
	return f.enqueueCount, f.enqueueErr
}
func (f *fakeSchedulerService) ResetStuck(_ context.Context, _ time.Duration) error {
	f.resetCalled = true
	return f.resetErr
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
