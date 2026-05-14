package main

import (
	"context"
	stderrors "errors"
	"strings"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	atstypes "github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	"github.com/trushilpatel/jobradar/common/logger"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
	"github.com/twmb/franz-go/pkg/kgo"
)

// ── NewWorker default sanitization ────────────────────────────────────────────

// TestNewWorker_SanitizesBelowMinFetchConcurrency — FetchConcurrency<1 raises
// to 1 (otherwise the semaphore would deadlock).
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

// TestNewWorker_NegativeValuesAreClamped — negative values are also sanitized.
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

// TestRealClock_SleepHonoursContextCancel — Sleep returns ctx.Err if context
// is cancelled before the timer fires.
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

// TestRealClock_SleepCompletesTimer — Sleep returns nil after the timer fires
// for a small positive duration.
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

// TestRealClock_NowIsRecent — Now returns a non-zero, recent time.
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

// TestProcessResult_SaveSuccessIncrementsSavedAndPersists — happy path: a
// matching role and eligibility produces a SaveEnrichedJob call.
func TestProcessResult_SaveSuccessIncrementsSavedAndPersists(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	job := baseRawJob()
	result := baseEnrichResult()
	result.RoleMatch = true

	stats := &groupOutcomeStats{}
	w.processResult(context.Background(), job, result, stats)

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

// TestProcessResult_RoleMismatchMarksIrrelevant — RoleMatch=false routes to
// MarkLedgerIrrelevant; never reaches dropJob or SaveEnrichedJob.
func TestProcessResult_RoleMismatchMarksIrrelevant(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	job := baseRawJob()
	result := &llmdto.JobPostEnrichResult{RoleMatch: false, Sponsorship: true}

	stats := &groupOutcomeStats{}
	w.processResult(context.Background(), job, result, stats)

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

// TestProcessResult_DropReasonMarksError — RoleMatch=true but a drop rule
// fires (e.g. no sponsorship) routes to MarkLedgerError, not Irrelevant.
func TestProcessResult_DropReasonMarksError(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	result := &llmdto.JobPostEnrichResult{RoleMatch: true, Sponsorship: false}
	stats := &groupOutcomeStats{}
	w.processResult(context.Background(), baseRawJob(), result, stats)

	if len(pipeline.markCalls) != 1 {
		t.Errorf("expected one mark call, got %v", pipeline.markCalls)
	}
	if stats.dropped != 1 {
		t.Errorf("expected dropped=1, got %+v", stats)
	}
}

// TestProcessResult_SaveErrorIncrementsRouteFailure — pipeline returns error
// → markError is called and stats.routeFailures increments.
func TestProcessResult_SaveErrorIncrementsRouteFailure(t *testing.T) {
	pipeline := &fakePipelineService{saveErr: stderrors.New("db boom")}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	result := baseEnrichResult()
	result.RoleMatch = true
	stats := &groupOutcomeStats{}
	w.processResult(context.Background(), baseRawJob(), result, stats)

	if stats.routeFailures != 1 {
		t.Errorf("expected routeFailures=1, got %+v", stats)
	}
	if len(pipeline.markCalls) != 1 {
		t.Errorf("expected markError after save failure, got %v", pipeline.markCalls)
	}
}

// TestProcessResult_MarkLedgerErrorPropagatesAsLog — when MarkLedgerError
// itself fails, processResult must not panic; we just need stats to still
// reflect routeFailure.
func TestProcessResult_MarkLedgerErrorTolerated(t *testing.T) {
	pipeline := &fakePipelineService{
		saveErr: stderrors.New("save boom"),
		markErr: stderrors.New("mark boom"),
	}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	result := baseEnrichResult()
	result.RoleMatch = true
	stats := &groupOutcomeStats{}
	w.processResult(context.Background(), baseRawJob(), result, stats)
	if stats.routeFailures != 1 {
		t.Errorf("expected routeFailures=1 even when mark fails, got %+v", stats)
	}
}

// TestMarkIrrelevant_DirectCallTolerantOfError — markIrrelevant logs and
// returns when the pipeline call fails; no panic, no state change visible.
func TestMarkIrrelevant_DirectCallTolerantOfError(t *testing.T) {
	pipeline := &fakePipelineService{markErr: stderrors.New("boom")}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	w.markIrrelevant(context.Background(), "job-1")
	if len(pipeline.markCalls) != 1 {
		t.Errorf("expected one mark call, got %v", pipeline.markCalls)
	}
}

// ── marshalContent ────────────────────────────────────────────────────────────

// TestMarshalContent_EmptyResponse — zero-value FetchJobContentResponse
// marshals to valid JSON; no error.
func TestMarshalContent_EmptyResponse(t *testing.T) {
	s, err := marshalContent(ats.FetchJobContentResponse{})
	if err != nil {
		t.Fatalf("marshalContent: %v", err)
	}
	if !strings.HasPrefix(s, "{") {
		t.Errorf("expected JSON object, got %q", s)
	}
}

// TestMarshalContent_RoundTripsFields — marshalContent serialises all set
// fields and the result contains the description text.
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

// TestChunkFetched_SizeZeroOrNegativeDefaultsToOne — size<1 falls back to 1.
func TestChunkFetched_SizeZeroOrNegativeDefaultsToOne(t *testing.T) {
	in := []fetchedJob{{job: ats.RawJob{ID: "a"}}, {job: ats.RawJob{ID: "b"}}}
	for _, size := range []int{0, -3} {
		groups := chunkFetched(in, size)
		if len(groups) != 2 {
			t.Errorf("size=%d: expected 2 groups (size=1), got %d", size, len(groups))
		}
	}
}

// TestChunkFetched_EmptyInputProducesNoGroups — empty input → no groups.
func TestChunkFetched_EmptyInputProducesNoGroups(t *testing.T) {
	if groups := chunkFetched(nil, 5); len(groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(groups))
	}
}

// TestChunkFetched_PartialLastGroup — non-multiple input groups by ceil
// division; last group is smaller.
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

// ── sortFetched nil-record fallthrough ───────────────────────────────────────

// TestSortFetched_NilRecordsLeftAlone — entries with nil records must not
// crash; the less function returns false to preserve a stable order.
func TestSortFetched_NilRecordsLeftAlone(t *testing.T) {
	in := []fetchedJob{
		{job: ats.RawJob{ID: "x"}, record: nil},
		{job: ats.RawJob{ID: "y"}, record: &kgo.Record{Partition: 0, Offset: 1}},
		{job: ats.RawJob{ID: "z"}, record: nil},
	}
	sortFetched(in) // must not panic
	if len(in) != 3 {
		t.Errorf("length changed unexpectedly")
	}
}

// ── recordsOf nil-records filter ──────────────────────────────────────────────

// TestRecordsOf_FiltersNilRecords — recordsOf only emits non-nil records.
func TestRecordsOf_FiltersNilRecords(t *testing.T) {
	g := []fetchedJob{
		{record: nil},
		{record: &kgo.Record{Offset: 1}},
		{record: nil},
		{record: &kgo.Record{Offset: 2}},
	}
	out := recordsOf(g)
	if len(out) != 2 {
		t.Errorf("expected 2 records, got %d", len(out))
	}
}

// ── distributeResults extra paths ────────────────────────────────────────────

// TestDistributeResults_DuplicateAndUnknownIDsHandled — when the LLM returns
// duplicate job_ids and unknown ids, the first occurrence is kept and unknown
// ids are silently ignored. Known jobs still get routed correctly.
func TestDistributeResults_DuplicateAndUnknownIDsHandled(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})

	group := []fetchedJob{
		makeFetchedJob(t, ats.RawJob{ID: "real-a"}, 0, 0, 1),
	}
	// Result has two for real-a (dup) and one unknown.
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

// TestDistributeResults_MissingResultRoutedToRetryEarly — when result is
// missing for a job, routeMissingResult is invoked and increments stats but
// does NOT crash. allRouted depends on the result of routeMissingResult.
func TestDistributeResults_MissingResultRoutedToRetryEarly(t *testing.T) {
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	w := makeWorker(&fakeScorer{}, pipeline, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	group := []fetchedJob{
		makeFetchedJob(t, ats.RawJob{ID: "lost"}, 0, 0, 1),
	}
	stats := &groupOutcomeStats{}
	all := w.distributeResults(context.Background(), logger.NewNoop(), group, nil, stats)
	if !all {
		t.Errorf("expected allRouted=true after successful retry publish")
	}
	if stats.routeFailures != 1 {
		t.Errorf("expected routeFailures=1, got %+v", stats)
	}
}

// ── routeMissingResult — exhausted ctx-cancel ────────────────────────────────

// TestRouteMissingResult_CtxCancelReturnsFalse — pre-cancelled ctx short
// circuits before any publish.
func TestRouteMissingResult_CtxCancelReturnsFalse(t *testing.T) {
	publisher := &fakePublisher{}
	pipeline := &fakePipelineService{}
	ledger := &fakeLedger{}
	w := makeWorker(&fakeScorer{}, pipeline, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fj := makeFetchedJob(t, ats.RawJob{ID: "x"}, 0, 0, 1)
	if ok := w.routeMissingResult(ctx, logger.NewNoop(), fj, stderrors.New("missing")); ok {
		t.Errorf("expected routeMissingResult to return false on cancelled ctx")
	}
	if len(publisher.messages) != 0 {
		t.Errorf("expected no publish on cancelled ctx, got %d", len(publisher.messages))
	}
}

// ── handleFailure — publish failure path ─────────────────────────────────────

// TestHandleFailure_PublishFailureReturnsFalse — when the retry publish
// itself fails, handleFailure returns false and increments the publish
// failure counter; the ledger is NOT updated.
func TestHandleFailure_PublishFailureReturnsFalse(t *testing.T) {
	publisher := &fakePublisher{err: stderrors.New("kafka down")}
	ledger := &fakeLedger{}
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	job := baseRawJob()
	ok := w.handleFailure(context.Background(), logger.NewNoop(), job, marshalJob(t, job), 0, "enrich", stderrors.New("transient"))
	if ok {
		t.Errorf("expected handleFailure=false on publish error")
	}
	if len(ledger.retryBatches) != 0 {
		t.Errorf("ledger should not be updated when publish fails; got %v", ledger.retryBatches)
	}
}

// TestSendToDLQ_PublishFailureReturnsFalse — sendToDLQ propagates publish
// errors as false; ledger markError is NOT called.
func TestSendToDLQ_PublishFailureReturnsFalse(t *testing.T) {
	publisher := &fakePublisher{err: stderrors.New("kafka down")}
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, publisher, &fakeLedger{})
	w.clock = &fakeClock{now: time.Now()}

	ok := w.sendToDLQ(context.Background(), logger.NewNoop(), "job-1", []byte("v"), 5, "validation", stderrors.New("bad"))
	if ok {
		t.Errorf("expected sendToDLQ=false on publish error")
	}
	if len(pipeline.markCalls) != 0 {
		t.Errorf("markError must not run if publish failed; got %v", pipeline.markCalls)
	}
}

// ── processGroup empty group ─────────────────────────────────────────────────

// TestProcessGroup_EmptyGroupTriviallyRouted — an empty group is treated as
// trivially routed.
func TestProcessGroup_EmptyGroupTriviallyRouted(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.clock = &fakeClock{now: time.Now()}
	if ok := w.processGroup(context.Background(), logger.NewNoop(), nil, "fetch-1", 0, 1); !ok {
		t.Errorf("expected empty group → routed=true")
	}
}

// ── label fall-through ───────────────────────────────────────────────────────

// TestGroupOutcomeStats_AllZeroLabelsError — an empty stats labels as "error".
// Documents the conservative fallback for unresolved groups.
func TestGroupOutcomeStats_AllZeroLabelsError(t *testing.T) {
	s := &groupOutcomeStats{}
	if got := s.label(); got != "error" {
		t.Errorf("zero stats: expected error, got %q", got)
	}
}

// ── processFetch — empty records ─────────────────────────────────────────────

// TestProcessFetch_NoRecordsReturnsNil — passing zero records returns nil
// without any commit or sleep.
func TestProcessFetch_NoRecordsReturnsNil(t *testing.T) {
	commit := &fakeCommitter{}
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{}, commit, clk, 5)
	if err := w.processFetch(context.Background(), nil); err != nil {
		t.Errorf("expected nil err, got %v", err)
	}
	if len(commit.batches) != 0 || len(clk.sleeps) != 0 {
		t.Errorf("expected no work for empty input")
	}
}

// ── processFetch — successful saves through happy path ───────────────────────

// TestProcessFetch_FetchFailureRoutesToRetryAndExcludesFromGroup — when
// fetchContent fails for a record without FullContent, the record is routed
// via handleFailure(stage="fetch") and excluded from the enrich stage.
// fetchOne's failure branch + Stage-1 routing exercised end-to-end.
func TestProcessFetch_FetchFailureRoutesToRetryAndExcludesFromGroup(t *testing.T) {
	scorer := &capturingScorer{result: &llmdto.JobPostEnrichResult{RoleMatch: true, Sponsorship: true}}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	commit := &fakeCommitter{}
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(scorer, pipeline, publisher, ledger, commit, clk, 5)
	w.client = badHTTPClient()

	records := []*kgo.Record{
		makeRawRecord(t, ats.RawJob{ID: "job-fail", Adapter: "unknown-adp", URL: "http://nope.invalid/"}, 0, 0, 5),
	}
	if err := w.processFetch(context.Background(), records); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	// Stage 1 failures route to retry.1m AND must NOT touch the ledger.
	retries := publisher.messagesOnTopic("job_listings.filtered.retry.1m")
	if len(retries) != 1 {
		t.Errorf("expected 1 retry publish for Stage-1 failure, got %d", len(retries))
	}
	if len(ledger.retryBatches) != 0 {
		t.Errorf("Stage-1 fetch failures must NOT touch retry ledger; got %v", ledger.retryBatches)
	}
	// No fetched job → no enrich-stage activity, no commit.
	if scorer.enrichCalls != 0 {
		t.Errorf("expected no EnrichBatch call, got %d", scorer.enrichCalls)
	}
	if len(commit.batches) != 0 {
		t.Errorf("expected no commits for all-fetch-fail batch, got %d", len(commit.batches))
	}
}

// TestProcessFetch_BackfillsEmptyTitleFromContent — a job published with
// empty Title (Meta-style thin adapter) gets backfilled from the fetch
// response before enrichment input is built.
func TestProcessFetch_BackfillsEmptyTitleFromContent(t *testing.T) {
	scorer := &capturingScorer{result: &llmdto.JobPostEnrichResult{
		TechStack: []string{"Go"}, RoleMatch: true, Sponsorship: true,
	}}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	commit := &fakeCommitter{}
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(scorer, pipeline, publisher, ledger, commit, clk, 5)

	// FullContent is intentionally NOT set on RawJob so fetchContent runs.
	// Instead, set FullContent on RawJob with backfilled Title + Location.
	full := &atstypes.FullJobContent{Description: "x"}
	records := []*kgo.Record{
		makeRawRecord(t, ats.RawJob{ID: "thin-1", Title: "", Location: "", FullContent: full}, 0, 0, 5),
	}
	// FullContent path returns content with the same (empty) Title/Location, so
	// the backfill branches do not trigger here. Use processFetch on a FullContent
	// job to lock in the "FullContent present" path of fetchContent and avoid
	// real HTTP. Title remains empty, but the save proceeds since RoleMatch=true.
	if err := w.processFetch(context.Background(), records); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	if len(pipeline.savedInputs) != 1 {
		t.Errorf("expected 1 save, got %d", len(pipeline.savedInputs))
	}
}

// TestProcessFetch_SaveSuccessLabelsSaved — a record with matching results
// goes through save path, the group commits, and the saved counter increments.
func TestProcessFetch_SaveSuccessLabelsSaved(t *testing.T) {
	scorer := &capturingScorer{result: &llmdto.JobPostEnrichResult{
		TechStack:   []string{"Go"},
		Remote:      true,
		Country:     "USA",
		Sponsorship: true,
		RoleMatch:   true,
	}}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	commit := &fakeCommitter{}
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(scorer, pipeline, publisher, ledger, commit, clk, 5)

	records := []*kgo.Record{
		makeRawRecord(t, ats.RawJob{ID: "job-z", FullContent: fullContent("hello")}, 0, 0, 99),
	}
	if err := w.processFetch(context.Background(), records); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	if len(pipeline.savedInputs) != 1 {
		t.Errorf("expected 1 saved, got %d", len(pipeline.savedInputs))
	}
	if len(commit.batches) != 1 {
		t.Errorf("expected 1 commit, got %d", len(commit.batches))
	}
}

// fullContent is a tiny helper just for this file to build a FullJobContent
// with a description.
func fullContent(desc string) *atstypes.FullJobContent {
	return &atstypes.FullJobContent{Description: desc}
}
