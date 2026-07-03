package main

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/logger"
)

// ── computeJitter ─────────────────────────────────────────────────────────────

// TestComputeJitter_WithinBound asserts that jitter is always strictly less
// than checkInterval/10 — this is the documented contract that keeps the
// effective tick interval inside `[check, check*1.1)`.
func TestComputeJitter_WithinBound(t *testing.T) {
	interval := 60 * time.Second
	bound := interval / 10
	src := rand.New(rand.NewSource(1))

	for i := 0; i < 1000; i++ {
		j := computeJitter(interval, src)
		if j < 0 || j >= bound {
			t.Fatalf("jitter %v out of [0, %v)", j, bound)
		}
	}
}

// TestComputeJitter_DeterministicWithSeededRand asserts that two calls with
// freshly seeded *rand.Rand instances using the same seed produce identical
// sequences. Without this, scheduler replicas could drift between restarts.
func TestComputeJitter_DeterministicWithSeededRand(t *testing.T) {
	a := rand.New(rand.NewSource(42))
	b := rand.New(rand.NewSource(42))

	for i := 0; i < 10; i++ {
		ja := computeJitter(60*time.Second, a)
		jb := computeJitter(60*time.Second, b)
		if ja != jb {
			t.Fatalf("seeded sources disagreed at iteration %d: %v vs %v", i, ja, jb)
		}
	}
}

// TestComputeJitter_NonPositiveIntervalReturnsZero covers the guard that
// prevents rand.Int63n from panicking on a zero or negative bound. Sub-10ns
// intervals do not appear in production but appear in tightly-driven tests.
func TestComputeJitter_NonPositiveIntervalReturnsZero(t *testing.T) {
	cases := []time.Duration{0, -1 * time.Second, 1 * time.Nanosecond, 9 * time.Nanosecond}
	for _, d := range cases {
		if got := computeJitter(d, rand.New(rand.NewSource(1))); got != 0 {
			t.Errorf("computeJitter(%v) = %v, want 0", d, got)
		}
	}
}

// TestComputeJitter_NilSourceFallsBackToGlobal asserts the helper does not
// panic when no rand.Rand is provided — the production code in main.go always
// supplies a source, but other callers should be safe.
func TestComputeJitter_NilSourceFallsBackToGlobal(t *testing.T) {
	got := computeJitter(60*time.Second, nil)
	if got < 0 || got >= 6*time.Second {
		t.Fatalf("nil-source jitter %v out of [0, 6s)", got)
	}
}

// ── Run loop ─────────────────────────────────────────────────────────────────

// TestRun_ExecutesImmediateCycle asserts Run runs one full cycle before any
// ticker can fire — this matches the documented "don't wait a full interval
// for the first scan" contract.
func TestRun_ExecutesImmediateCycle(t *testing.T) {
	svc := &fakeSchedulerService{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before Run starts the ticker loop

	Run(ctx, logger.NewNoop(), svc, 1440, 1*time.Hour, 30*time.Minute)

	if svc.enqueueCalls.Load() != 1 {
		t.Errorf("expected exactly 1 immediate EnqueueDue call, got %d", svc.enqueueCalls.Load())
	}
	if svc.resetCalls.Load() != 1 {
		t.Errorf("expected exactly 1 immediate ResetStuck call, got %d", svc.resetCalls.Load())
	}
	if svc.purgeCalls.Load() != 1 {
		t.Errorf("expected exactly 1 immediate PurgeInactive call, got %d", svc.purgeCalls.Load())
	}
}

// TestRun_TicksMultipleTimes asserts that with a small tick interval Run
// invokes runCycle more than once before ctx is cancelled, proving the
// ticker branch executes.
func TestRun_TicksMultipleTimes(t *testing.T) {
	// ready closes when EnqueueDue has been called at least twice
	// (1 immediate + ≥1 ticker-driven cycle).
	ready := make(chan struct{})
	svc := &fakeSchedulerService{readyAt: 2, readyCh: ready}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		Run(ctx, logger.NewNoop(), svc, 1440, 10*time.Millisecond, 30*time.Minute)
		close(done)
	}()

	select {
	case <-ready:
		// condition met — cancel and wait for Run to exit
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not execute ≥2 cycles within 2s")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}

	// 1 immediate + ≥1 ticker-driven cycle.
	if got := svc.enqueueCalls.Load(); got < 2 {
		t.Errorf("expected at least 2 EnqueueDue calls (immediate + ticker), got %d", got)
	}
}

// TestRun_ReturnsOnContextCancellation_FromTickerLoop asserts the loop
// exits promptly when ctx is cancelled while it is parked on a select.
func TestRun_ReturnsOnContextCancellation_FromTickerLoop(t *testing.T) {
	// ready closes after the immediate cycle's EnqueueDue call, confirming
	// Run has entered the ticker select before we cancel the context.
	ready := make(chan struct{})
	svc := &fakeSchedulerService{readyAt: 1, readyCh: ready}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		// Long tick interval so the goroutine sits on the ctx.Done branch.
		Run(ctx, logger.NewNoop(), svc, 1440, 1*time.Hour, 30*time.Minute)
		close(done)
	}()

	// Wait for the immediate cycle to complete, then cancel.
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("immediate cycle did not complete within 2s")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s of context cancellation")
	}
}

// TestRun_NonPositiveTickIntervalReturnsAfterImmediateCycle exercises the
// defensive branch that prevents time.NewTicker from panicking on a zero or
// negative interval.
func TestRun_NonPositiveTickIntervalReturnsAfterImmediateCycle(t *testing.T) {
	cases := []time.Duration{0, -1 * time.Second}
	for _, d := range cases {
		svc := &fakeSchedulerService{}
		Run(context.Background(), logger.NewNoop(), svc, 1440, d, 30*time.Minute)
		if got := svc.enqueueCalls.Load(); got != 1 {
			t.Errorf("tickInterval=%v: expected exactly 1 cycle, got %d", d, got)
		}
	}
}

// TestRun_PassesArguments asserts Run forwards scanIntervalMinutes and
// stuckTimeout down to the cycle on every tick.
func TestRun_PassesArguments(t *testing.T) {
	svc := &fakeSchedulerService{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	Run(ctx, logger.NewNoop(), svc, 720, 1*time.Hour, 45*time.Minute)

	if svc.enqueueDueArgs != 720 {
		t.Errorf("expected EnqueueDue to receive 720, got %d", svc.enqueueDueArgs)
	}
	if svc.resetStuckArg != 45*time.Minute {
		t.Errorf("expected ResetStuck to receive 45m, got %v", svc.resetStuckArg)
	}
}

// TestRun_ConcurrentSafe runs Run in two goroutines to ensure the helper has
// no package-level shared state that would race under -race. The fake's
// internal mutex is part of the test fixture, not the system under test.
func TestRun_ConcurrentSafe(t *testing.T) {
	if testing.Short() {
		t.Skip("skip concurrent race smoke under -short")
	}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// ready closes after the first two EnqueueDue calls (immediate +
			// one ticker cycle), then we cancel so the goroutine exits cleanly.
			ready := make(chan struct{})
			fakeSvc := &fakeSchedulerService{readyAt: 2, readyCh: ready}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() {
				select {
				case <-ready:
				case <-time.After(2 * time.Second):
				}
				cancel()
			}()
			Run(ctx, logger.NewNoop(), fakeSvc, 1440, 5*time.Millisecond, 30*time.Minute)
		}()
	}
	wg.Wait()
}

// ── runCycle additional coverage ─────────────────────────────────────────────

// TestRunCycle_PurgedRowsLoggedAtInfo covers the branch where PurgeInactive
// returns a positive count — previously uncovered.
func TestRunCycle_PurgedRowsLoggedAtInfo(t *testing.T) {
	svc := &fakeSchedulerService{purgeCount: 7}
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)

	if !svc.purgeCalled() {
		t.Error("expected PurgeInactive to be called")
	}
	if svc.purgeCalls.Load() != 1 {
		t.Errorf("expected PurgeInactive call count = 1, got %d", svc.purgeCalls.Load())
	}
}

// TestRunCycle_ForwardsStuckTimeout asserts the stuck-timeout argument is
// threaded through to ResetStuck unchanged.
func TestRunCycle_ForwardsStuckTimeout(t *testing.T) {
	svc := &fakeSchedulerService{}
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 12*time.Minute)
	if svc.resetStuckArg != 12*time.Minute {
		t.Errorf("ResetStuck got %v, want 12m", svc.resetStuckArg)
	}
}

// TestRunCycle_AllStepsRun_OnAllErrors covers fault-tolerance: every
// downstream call fails, but the cycle still touches every step (no early
// return on the first failure).
func TestRunCycle_AllStepsRun_OnAllErrors(t *testing.T) {
	svc := &fakeSchedulerService{
		resetErr:   errors.New("reset down"),
		purgeErr:   errors.New("purge down"),
		enqueueErr: errors.New("enqueue down"),
	}
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)

	if svc.resetCalls.Load() != 1 {
		t.Errorf("ResetStuck calls = %d, want 1", svc.resetCalls.Load())
	}
	if svc.purgeCalls.Load() != 1 {
		t.Errorf("PurgeInactive calls = %d, want 1", svc.purgeCalls.Load())
	}
	if svc.enqueueCalls.Load() != 1 {
		t.Errorf("EnqueueDue calls = %d, want 1", svc.enqueueCalls.Load())
	}
}

// ── RC-7: EscalateStuckRetryJobs in runCycle ─────────────────────────────────

// TestRunCycle_EscalateStuckRetryJobs_CalledEachCycle asserts that
// EscalateStuckRetryJobs is invoked in every runCycle pass.
func TestRunCycle_EscalateStuckRetryJobs_CalledEachCycle(t *testing.T) {
	svc := &fakeSchedulerService{}
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)
	if svc.escalateStuckCalls.Load() != 1 {
		t.Errorf("EscalateStuckRetryJobs calls = %d, want 1", svc.escalateStuckCalls.Load())
	}
}

// TestRunCycle_EscalateStuckRetryJobs_ErrorContinues asserts that an error from
// EscalateStuckRetryJobs does not abort subsequent steps in runCycle.
func TestRunCycle_EscalateStuckRetryJobs_ErrorContinues(t *testing.T) {
	svc := &fakeSchedulerService{
		escalateStuckErr: errors.New("db down"),
		enqueueCount:     1,
	}
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)

	if svc.escalateStuckCalls.Load() != 1 {
		t.Errorf("EscalateStuckRetryJobs calls = %d, want 1", svc.escalateStuckCalls.Load())
	}
	if svc.enqueueCalls.Load() != 1 {
		t.Errorf("EnqueueDue calls = %d, want 1 (must run even after escalateStuck error)", svc.enqueueCalls.Load())
	}
}

// TestRunCycle_ReclaimStuckFetchRows_CalledEachCycle asserts that
// ReclaimStuckFetchRows is invoked in every runCycle pass.
func TestRunCycle_ReclaimStuckFetchRows_CalledEachCycle(t *testing.T) {
	svc := &fakeSchedulerService{}
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)
	if svc.reclaimStuckCalls.Load() != 1 {
		t.Errorf("ReclaimStuckFetchRows calls = %d, want 1", svc.reclaimStuckCalls.Load())
	}
}

// TestRunCycle_ReclaimStuckFetchRows_ErrorContinues asserts that an error from
// ReclaimStuckFetchRows does not abort subsequent steps in runCycle.
func TestRunCycle_ReclaimStuckFetchRows_ErrorContinues(t *testing.T) {
	svc := &fakeSchedulerService{
		reclaimStuckErr: errors.New("db down"),
		enqueueCount:    1,
	}
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)

	if svc.reclaimStuckCalls.Load() != 1 {
		t.Errorf("ReclaimStuckFetchRows calls = %d, want 1", svc.reclaimStuckCalls.Load())
	}
	if svc.enqueueCalls.Load() != 1 {
		t.Errorf("EnqueueDue calls = %d, want 1 (must run even after reclaimStuck error)", svc.enqueueCalls.Load())
	}
}

// TestRunCycle_ReclaimStuckFetchRows_LogsOnReclaim exercises the n>0 INFO-log
// branch of the reclaim step.
func TestRunCycle_ReclaimStuckFetchRows_LogsOnReclaim(t *testing.T) {
	svc := &fakeSchedulerService{reclaimStuckN: 7}
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)
	if svc.reclaimStuckCalls.Load() != 1 {
		t.Errorf("ReclaimStuckFetchRows calls = %d, want 1", svc.reclaimStuckCalls.Load())
	}
}

// TestRunCycle_AutoPause_CalledEachCycle asserts the auto-pause sweep runs in
// every runCycle pass.
func TestRunCycle_AutoPause_CalledEachCycle(t *testing.T) {
	svc := &fakeSchedulerService{}
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)
	if svc.autoPauseCalls.Load() != 1 {
		t.Errorf("AutoPauseChronicFailures calls = %d, want 1", svc.autoPauseCalls.Load())
	}
}

// TestRunCycle_AutoPause_ErrorContinues asserts an auto-pause error does not
// abort subsequent steps (EnqueueDue still runs).
func TestRunCycle_AutoPause_ErrorContinues(t *testing.T) {
	svc := &fakeSchedulerService{
		autoPauseErr: errors.New("db down"),
		enqueueCount: 1,
	}
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)

	if svc.autoPauseCalls.Load() != 1 {
		t.Errorf("AutoPauseChronicFailures calls = %d, want 1", svc.autoPauseCalls.Load())
	}
	if svc.enqueueCalls.Load() != 1 {
		t.Errorf("EnqueueDue calls = %d, want 1 (must run even after auto-pause error)", svc.enqueueCalls.Load())
	}
}

// TestRunCycle_AutoPause_LogsOnPause exercises the n>0 INFO-log branch of the
// auto-pause step.
func TestRunCycle_AutoPause_LogsOnPause(t *testing.T) {
	svc := &fakeSchedulerService{autoPauseN: 4}
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)
	if svc.autoPauseCalls.Load() != 1 {
		t.Errorf("AutoPauseChronicFailures calls = %d, want 1", svc.autoPauseCalls.Load())
	}
}

// TestRunCycle_RefreshesGaugeMetrics asserts RefreshGaugeMetrics is invoked
// in every runCycle pass.
func TestRunCycle_RefreshesGaugeMetrics(t *testing.T) {
	svc := &fakeSchedulerService{}
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)

	if svc.refreshGaugeCalls.Load() != 1 {
		t.Errorf("RefreshGaugeMetrics calls = %d, want 1", svc.refreshGaugeCalls.Load())
	}
}

// TestRunCycle_RefreshGaugeMetricsError_ContinuesToEnqueue asserts a
// gauge-refresh failure does not abort the cycle — enqueue still runs after.
func TestRunCycle_RefreshGaugeMetricsError_ContinuesToEnqueue(t *testing.T) {
	svc := &fakeSchedulerService{refreshGaugeErr: errors.New("db down"), enqueueCount: 2}
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)

	if svc.enqueueCalls.Load() != 1 {
		t.Errorf("EnqueueDue calls = %d, want 1 (gauge-refresh failure must not block enqueue)", svc.enqueueCalls.Load())
	}
}


// ── AC-6: ReclaimStuckFilteredRows in runCycle ────────────────────────────────

// TestRunCycle_ReclaimStuckFilteredRows_CalledEachCycle asserts that
// ReclaimStuckFilteredRows is invoked in every runCycle pass.
func TestRunCycle_ReclaimStuckFilteredRows_CalledEachCycle(t *testing.T) {
	svc := &fakeSchedulerService{}
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)
	if svc.reclaimFilteredCalls.Load() != 1 {
		t.Errorf("ReclaimStuckFilteredRows calls = %d, want 1", svc.reclaimFilteredCalls.Load())
	}
}

// TestRunCycle_ReclaimStuckFilteredRows_ErrorContinues asserts that an error
// from ReclaimStuckFilteredRows does not abort subsequent steps in runCycle.
func TestRunCycle_ReclaimStuckFilteredRows_ErrorContinues(t *testing.T) {
	svc := &fakeSchedulerService{
		reclaimFilteredErr: errors.New("db down"),
		enqueueCount:       1,
	}
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)

	if svc.reclaimFilteredCalls.Load() != 1 {
		t.Errorf("ReclaimStuckFilteredRows calls = %d, want 1", svc.reclaimFilteredCalls.Load())
	}
	if svc.enqueueCalls.Load() != 1 {
		t.Errorf("EnqueueDue calls = %d, want 1 (must run even after reclaimFiltered error)", svc.enqueueCalls.Load())
	}
}

// TestRunCycle_ReclaimStuckFilteredRows_LogsOnReclaim exercises the n>0
// INFO-log branch of the reclaim-filtered step.
func TestRunCycle_ReclaimStuckFilteredRows_LogsOnReclaim(t *testing.T) {
	svc := &fakeSchedulerService{reclaimFilteredN: 5}
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)
	if svc.reclaimFilteredCalls.Load() != 1 {
		t.Errorf("ReclaimStuckFilteredRows calls = %d, want 1", svc.reclaimFilteredCalls.Load())
	}
}
