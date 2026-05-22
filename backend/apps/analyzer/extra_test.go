package main

import (
	"context"
	stderrors "errors"
	"strings"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	atstypes "github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	"github.com/trushilpatel/jobradar/common/logger"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
)

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
	job := baseRawJob()
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
	if got := pipeline.savedInputs[0]; got.Raw.ID != job.ID || got.Enriched != result {
		t.Errorf("save input mismatch: %+v", got)
	}
	if stats.saved != 1 {
		t.Errorf("expected saved=1, got %+v", stats)
	}
}

func TestProcessResult_RoleMismatchMarksIrrelevant(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	job := baseRawJob()
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

	fj, state := makeTestFetchedJob(t, baseRawJob(), 0)
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

	fj, state := makeTestFetchedJob(t, baseRawJob(), 0)
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

	fj, _ := makeTestFetchedJob(t, baseRawJob(), 0)
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

// ── marshalContent ────────────────────────────────────────────────────────────

func TestMarshalContent_EmptyResponse(t *testing.T) {
	s, err := marshalContent(ats.FetchJobContentResponse{})
	if err != nil {
		t.Fatalf("marshalContent: %v", err)
	}
	if !strings.HasPrefix(s, "{") {
		t.Errorf("expected JSON object, got %q", s)
	}
}

func TestMarshalContent_RoundTripsFields(t *testing.T) {
	resp := ats.FetchJobContentResponse{
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
	in := []fetchedJob{{job: ats.RawJob{ID: "a"}}, {job: ats.RawJob{ID: "b"}}}
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
		{job: ats.RawJob{ID: "a"}}, {job: ats.RawJob{ID: "b"}}, {job: ats.RawJob{ID: "c"}},
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
		makeFetchedJob(t, ats.RawJob{ID: "real-a"}, 0),
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
		makeFetchedJob(t, ats.RawJob{ID: "lost"}, 0),
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
	fj := makeFetchedJob(t, ats.RawJob{ID: "x"}, 0)
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

	fj, state := makeTestFetchedJob(t, baseRawJob(), 5)
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
	w.client = badHTTPClient()

	msg, state := makeTestMsg(t, ats.RawJob{ID: "job-fail", Adapter: "unknown-adp", URL: "http://nope.invalid/"}, 1)
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

	full := &atstypes.FullJobContent{Description: "x"}
	msg, _ := makeTestMsg(t, ats.RawJob{ID: "thin-1", Title: "", Location: "", FullContent: full}, 1)
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

	msg, state := makeTestMsg(t, ats.RawJob{ID: "job-z", FullContent: fullContent("hello")}, 1)
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

// fullContent is a tiny helper to build a FullJobContent with a description.
func fullContent(desc string) *atstypes.FullJobContent {
	return &atstypes.FullJobContent{Description: desc}
}
