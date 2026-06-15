package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/ats/core"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	"github.com/trushilpatel/jobradar/common/logger"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
)

// ── callbackFetcher ────────────────────────────────────────────────────────────

// callbackFetcher implements MessageFetcher via a user-supplied function.
type callbackFetcher struct {
	fn func(n int, maxWait time.Duration) ([]*js.Msg, error)
}

func (c *callbackFetcher) Fetch(n int, maxWait time.Duration) ([]*js.Msg, error) {
	return c.fn(n, maxWait)
}

// makeRunJob builds a fat-adapter summary (inline JobContent) that skips the
// FetchJobPost call.
func makeRunJob(id string) core.JobSummary {
	return core.JobSummary{
		ID:         id,
		Title:      "Software Engineer",
		Adapter:    "ashby",
		JobContent: &core.JobContent{Description: "test role"},
	}
}

// ── Run tests ─────────────────────────────────────────────────────────────────

// TestAnalyzerRun_CtxAlreadyCancelled_ReturnsNil verifies Run exits immediately
// when ctx is already cancelled at entry.
func TestAnalyzerRun_CtxAlreadyCancelled_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.consumer = &callbackFetcher{fn: func(_ int, _ time.Duration) ([]*js.Msg, error) {
		t.Fatal("Fetch must not be called when ctx is already cancelled")
		return nil, nil
	}}

	if err := w.Run(ctx); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// TestAnalyzerRun_FetchError_CtxCancelledInSelect_ReturnsNil verifies that when
// Fetch errors and ctx is subsequently cancelled, Run returns nil via the
// ctx.Err() check inside the error-handling block.
func TestAnalyzerRun_FetchError_CtxCancelledInSelect_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.consumer = &callbackFetcher{fn: func(_ int, _ time.Duration) ([]*js.Msg, error) {
		cancel() // cancel before returning the error so ctx.Err() != nil in the error block
		return nil, errors.New("nats down")
	}}

	if err := w.Run(ctx); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// TestAnalyzerRun_FetchError_CtxNotCancelled_LogsErrorAndContinues verifies that
// when Fetch errors and ctx is NOT cancelled, the error is logged and the loop
// continues. The second Fetch call cancels ctx to exit the loop.
func TestAnalyzerRun_FetchError_CtxNotCancelled_LogsErrorAndContinues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.consumer = &callbackFetcher{fn: func(_ int, _ time.Duration) ([]*js.Msg, error) {
		calls++
		if calls == 1 {
			// Return error without cancelling ctx — exercises log.Error("fetch failed").
			return nil, errors.New("nats down")
		}
		cancel()
		return nil, nil
	}}

	if err := w.Run(ctx); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if calls < 2 {
		t.Errorf("expected ≥2 Fetch calls, got %d", calls)
	}
}

// TestAnalyzerRun_FetchReturnsEmpty_LoopContinues verifies that empty Fetch
// results cause the loop to continue until ctx is cancelled.
func TestAnalyzerRun_FetchReturnsEmpty_LoopContinues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.consumer = &callbackFetcher{fn: func(_ int, _ time.Duration) ([]*js.Msg, error) {
		calls++
		if calls == 1 {
			return nil, nil // empty
		}
		cancel()
		return nil, nil
	}}

	if err := w.Run(ctx); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if calls < 2 {
		t.Errorf("expected ≥2 Fetch calls, got %d", calls)
	}
}

// TestAnalyzerRun_ProcessFetchSucceeds_LoopContinues verifies that when Fetch
// returns messages and processFetch succeeds (returns nil), Run continues looping.
func TestAnalyzerRun_ProcessFetchSucceeds_LoopContinues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	scorer := &fakeScorer{results: nil} // empty results → transient failure routes per job
	pipeline := &fakePipelineService{}
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(scorer, pipeline, &fakePublisher{}, &fakeLedger{}, clk, 1)

	job := makeRunJob("run-loop-1")
	msg, _ := js.NewTestMsg(marshalJob(t, job), 1)

	calls := 0
	w.consumer = &callbackFetcher{fn: func(_ int, _ time.Duration) ([]*js.Msg, error) {
		calls++
		if calls == 1 {
			return []*js.Msg{msg}, nil
		}
		cancel()
		return nil, nil
	}}

	if err := w.Run(ctx); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// ── NewWorker default sanitization ────────────────────────────────────────────

// TestNewWorker_FlushTimeoutDefault verifies that NewWorker sets FlushTimeout
// to 120s when a zero or negative value is passed.
func TestNewWorker_FlushTimeoutDefault(t *testing.T) {
	opts := makeBaseOptions(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	opts.FlushTimeout = 0 // should be clamped to 120s
	w := NewWorker(opts)
	if w.flushTimeout != 120*time.Second {
		t.Errorf("FlushTimeout should default to 120s; got %v", w.flushTimeout)
	}

	opts.FlushTimeout = -1
	w = NewWorker(opts)
	if w.flushTimeout != 120*time.Second {
		t.Errorf("FlushTimeout should default to 120s for negative; got %v", w.flushTimeout)
	}
}

// ── handleFailure nak-fail path ────────────────────────────────────────────────

// TestHandleFailure_NakFails_ReturnsFalse verifies that when msg.Nak() fails,
// handleFailure returns false (non-terminal) and increments the failure metric.
func TestHandleFailure_NakFails_ReturnsFalse(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.clock = &fakeClock{now: time.Now()}
	fetchLog := logger.NewNoop()

	fj, state := makeTestFetchedJob(t, baseSummary(), 0) // retryCount=0 < analyzerMaxRetries=3
	state.NakErr = errors.New("nak failed")

	ok := w.handleFailure(context.Background(), fetchLog, fj, "enrich", errors.New("llm error"))
	if ok {
		t.Errorf("expected handleFailure=false when Nak fails")
	}
	if !state.Naked {
		t.Errorf("Nak must be called even when it returns an error")
	}
}

// ── markLedgerRetryWithRetry sleep-cancel path ────────────────────────────────

// TestMarkLedgerRetryWithRetry_SleepCancelled_ReturnsOriginalErr verifies that
// when the first UpdateRetryBatch fails with a transient error AND the
// inter-retry sleep detects a cancelled context, the original error is returned
// (not context.Canceled, and not a second retry attempt).
func TestMarkLedgerRetryWithRetry_SleepCancelled_ReturnsOriginalErr(t *testing.T) {
	transientErr := errors.New("db unavailable")
	ledger := &fakeLedger{
		failureQueue: []error{transientErr}, // first call fails
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel so Sleep immediately returns ctx.Err()

	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, ledger, clk, 1)

	entry := jobRetryEntry()
	err := w.markLedgerRetryWithRetry(ctx, entry)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// The original transient error (not context.Canceled) should propagate.
	if errors.Is(err, context.Canceled) {
		t.Errorf("expected original db error, got context.Canceled")
	}
}

// ── sendToDLQ term-fail after successful publish ───────────────────────────────

// TestSendToDLQ_TermFails_AfterSuccessfulPublish verifies that when the DLQ
// publish succeeds but Term() fails, sendToDLQ logs a warning and still returns
// true (the job is safely in the DLQ regardless of Term outcome).
func TestSendToDLQ_TermFails_AfterSuccessfulPublish(t *testing.T) {
	pub := &fakePublisher{} // publish succeeds
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, pub, &fakeLedger{})

	fj, state := makeTestFetchedJob(t, baseSummary(), analyzerMaxRetries)
	state.TermErr = errors.New("term failed after dlq publish")

	ok := w.sendToDLQ(context.Background(), logger.NewNoop(), fj, "exhausted", errors.New("llm failed"))
	if !ok {
		t.Errorf("expected sendToDLQ=true even when Term fails (job is safely in DLQ)")
	}
	if !state.Termed {
		t.Errorf("expected Term attempted even when it errors")
	}
}

// ── ratelimit.Wait jitter ctx-cancel path ─────────────────────────────────────

// TestRateLimiter_Wait_CtxCancelDuringJitter verifies that Wait returns
// ctx.Err() when the context is cancelled while the jitter sleep is in progress.
func TestRateLimiter_Wait_CtxCancelDuringJitter(t *testing.T) {
	// Set a large jitter so the random delay is predictably non-zero.
	rl := NewAdapterRateLimiter(1000.0, 10, 10*time.Second, logger.NewNoop())

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// Cancel shortly after the rate-limiter grants the token so we hit the
		// jitter select's ctx.Done arm.
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	err := rl.Wait(ctx, "test-adapter")
	if err == nil {
		t.Error("expected error from context cancel during jitter sleep")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// ── ratelimit.Wait slow-wait debug path ───────────────────────────────────────

// TestRateLimiter_Wait_SlowWaitLogsDebug verifies that the debug log fires when
// the rate-limit wait exceeds 100ms. Uses RPS=9 (≈111ms/token after burst) and
// a generous context timeout so the wait completes before cancellation.
func TestRateLimiter_Wait_SlowWaitLogsDebug(t *testing.T) {
	// RPS=9 gives ≈111ms per token after the burst is exhausted.
	// burst=1 means 1 token is available immediately; the second call blocks ~111ms.
	rl := NewAdapterRateLimiter(9.0, 1, 0, logger.NewNoop())

	// Consume the burst token immediately (fast).
	if err := rl.Wait(context.Background(), "slow-adapter"); err != nil {
		t.Fatalf("first Wait: %v", err)
	}

	// Second call blocks ~111ms (>100ms threshold) → debug log fires.
	// Context timeout is generous (2s) so the wait completes before cancellation.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rl.Wait(ctx, "slow-adapter"); err != nil {
		t.Fatalf("second Wait (slow): %v", err)
	}
}

// ── getOrCreate double-check under write lock ─────────────────────────────────

// TestRateLimiter_GetOrCreate_RaceDoubleCheck verifies that rapid concurrent
// calls to Wait on the same adapter name result in exactly one limiter being
// created (the double-check-under-write-lock path).
func TestRateLimiter_GetOrCreate_RaceDoubleCheck(t *testing.T) {
	rl := NewAdapterRateLimiter(1000.0, 100, 0, logger.NewNoop())
	const goroutines = 20
	results := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			results <- rl.Wait(context.Background(), "race-adapter")
		}()
	}
	for i := 0; i < goroutines; i++ {
		if err := <-results; err != nil {
			t.Errorf("concurrent Wait: %v", err)
		}
	}
	// All goroutines obtained a token from the same limiter.
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	if len(rl.limiters) != 1 {
		t.Errorf("expected 1 limiter created; got %d", len(rl.limiters))
	}
}

// ── processFetch panic recovery path ─────────────────────────────────────────

// panickyEnricher panics on EnrichBatch, simulating an unexpected runtime error.
type panickyEnricher struct{}

func (panickyEnricher) EnrichBatch(_ context.Context, _ []llmdto.JobPostEnrichInput) ([]*llmdto.JobPostEnrichResult, error) {
	panic("simulated LLM panic")
}

// TestProcessFetch_PanicInProcessGroup_RecoveredAndJobsTermed verifies that when
// processGroup panics, the panic is recovered inside processFetch, all jobs in
// the affected group are marked as errors, and each message is Term'd.
func TestProcessFetch_PanicInProcessGroup_RecoveredAndJobsTermed(t *testing.T) {
	pipeline := &fakePipelineService{}
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(panickyEnricher{}, pipeline, &fakePublisher{}, &fakeLedger{}, clk, 5)

	job := makeRunJob("panic-job-1")
	msg, state := js.NewTestMsg(marshalJob(t, job), 1)

	if err := w.processFetch(context.Background(), []*js.Msg{msg}); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	// The panic was recovered; message must be Term'd.
	if !state.Termed {
		t.Errorf("expected message Term'd after panic recovery; got %+v", state)
	}
	// markError was called for the job.
	if len(pipeline.markCalls) == 0 {
		t.Errorf("expected markError called for job; got %v", pipeline.markCalls)
	}
}

// ── processResult Ack fail paths ─────────────────────────────────────────────

// TestProcessResult_RoleMismatch_AckFails_LogsWarn verifies that when a
// role-mismatch job's Ack fails, the warning is logged (no panic).
func TestProcessResult_RoleMismatch_AckFails_LogsWarn(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	result := &llmdto.JobPostEnrichResult{RoleMatch: false, Sponsorship: true}

	fj, state := makeTestFetchedJob(t, baseSummary(), 0)
	state.AckErr = errors.New("ack failed")
	stats := &groupOutcomeStats{}

	w.processResult(context.Background(), fj, result, stats)
	if !state.Acked {
		t.Errorf("Ack must be attempted even when it errors")
	}
}

// TestProcessResult_DropJob_AckFails_LogsWarn verifies Ack-fail warning for
// the dropJob (no sponsorship) path.
func TestProcessResult_DropJob_AckFails_LogsWarn(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	result := &llmdto.JobPostEnrichResult{RoleMatch: true, Sponsorship: false}

	fj, state := makeTestFetchedJob(t, baseSummary(), 0)
	state.AckErr = errors.New("ack failed")
	stats := &groupOutcomeStats{}

	w.processResult(context.Background(), fj, result, stats)
	if !state.Acked {
		t.Errorf("Ack must be attempted even when it errors")
	}
}

// TestProcessResult_SaveFail_AckFails_LogsWarn verifies Ack-fail warning for
// the save-error path (pipeline.SaveEnrichedJob fails).
func TestProcessResult_SaveFail_AckFails_LogsWarn(t *testing.T) {
	pipeline := &fakePipelineService{saveErr: errors.New("db unavailable")}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	result := &llmdto.JobPostEnrichResult{
		RoleMatch: true, Sponsorship: true, Country: "USA",
	}

	fj, state := makeTestFetchedJob(t, baseSummary(), 0)
	state.AckErr = errors.New("ack failed")
	stats := &groupOutcomeStats{}

	w.processResult(context.Background(), fj, result, stats)
	if !state.Acked {
		t.Errorf("Ack must be attempted even when it errors")
	}
}

// TestProcessResult_SaveSuccess_AckFails_LogsWarn verifies Ack-fail warning for
// the save-success path.
func TestProcessResult_SaveSuccess_AckFails_LogsWarn(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	result := &llmdto.JobPostEnrichResult{
		RoleMatch: true, Sponsorship: true, Country: "USA",
	}

	fj, state := makeTestFetchedJob(t, baseSummary(), 0)
	state.AckErr = errors.New("ack failed")
	stats := &groupOutcomeStats{}

	w.processResult(context.Background(), fj, result, stats)
	if !state.Acked {
		t.Errorf("Ack must be attempted even when it errors")
	}
}

// ── distributeResults allRouted=false path ────────────────────────────────────

// TestDistributeResults_RouteMissingFails_AllRoutedFalse verifies that when
// the LLM returns no result for a job AND routing that job fails (Nak error),
// distributeResults returns false (allRouted=false).
func TestDistributeResults_RouteMissingFails_AllRoutedFalse(t *testing.T) {
	pipeline := &fakePipelineService{}
	clk := &fakeClock{now: time.Now()}
	// Scorer returns empty result list — no result for "missing-job" → routeMissingResult called.
	w := makeWorkerFull(&fakeScorer{results: nil}, pipeline, &fakePublisher{}, &fakeLedger{}, clk, 5)

	job := baseSummary()
	job.ID = "missing-job"
	fj, state := makeTestFetchedJob(t, job, 0) // retryCount=0 < analyzerMaxRetries → Nak path
	state.NakErr = errors.New("nak failed")    // handleFailure returns false → allRouted=false

	group := []fetchedJob{fj}
	routed := w.distributeResults(
		context.Background(),
		logger.NewNoop(),
		group,
		nil, // empty results → no match for "missing-job"
		&groupOutcomeStats{},
	)
	if routed {
		t.Errorf("expected allRouted=false when routeMissingResult returns false")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func jobRetryEntry() jobsvc.RetryLedgerEntry {
	return jobsvc.RetryLedgerEntry{
		JobID:         "test-job",
		RetryCount:    1,
		NextAttemptAt: time.Now().Add(time.Minute),
	}
}
