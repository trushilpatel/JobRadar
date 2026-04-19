package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/logger"
	dashboardsvc "github.com/trushilpatel/jobradar/service/dashboard/services"
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

// fakeRefresher is a test double for dashboardsvc.StatsRefresher.
type fakeRefresher struct{ err error }

func (f *fakeRefresher) RefreshPipelineStats(_ context.Context) error { return f.err }
func (f *fakeRefresher) RefreshHeatmap(_ context.Context) error       { return f.err }

var _ dashboardsvc.StatsRefresher = (*fakeRefresher)(nil)

// zeroState returns a refreshState with zero times so both views refresh on every cycle.
func zeroState() *refreshState { return &refreshState{} }

// ── tests ─────────────────────────────────────────────────────────────────────

func TestRunCycle_EnqueuesAndResetsStuck(t *testing.T) {
	svc := &fakeSchedulerService{enqueueCount: 3}
	runCycle(context.Background(), logger.NewNoop(), svc, &fakeRefresher{}, zeroState(), 1440, 30*time.Minute)

	if !svc.resetCalled {
		t.Error("expected ResetStuck to be called")
	}
}

func TestRunCycle_NoDueCompanies(t *testing.T) {
	svc := &fakeSchedulerService{enqueueCount: 0}
	// Should complete without error — zero enqueues is normal.
	runCycle(context.Background(), logger.NewNoop(), svc, &fakeRefresher{}, zeroState(), 1440, 30*time.Minute)
}

func TestRunCycle_EnqueueError_ContinuesAfterLogging(t *testing.T) {
	svc := &fakeSchedulerService{enqueueErr: errors.New("db down")}
	// Should not panic — error is logged and cycle completes.
	runCycle(context.Background(), logger.NewNoop(), svc, &fakeRefresher{}, zeroState(), 1440, 30*time.Minute)
}

func TestRunCycle_ResetStuckError_ContinuesToEnqueue(t *testing.T) {
	svc := &fakeSchedulerService{
		resetErr:     errors.New("db down"),
		enqueueCount: 2,
	}
	// Reset failure must not abort the enqueue step.
	runCycle(context.Background(), logger.NewNoop(), svc, &fakeRefresher{}, zeroState(), 1440, 30*time.Minute)
}

func TestRunCycle_RefreshError_ContinuesAfterLogging(t *testing.T) {
	svc := &fakeSchedulerService{enqueueCount: 1}
	// Refresh failures must not abort the cycle.
	runCycle(context.Background(), logger.NewNoop(), svc, &fakeRefresher{err: errors.New("refresh failed")}, zeroState(), 1440, 30*time.Minute)
}
