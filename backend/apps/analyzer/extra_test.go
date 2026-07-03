package main

import (
	"context"
	stderrors "errors"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters"
	"github.com/trushilpatel/jobradar/common/ats/core"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	"github.com/trushilpatel/jobradar/common/logger"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
)

// capturingLogger records log calls so tests can assert that errors are not
// silently swallowed.
type capturingLogger struct {
	mu     sync.Mutex
	errors []string
	warns  []string
}

func (l *capturingLogger) Debug(_ string, _ ...logger.Field) {}
func (l *capturingLogger) Info(_ string, _ ...logger.Field)  {}
func (l *capturingLogger) Warn(msg string, _ ...logger.Field) {
	l.mu.Lock()
	l.warns = append(l.warns, msg)
	l.mu.Unlock()
}
func (l *capturingLogger) Error(msg string, _ error, _ ...logger.Field) {
	l.mu.Lock()
	l.errors = append(l.errors, msg)
	l.mu.Unlock()
}
func (l *capturingLogger) With(_ ...logger.Field) logger.Logger        { return l }
func (l *capturingLogger) WithContext(_ context.Context) logger.Logger { return l }

func (l *capturingLogger) hasError(substr string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.errors {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}

func (l *capturingLogger) hasWarn(substr string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, w := range l.warns {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

// ── NewWorker default sanitization ────────────────────────────────────────────

func TestNewWorker_SanitizesBelowMinFetchConcurrency(t *testing.T) {
	opts := makeBaseOptions(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	opts.FetchConcurrency = 0
	opts.LLMBatchSize = 0
	w := NewWorker(opts)
	if w.fetchConc < 1 {
		t.Errorf("FetchConcurrency must be >= 1; got %d", w.fetchConc)
	}
	if w.llmBatchSize < 1 {
		t.Errorf("LLMBatchSize must be >= 1; got %d", w.llmBatchSize)
	}
}

func TestNewWorker_NegativeValuesAreClamped(t *testing.T) {
	opts := makeBaseOptions(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	opts.FetchConcurrency = -5
	opts.LLMBatchSize = -2
	w := NewWorker(opts)
	if w.fetchConc != 1 || w.llmBatchSize != 1 {
		t.Errorf("expected sanitized to 1; got fetchConc=%d batch=%d", w.fetchConc, w.llmBatchSize)
	}
}

// ── realClock.Sleep ───────────────────────────────────────────────────────────

func TestRealClock_SleepHonoursContextCancel(t *testing.T) {
	c := realClock{}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	err := c.Sleep(ctx, 10*time.Second)
	if err == nil {
		t.Errorf("expected context cancel error")
	}
}

func TestRealClock_SleepCompletesTimer(t *testing.T) {
	c := realClock{}
	start := time.Now()
	if err := c.Sleep(context.Background(), 5*time.Millisecond); err != nil {
		t.Errorf("Sleep: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 5*time.Millisecond {
		t.Errorf("Sleep should wait the full duration; elapsed=%v", elapsed)
	}
}

func TestRealClock_NowIsRecent(t *testing.T) {
	c := realClock{}
	got := c.Now()
	if got.IsZero() {
		t.Errorf("Now() returned zero time")
	}
	if diff := time.Since(got); diff > time.Minute {
		t.Errorf("Now() should be recent; got diff=%v", diff)
	}
}

// ── processResult — save / drop / mark-irrelevant / mark-error paths ─────────

func TestProcessResult_SaveSuccessIncrementsSavedAndPersists(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	job := baseSummary()
	result := baseEnrichResult()
	result.RoleMatch = true

	fj, state := makeTestFetchedJob(t, job, 0)
	stats := &groupOutcomeStats{}
	w.processResult(context.Background(), fj, result, stats)

	if !state.Acked {
		t.Errorf("expected msg Acked on save success, got %+v", state)
	}
	if len(pipeline.savedInputs) != 1 {
		t.Fatalf("expected 1 saved input, got %d", len(pipeline.savedInputs))
	}
	if got := pipeline.savedInputs[0]; got.Summary.ID != job.ID || got.Enriched != result {
		t.Errorf("save input mismatch: %+v", got)
	}
	if stats.saved != 1 {
		t.Errorf("expected saved=1, got %+v", stats)
	}
}

func TestProcessResult_RoleMismatchMarksIrrelevant(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	job := baseSummary()
	result := &llmdto.JobPostEnrichResult{RoleMatch: false, Sponsorship: true}

	fj, state := makeTestFetchedJob(t, job, 0)
	stats := &groupOutcomeStats{}
	w.processResult(context.Background(), fj, result, stats)

	if !state.Acked {
		t.Errorf("expected msg Acked on role mismatch, got %+v", state)
	}
	if len(pipeline.markCalls) != 1 || pipeline.markCalls[0] != job.ID {
		t.Errorf("expected MarkLedgerIrrelevant call, got %v", pipeline.markCalls)
	}
	if len(pipeline.savedInputs) != 0 {
		t.Errorf("must not save on role mismatch")
	}
	if stats.dropped != 1 {
		t.Errorf("expected dropped=1, got %+v", stats)
	}
}

func TestProcessResult_DropReasonMarksError(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	result := &llmdto.JobPostEnrichResult{RoleMatch: true, Sponsorship: false}

	fj, state := makeTestFetchedJob(t, baseSummary(), 0)
	stats := &groupOutcomeStats{}
	w.processResult(context.Background(), fj, result, stats)

	if !state.Acked {
		t.Errorf("expected msg Acked on drop, got %+v", state)
	}
	if len(pipeline.markCalls) != 1 {
		t.Errorf("expected one mark call, got %v", pipeline.markCalls)
	}
	if stats.dropped != 1 {
		t.Errorf("expected dropped=1, got %+v", stats)
	}
}

func TestProcessResult_SaveErrorIncrementsRouteFailure(t *testing.T) {
	pipeline := &fakePipelineService{saveErr: stderrors.New("db boom")}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	result := baseEnrichResult()
	result.RoleMatch = true

	fj, state := makeTestFetchedJob(t, baseSummary(), 0)
	stats := &groupOutcomeStats{}
	w.processResult(context.Background(), fj, result, stats)

	if !state.Acked {
		t.Errorf("expected msg Acked even on save error (terminal path), got %+v", state)
	}
	if stats.routeFailures != 1 {
		t.Errorf("expected routeFailures=1, got %+v", stats)
	}
	if len(pipeline.markCalls) != 1 {
		t.Errorf("expected markError after save failure, got %v", pipeline.markCalls)
	}
}

func TestProcessResult_MarkLedgerErrorTolerated(t *testing.T) {
	pipeline := &fakePipelineService{
		saveErr: stderrors.New("save boom"),
		markErr: stderrors.New("mark boom"),
	}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	result := baseEnrichResult()
	result.RoleMatch = true

	fj, _ := makeTestFetchedJob(t, baseSummary(), 0)
	stats := &groupOutcomeStats{}
	w.processResult(context.Background(), fj, result, stats)
	if stats.routeFailures != 1 {
		t.Errorf("expected routeFailures=1 even when mark fails, got %+v", stats)
	}
}

func TestMarkIrrelevant_DirectCallTolerantOfError(t *testing.T) {
	pipeline := &fakePipelineService{markErr: stderrors.New("boom")}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	w.markIrrelevant(context.Background(), "job-1")
	if len(pipeline.markCalls) != 1 {
		t.Errorf("expected one mark call, got %v", pipeline.markCalls)
	}
}

func TestMarkNonUSA_DirectCallTolerantOfError(t *testing.T) {
	pipeline := &fakePipelineService{markErr: stderrors.New("boom")}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	w.markNonUSA(context.Background(), "job-india-1")
	if len(pipeline.markCalls) != 1 {
		t.Errorf("expected one mark call, got %v", pipeline.markCalls)
	}
}

func TestMarkNonUSA_DirectCallSuccess(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	w.markNonUSA(context.Background(), "job-india-2")
	if len(pipeline.markCalls) != 1 || pipeline.markCalls[0] != "job-india-2" {
		t.Errorf("expected one mark call for job-india-2, got %v", pipeline.markCalls)
	}
}

// ── marshalContent ────────────────────────────────────────────────────────────

func TestMarshalContent_EmptyResponse(t *testing.T) {
	s, err := marshalContent(core.JobPost{})
	if err != nil {
		t.Fatalf("marshalContent: %v", err)
	}
	if !strings.HasPrefix(s, "{") {
		t.Errorf("expected JSON object, got %q", s)
	}
}

func TestMarshalContent_RoundTripsFields(t *testing.T) {
	resp := core.JobPost{
		ID:          "x",
		Title:       "Senior",
		Description: "A description with special chars: \" & < >",
	}
	s, err := marshalContent(resp)
	if err != nil {
		t.Fatalf("marshalContent: %v", err)
	}
	if !strings.Contains(s, "A description with special chars") {
		t.Errorf("description should appear in JSON; got %q", s)
	}
}

// ── chunkFetched edge cases ───────────────────────────────────────────────────

func TestChunkFetched_SizeZeroOrNegativeDefaultsToOne(t *testing.T) {
	in := []fetchedJob{{summary: core.JobSummary{ID: "a"}}, {summary: core.JobSummary{ID: "b"}}}
	for _, size := range []int{0, -3} {
		groups := chunkFetched(in, size)
		if len(groups) != 2 {
			t.Errorf("size=%d: expected 2 groups (size=1), got %d", size, len(groups))
		}
	}
}

func TestChunkFetched_EmptyInputProducesNoGroups(t *testing.T) {
	if groups := chunkFetched(nil, 5); len(groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(groups))
	}
}

func TestChunkFetched_PartialLastGroup(t *testing.T) {
	in := []fetchedJob{
		{summary: core.JobSummary{ID: "a"}}, {summary: core.JobSummary{ID: "b"}}, {summary: core.JobSummary{ID: "c"}},
	}
	groups := chunkFetched(in, 2)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if len(groups[0]) != 2 || len(groups[1]) != 1 {
		t.Errorf("expected sizes [2,1]; got [%d,%d]", len(groups[0]), len(groups[1]))
	}
}

// ── distributeResults extra paths ─────────────────────────────────────────────

func TestDistributeResults_DuplicateAndUnknownIDsHandled(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})

	group := []fetchedJob{
		makeFetchedJob(t, core.JobSummary{ID: "real-a"}, 0),
	}
	results := []*llmdto.JobPostEnrichResult{
		{JobID: "real-a", RoleMatch: true, Sponsorship: true},
		{JobID: "real-a", RoleMatch: true, Sponsorship: true}, // duplicate
		{JobID: "ghost", RoleMatch: true, Sponsorship: true},  // unknown
		nil, // nil entry, must be skipped
	}
	stats := &groupOutcomeStats{}
	all := w.distributeResults(context.Background(), logger.NewNoop(), group, results, stats)
	if !all {
		t.Errorf("expected all routed=true")
	}
	if stats.saved != 1 {
		t.Errorf("expected exactly one save from the first duplicate; got %+v", stats)
	}
}

func TestDistributeResults_MissingResultRoutedToRetryEarly(t *testing.T) {
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	w := makeWorker(&fakeScorer{}, pipeline, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	group := []fetchedJob{
		makeFetchedJob(t, core.JobSummary{ID: "lost"}, 0),
	}
	stats := &groupOutcomeStats{}
	all := w.distributeResults(context.Background(), logger.NewNoop(), group, nil, stats)
	if !all {
		t.Errorf("expected allRouted=true after successful Nak")
	}
	if stats.routeFailures != 1 {
		t.Errorf("expected routeFailures=1, got %+v", stats)
	}
}

// ── routeMissingResult — ctx-cancel ──────────────────────────────────────────

func TestRouteMissingResult_CtxCancelReturnsFalse(t *testing.T) {
	publisher := &fakePublisher{}
	pipeline := &fakePipelineService{}
	ledger := &fakeLedger{}
	w := makeWorker(&fakeScorer{}, pipeline, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fj := makeFetchedJob(t, core.JobSummary{ID: "x"}, 0)
	if ok := w.routeMissingResult(ctx, logger.NewNoop(), fj, stderrors.New("missing")); ok {
		t.Errorf("expected routeMissingResult to return false on cancelled ctx")
	}
	if len(publisher.records) != 0 {
		t.Errorf("expected no publish on cancelled ctx, got %d", len(publisher.records))
	}
}

// ── handleFailure — DLQ publish failure path ─────────────────────────────────

// When the DLQ publish fails the message must NOT be Termed (to allow NATS redelivery).
func TestSendToDLQ_PublishFailureReturnsFalse(t *testing.T) {
	publisher := &fakePublisher{err: stderrors.New("nats down")}
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, publisher, &fakeLedger{})
	w.clock = &fakeClock{now: time.Now()}

	fj, state := makeTestFetchedJob(t, baseSummary(), 5)
	ok := w.sendToDLQ(context.Background(), logger.NewNoop(), fj, "validation", stderrors.New("bad"))
	if ok {
		t.Errorf("expected sendToDLQ=false on publish error")
	}
	if state.Termed {
		t.Errorf("msg must NOT be Termed if DLQ publish failed; got %+v", state)
	}
	if len(pipeline.markCalls) != 0 {
		t.Errorf("markError must not run if publish failed; got %v", pipeline.markCalls)
	}
}

// ── processGroup empty group ──────────────────────────────────────────────────

func TestProcessGroup_EmptyGroupTriviallyRouted(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.clock = &fakeClock{now: time.Now()}
	if ok := w.processGroup(context.Background(), logger.NewNoop(), nil, "fetch-1", 0, 1); !ok {
		t.Errorf("expected empty group → routed=true")
	}
}

// ── label fall-through ────────────────────────────────────────────────────────

func TestGroupOutcomeStats_AllZeroLabelsError(t *testing.T) {
	s := &groupOutcomeStats{}
	if got := s.label(); got != "error" {
		t.Errorf("zero stats: expected error, got %q", got)
	}
}

// ── processFetch — empty records ──────────────────────────────────────────────

func TestProcessFetch_NoRecordsReturnsNil(t *testing.T) {
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{}, clk, 5)
	if err := w.processFetch(context.Background(), nil); err != nil {
		t.Errorf("expected nil err, got %v", err)
	}
	if len(clk.sleeps) != 0 {
		t.Errorf("expected no work for empty input")
	}
}

// ── processFetch — fetch failure routes to Nak ────────────────────────────────

// Stage-1 fetch failure (bad HTTP) → msg Nak'd, no EnrichBatch call, no ledger write.
func TestProcessFetch_FetchFailureRoutesToNakAndExcludesFromGroup(t *testing.T) {
	scorer := &capturingScorer{result: &llmdto.JobPostEnrichResult{RoleMatch: true, Sponsorship: true}}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(scorer, pipeline, publisher, ledger, clk, 5)
	// Unknown adapter → bundle resolution fails → stage-1 fetch failure.
	w.bundleFn = func(_ string, _ logger.Logger, _ *http.Client) (adapters.AdapterBundle, error) {
		return adapters.AdapterBundle{}, stderrors.New("unknown adapter")
	}

	msg, state := makeTestMsg(t, core.JobSummary{ID: "job-fail", Adapter: "unknown-adp", URL: "http://nope.invalid/"}, 1)
	if err := w.processFetch(context.Background(), []*js.Msg{msg}); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	// Stage 1 failure → Nak (retryCount=0 < analyzerMaxRetries=3), no ledger write.
	if !state.Naked {
		t.Errorf("expected msg Naked for Stage-1 fetch failure, got %+v", state)
	}
	if len(ledger.retryBatches) != 0 {
		t.Errorf("Stage-1 fetch failures must NOT touch retry ledger; got %v", ledger.retryBatches)
	}
	if scorer.enrichCalls != 0 {
		t.Errorf("expected no EnrichBatch call, got %d", scorer.enrichCalls)
	}
}

// ── processFetch — title backfill ─────────────────────────────────────────────

func TestProcessFetch_BackfillsEmptyTitleFromContent(t *testing.T) {
	scorer := &capturingScorer{result: &llmdto.JobPostEnrichResult{
		TechStack: []string{"Go"}, RoleMatch: true, Sponsorship: true,
	}}
	pipeline := &fakePipelineService{}
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(scorer, pipeline, &fakePublisher{}, &fakeLedger{}, clk, 5)

	full := &core.JobContent{Description: "x"}
	msg, _ := makeTestMsg(t, core.JobSummary{ID: "thin-1", Title: "", Location: "", JobContent: full}, 1)
	if err := w.processFetch(context.Background(), []*js.Msg{msg}); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	if len(pipeline.savedInputs) != 1 {
		t.Errorf("expected 1 save, got %d", len(pipeline.savedInputs))
	}
}

// ── processFetch — save through happy path ────────────────────────────────────

func TestProcessFetch_SaveSuccessAcksMsg(t *testing.T) {
	scorer := &capturingScorer{result: &llmdto.JobPostEnrichResult{
		TechStack:   []string{"Go"},
		Remote:      true,
		Country:     "USA",
		Sponsorship: true,
		RoleMatch:   true,
	}}
	pipeline := &fakePipelineService{}
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(scorer, pipeline, &fakePublisher{}, &fakeLedger{}, clk, 5)

	msg, state := makeTestMsg(t, core.JobSummary{ID: "job-z", JobContent: fullContent("hello")}, 1)
	if err := w.processFetch(context.Background(), []*js.Msg{msg}); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	if len(pipeline.savedInputs) != 1 {
		t.Errorf("expected 1 saved, got %d", len(pipeline.savedInputs))
	}
	if !state.Acked {
		t.Errorf("expected msg Acked on save success, got %+v", state)
	}
}

// fullContent is a tiny helper to build inline JobContent with a description.
func fullContent(desc string) *core.JobContent {
	return &core.JobContent{Description: desc}
}

// ── sendToDLQ — NATS error path assertions ────────────────────────────────────

// TestSendToDLQ_NATSPublishError_ErrorLogged verifies that when the NATS
// publisher returns an error the failure is logged at ERROR level (not
// silently swallowed) and the message is NOT Termed so NATS can redeliver.
func TestSendToDLQ_NATSPublishError_ErrorLogged(t *testing.T) {
	log := &capturingLogger{}
	publisher := &fakePublisher{err: stderrors.New("nats: connection refused")}
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, publisher, &fakeLedger{})

	fj, state := makeTestFetchedJob(t, baseSummary(), 2)
	ok := w.sendToDLQ(context.Background(), log, fj, "exhausted", stderrors.New("LLM failed"))
	if ok {
		t.Errorf("expected sendToDLQ=false when NATS publish fails")
	}
	if state.Termed {
		t.Errorf("msg must NOT be Termed if DLQ publish failed (NATS should redeliver); got %+v", state)
	}
	if len(pipeline.markCalls) != 0 {
		t.Errorf("markError must not run when DLQ publish fails; got %v", pipeline.markCalls)
	}
	if !log.hasError("publish DLQ failed") {
		t.Errorf("expected 'publish DLQ failed' logged at ERROR level; got errors=%v", log.errors)
	}
}

// TestSendToDLQ_SuccessPath_JobTermedAndLogged verifies the happy path:
// successful DLQ publish Terms the message, marks the ledger, and logs a WARN.
func TestSendToDLQ_SuccessPath_JobTermedAndLogged(t *testing.T) {
	log := &capturingLogger{}
	publisher := &fakePublisher{}
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, publisher, &fakeLedger{})

	fj, state := makeTestFetchedJob(t, baseSummary(), 3)
	before := dlqCount("exhausted")
	origErr := stderrors.New("LLM failed after max retries")
	ok := w.sendToDLQ(context.Background(), log, fj, "exhausted", origErr)
	if !ok {
		t.Fatalf("expected sendToDLQ=true on successful publish")
	}
	if !state.Termed {
		t.Errorf("msg must be Termed after successful DLQ publish; got %+v", state)
	}
	if len(pipeline.markCalls) != 1 || pipeline.markCalls[0] != fj.summary.ID {
		t.Errorf("expected markError for job %q; got %v", fj.summary.ID, pipeline.markCalls)
	}
	if got := dlqCount("exhausted") - before; got != 1 {
		t.Errorf("expected enricher_dlq_publishes_total{exhausted} +1; got +%v", got)
	}
	if pubs := publisher.messagesOnSubject(js.SubjectJobsDLQ); len(pubs) != 1 {
		t.Errorf("expected 1 DLQ publish; got %d", len(pubs))
	}
	if !log.hasWarn("moved to DLQ") {
		t.Errorf("expected 'moved to DLQ' logged at WARN level; got warns=%v", log.warns)
	}
}

// ── fetchOne: Ack-failure paths on the non-USA reject gates ───────────────────

// TestFetchOne_EarlyNonUSA_AckFails_NoPanic exercises the Path A early reject
// gate (LocationKnown=true, non-USA) when the subsequent msg.Ack() returns an
// error: the worker must log a warning and return ok=false without panicking.
// Covers the Ack-error branch in fetchOne's early non-USA path.
func TestFetchOne_EarlyNonUSA_AckFails_NoPanic(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})

	// LocationKnown=true with a foreign location → early reject before any fetch.
	summary := core.JobSummary{
		ID:            "wd-london",
		Adapter:       "workday",
		URL:           "https://wd/london",
		LocationKnown: true,
		Location:      "London, United Kingdom",
	}
	msg, state := makeTestMsg(t, summary, 1)
	state.AckErr = stderrors.New("nats ack error") // force the Ack-failure branch

	_, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)
	if ok {
		t.Fatalf("fetchOne should reject non-USA job early (ok=false)")
	}
	if !state.Acked {
		t.Errorf("Ack must still be attempted on early non-USA reject; got %+v", state)
	}
	if len(pipeline.markCalls) != 1 || pipeline.markCalls[0] != summary.ID {
		t.Errorf("expected MarkLedgerNonUSA(%q); got %v", summary.ID, pipeline.markCalls)
	}
}

// TestFetchOne_PostFetchNonUSA_AckFails_NoPanic exercises the Path B post-fetch
// reject gate (CountryCode non-US) when msg.Ack() returns an error: the worker
// logs a warning and returns ok=false without panicking. Covers the Ack-error
// branch in fetchOne's post-fetch non-USA path.
func TestFetchOne_PostFetchNonUSA_AckFails_NoPanic(t *testing.T) {
	pipeline := &fakePipelineService{}
	pf := &fakePostFetcher{post: core.JobPost{Title: "Eng", Location: "Bengaluru", CountryCode: "IN"}}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	w.bundleFn = bundleFnReturning(pf, nil)

	summary := core.JobSummary{ID: "wd-in-ackfail", Adapter: "workday", URL: "https://wd/2"}
	msg, state := makeTestMsg(t, summary, 1)
	state.AckErr = stderrors.New("nats ack error") // force the Ack-failure branch

	_, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)
	if ok {
		t.Fatalf("fetchOne should reject CountryCode=IN job (ok=false)")
	}
	if !state.Acked {
		t.Errorf("Ack must still be attempted on post-fetch non-USA reject; got %+v", state)
	}
	if len(pipeline.markCalls) != 1 || pipeline.markCalls[0] != summary.ID {
		t.Errorf("expected MarkLedgerNonUSA(%q); got %v", summary.ID, pipeline.markCalls)
	}
}

// ── analyzerMergedLocations: single-Location fallback branch ──────────────────

// TestAnalyzerMergedLocations_LocationOnlyFallback verifies that when
// AllLocations is empty but the single Location string is set, the helper wraps
// it in a one-element slice. Covers the Location-only fallback branch.
func TestAnalyzerMergedLocations_LocationOnlyFallback(t *testing.T) {
	got := analyzerMergedLocations(core.JobSummary{Location: "Austin, TX"})
	if len(got) != 1 || got[0] != "Austin, TX" {
		t.Errorf("expected [Austin, TX]; got %v", got)
	}
}

// TestAnalyzerMergedLocations_AllLocationsPreferred verifies AllLocations wins
// over the single Location when both are present.
func TestAnalyzerMergedLocations_AllLocationsPreferred(t *testing.T) {
	got := analyzerMergedLocations(core.JobSummary{
		Location:     "Austin, TX",
		AllLocations: []string{"NYC", "SF"},
	})
	if len(got) != 2 || got[0] != "NYC" || got[1] != "SF" {
		t.Errorf("expected [NYC SF]; got %v", got)
	}
}

// TestAnalyzerMergedLocations_BothEmptyReturnsNil verifies a summary with no
// location signal at all yields nil.
func TestAnalyzerMergedLocations_BothEmptyReturnsNil(t *testing.T) {
	if got := analyzerMergedLocations(core.JobSummary{}); got != nil {
		t.Errorf("expected nil when no location set; got %v", got)
	}
}

// ── AdapterRateLimiter.getOrCreate: double-check-under-write-lock branch ──────

// TestAdapterRateLimiter_GetOrCreate_DoubleCheckRace forces the double-check
// branch inside getOrCreate, where a goroutine acquires the write lock only to
// find another goroutine already created the limiter. A tight loop of fresh
// limiters with many concurrent racers reliably exercises the second-check
// early return: every racer must observe the identical limiter instance for a
// given adapter, proving no duplicate was created. Runs clean under -race.
func TestAdapterRateLimiter_GetOrCreate_DoubleCheckRace(t *testing.T) {
	const adapter = "greenhouse"
	const racers = 64

	for attempt := 0; attempt < 200; attempt++ {
		rl := NewAdapterRateLimiter(1000.0, 1000, 0, logger.NewNoop())

		var wg sync.WaitGroup
		results := make([]*rate.Limiter, racers)
		start := make(chan struct{})
		wg.Add(racers)
		for i := 0; i < racers; i++ {
			go func(idx int) {
				defer wg.Done()
				<-start // release all goroutines simultaneously
				results[idx] = rl.getOrCreate(adapter)
			}(i)
		}
		// Give the scheduler a chance to park every goroutine on the barrier so
		// they contend for the write lock at the same instant.
		runtime.Gosched()
		close(start)
		wg.Wait()

		// Every racer must observe the exact same limiter instance — the
		// double-check guarantees only one is ever created.
		first := results[0]
		if first == nil {
			t.Fatalf("attempt %d: getOrCreate returned nil", attempt)
		}
		for i, l := range results {
			if l != first {
				t.Fatalf("attempt %d racer %d: got a different limiter instance; double-check failed", attempt, i)
			}
		}
	}
}

// ── processResult: post-enrichment country gate ───────────────────────────────

// A non-remote job enriched with a non-USA country must be marked
// non_usa_location and must NOT be saved to the DB, even when it is otherwise
// eligible (role match, sponsorship, etc.). This is the final gate that catches
// thin-adapter jobs (e.g. Workday ATCI India) that slipped through the
// fetcher-stage gates.
func TestProcessResult_NonUSACountry_MarksNonUSAAndSkipsSave(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})

	result := baseEnrichResult()
	result.RoleMatch = true
	result.Country = "India" // unambiguously non-USA
	result.Remote = false    // on-site: gate must fire

	fj, state := makeTestFetchedJob(t, baseSummary(), 0)
	stats := &groupOutcomeStats{}
	w.processResult(context.Background(), fj, result, stats)

	if len(pipeline.savedInputs) != 0 {
		t.Errorf("SaveEnrichedJob must NOT be called for a non-USA job, got %d calls", len(pipeline.savedInputs))
	}
	if len(pipeline.markCalls) != 1 || pipeline.markCalls[0] != baseSummary().ID {
		t.Errorf("expected markNonUSA for job ID %q; markCalls=%v", baseSummary().ID, pipeline.markCalls)
	}
	if !state.Acked {
		t.Errorf("msg must be Acked after non-USA rejection")
	}
	if stats.dropped != 1 {
		t.Errorf("stats.dropped should be 1 for non-USA rejection, got %d", stats.dropped)
	}
}

// Remote non-USA jobs must pass through regardless of country — they may be
// US-eligible even when posted from a non-US company or listed location.
func TestProcessResult_NonUSACountry_RemoteJob_Passes(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})

	result := baseEnrichResult()
	result.RoleMatch = true
	result.Country = "Canada"
	result.Remote = true

	fj, _ := makeTestFetchedJob(t, baseSummary(), 0)
	stats := &groupOutcomeStats{}
	w.processResult(context.Background(), fj, result, stats)

	if len(pipeline.savedInputs) != 1 {
		t.Errorf("remote non-USA job must pass through; expected 1 save, got %d", len(pipeline.savedInputs))
	}
	if len(pipeline.markCalls) != 0 {
		t.Errorf("expected no markNonUSA for remote non-USA job; markCalls=%v", pipeline.markCalls)
	}
}

// When the LLM could not determine country (returns ""), the job must pass
// through — never reject on missing signal.
func TestProcessResult_EmptyCountry_DoesNotReject(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})

	result := baseEnrichResult()
	result.RoleMatch = true
	result.Country = ""

	fj, _ := makeTestFetchedJob(t, baseSummary(), 0)
	stats := &groupOutcomeStats{}
	w.processResult(context.Background(), fj, result, stats)

	if len(pipeline.savedInputs) != 1 {
		t.Errorf("empty country should not trigger non-USA gate; expected 1 save, got %d", len(pipeline.savedInputs))
	}
	if len(pipeline.markCalls) != 0 {
		t.Errorf("expected no markNonUSA for empty country; markCalls=%v", pipeline.markCalls)
	}
}

// USA country must not trigger the non-USA gate.
func TestProcessResult_USACountry_Passes(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})

	result := baseEnrichResult()
	result.RoleMatch = true
	result.Country = "USA"

	fj, _ := makeTestFetchedJob(t, baseSummary(), 0)
	stats := &groupOutcomeStats{}
	w.processResult(context.Background(), fj, result, stats)

	if len(pipeline.savedInputs) != 1 {
		t.Errorf("USA country must pass through; expected 1 save, got %d", len(pipeline.savedInputs))
	}
}

// Non-USA gate fires for every distinct non-USA country string the LLM returns
// when the job is not remote.
func TestProcessResult_NonUSACountry_VariousCountries(t *testing.T) {
	countries := []string{"Canada", "UK", "Israel", "Romania", "China", "Philippines"}
	for _, country := range countries {
		t.Run(country, func(t *testing.T) {
			pipeline := &fakePipelineService{}
			w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})

			result := baseEnrichResult()
			result.RoleMatch = true
			result.Country = country
			result.Remote = false // on-site: gate must fire

			fj, _ := makeTestFetchedJob(t, baseSummary(), 0)
			w.processResult(context.Background(), fj, result, &groupOutcomeStats{})

			if len(pipeline.savedInputs) != 0 {
				t.Errorf("country %q must trigger non-USA gate; expected 0 saves, got %d", country, len(pipeline.savedInputs))
			}
			if len(pipeline.markCalls) != 1 {
				t.Errorf("country %q must trigger markNonUSA; markCalls=%v", country, pipeline.markCalls)
			}
		})
	}
}

// ── Run: fetchSize-below-one guard ───────────────────────────────────────────

// TestRun_FetchSizeBelowOne_ClampedToOne exercises the `fetchSize < 1` guard at
// the top of Run. NewWorker clamps fetchConc/llmBatchSize to >= 1, so we poke
// fetchConc to 0 directly to drive the product below one. The injected consumer
// asserts the clamped fetch size (1) is requested and then cancels the context
// so Run exits cleanly.
func TestRun_FetchSizeBelowOne_ClampedToOne(t *testing.T) {
	opts := makeBaseOptions(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w := NewWorker(opts)
	w.fetchConc = 0 // forces fetchSize = 0 * llmBatchSize = 0 → guard clamps to 1

	ctx, cancel := context.WithCancel(context.Background())
	var gotSize int
	var once sync.Once
	fc := &funcMessageFetcher{fn: func(n int, _ time.Duration) ([]*js.Msg, error) {
		once.Do(func() { gotSize = n })
		cancel()
		return nil, context.Canceled
	}}
	w.consumer = fc

	if err := w.Run(ctx); err != nil {
		t.Errorf("Run: want nil after ctx cancel, got %v", err)
	}
	if gotSize != 1 {
		t.Errorf("fetchSize guard should clamp to 1; Fetch requested %d", gotSize)
	}
}
