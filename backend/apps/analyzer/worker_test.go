package main

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	atstypes "github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	domainerr "github.com/trushilpatel/jobradar/common/errors"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
)

// ── fakes ────────────────────────────────────────────────────────────────────

type fakeScorer struct {
	results []*llmdto.JobPostEnrichResult
	err     error
	calls   int
	mu      sync.Mutex
}

func (f *fakeScorer) EnrichBatch(_ context.Context, _ []llmdto.JobPostEnrichInput) ([]*llmdto.JobPostEnrichResult, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return f.results, f.err
}

type fakePipelineService struct {
	saveErr     error
	markErr     error
	hasSeen     bool
	hasSeenErr  error
	savedInputs []jobdto.EnrichedJobInput
	markCalls   []string
	mu          sync.Mutex
}

func (f *fakePipelineService) SaveEnrichedJob(_ context.Context, input jobdto.EnrichedJobInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveErr != nil {
		return f.saveErr
	}
	f.savedInputs = append(f.savedInputs, input)
	return nil
}

func (f *fakePipelineService) MarkLedgerError(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markCalls = append(f.markCalls, id)
	return f.markErr
}

func (f *fakePipelineService) MarkLedgerIrrelevant(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markCalls = append(f.markCalls, id)
	return f.markErr
}

func (f *fakePipelineService) HasSeenJob(_ context.Context, id string) (bool, error) {
	return f.hasSeen, f.hasSeenErr
}

var _ jobsvc.PipelineService = (*fakePipelineService)(nil)

// fakeLedger records UpdateRetryBatch calls and can be programmed to fail
// the first N invocations for testing the inline-retry behaviour.
type fakeLedger struct {
	mu           sync.Mutex
	retryBatches [][]jobsvc.RetryLedgerEntry
	failureQueue []error
}

func (f *fakeLedger) UpdateRetryBatch(_ context.Context, entries []jobsvc.RetryLedgerEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.failureQueue) > 0 {
		err := f.failureQueue[0]
		f.failureQueue = f.failureQueue[1:]
		if err != nil {
			return err
		}
	}
	dup := append([]jobsvc.RetryLedgerEntry(nil), entries...)
	f.retryBatches = append(f.retryBatches, dup)
	return nil
}

var _ EnricherLedger = (*fakeLedger)(nil)

// publishedRecord captures a single call to the fake publisher.
type publishedRecord struct {
	subject string
	value   []byte
}

type fakePublisher struct {
	mu      sync.Mutex
	err     error
	records []publishedRecord
}

func (f *fakePublisher) Publish(_ context.Context, subject string, value []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.records = append(f.records, publishedRecord{subject: subject, value: value})
	return nil
}

func (f *fakePublisher) PublishWithMsgID(_ context.Context, subject string, value []byte, _ string) error {
	return f.Publish(context.Background(), subject, value)
}

func (f *fakePublisher) messagesOnSubject(subject string) []publishedRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []publishedRecord
	for _, r := range f.records {
		if strings.Contains(r.subject, subject) {
			out = append(out, r)
		}
	}
	return out
}

type fakeClock struct {
	mu         sync.Mutex
	now        time.Time
	sleeps     []time.Duration
	sleepErr   error
	sleepErrOn int
	sleepCalls int
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	f.sleepCalls++
	idx := f.sleepCalls
	if f.sleepErrOn > 0 && idx == f.sleepErrOn {
		f.mu.Unlock()
		return context.Canceled
	}
	if f.sleepErr != nil {
		f.mu.Unlock()
		return f.sleepErr
	}
	f.sleeps = append(f.sleeps, d)
	f.mu.Unlock()
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func makeBaseOptions(scorer llmEnricher, pipeline jobsvc.PipelineService, publisher js.Publisher, ledger EnricherLedger) WorkerOptions {
	return WorkerOptions{
		Consumer:         nil,
		Publisher:        publisher,
		Scorer:           scorer,
		Pipeline:         pipeline,
		Ledger:           ledger,
		RateLimiter:      nil,
		Log:              logger.NewNoop(),
		MaxExpYears:      0,
		FetchConcurrency: 5,
		LLMBatchSize:     5,
		LLMCallInterval:  5 * time.Second,
		FlushTimeout:     120 * time.Second,
	}
}

// llmEnricher is a local alias matching the scorer interface.
type llmEnricher = interface {
	EnrichBatch(ctx context.Context, inputs []llmdto.JobPostEnrichInput) ([]*llmdto.JobPostEnrichResult, error)
}

func makeWorker(scorer llmEnricher, pipeline jobsvc.PipelineService, publisher js.Publisher, ledger EnricherLedger) *Worker {
	return NewWorker(makeBaseOptions(scorer, pipeline, publisher, ledger))
}

// makeWorkerFull wires a worker with a fake clock for processFetch-level tests.
func makeWorkerFull(scorer llmEnricher, pipeline jobsvc.PipelineService, publisher js.Publisher, ledger EnricherLedger, clk clock, llmBatchSize int) *Worker {
	opts := makeBaseOptions(scorer, pipeline, publisher, ledger)
	opts.LLMBatchSize = llmBatchSize
	w := NewWorker(opts)
	w.clock = clk
	return w
}

func baseRawJob() ats.RawJob {
	return ats.RawJob{
		ID:        "ashby-abc123",
		CompanyID: "company-xyz",
		Title:     "Platform Engineer",
		URL:       "https://jobs.ashbyhq.com/co/abc123",
		Adapter:   "ashby",
	}
}

func baseEnrichResult() *llmdto.JobPostEnrichResult {
	return &llmdto.JobPostEnrichResult{
		TechStack:    []string{"Go", "K8s"},
		Remote:       true,
		Country:      "USA",
		RoleCategory: "backend",
		Sponsorship:  true,
	}
}

// mustMarshal marshals v to JSON or fails the test.
func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// makeTestMsg builds a NATS test message for a job with the given delivery count.
func makeTestMsg(t *testing.T, job ats.RawJob, deliveryCount int) (*js.Msg, *js.TestMsgState) {
	t.Helper()
	return js.NewTestMsg(mustMarshal(t, job), deliveryCount)
}

// makeTestFetchedJob builds a fetchedJob backed by a testMsg for direct method tests.
func makeTestFetchedJob(t *testing.T, job ats.RawJob, retryCount int) (fetchedJob, *js.TestMsgState) {
	t.Helper()
	msg, state := js.NewTestMsg(mustMarshal(t, job), retryCount+1)
	return fetchedJob{
		job:        job,
		content:    ats.FetchJobContentResponse{ID: job.ID, Title: job.Title},
		msg:        msg,
		retryCount: retryCount,
		ctx:        context.Background(),
	}, state
}

// makeFetchedJob builds a fetchedJob for direct processGroup/distributeResults tests.
func makeFetchedJob(t *testing.T, job ats.RawJob, retryCount int) fetchedJob {
	fj, _ := makeTestFetchedJob(t, job, retryCount)
	return fj
}

// marshalJob is a helper to produce the JSON body the worker would receive.
func marshalJob(t *testing.T, job ats.RawJob) []byte {
	t.Helper()
	return mustMarshal(t, job)
}

// ── toEnrichedInput / dropJob tests ──────────────────────────────────────────

func TestToEnrichedInput_FieldMapping(t *testing.T) {
	job := baseRawJob()
	result := baseEnrichResult()

	input := toEnrichedInput(job, result)

	if input.Raw.ID != job.ID {
		t.Errorf("Raw.ID: want %q got %q", job.ID, input.Raw.ID)
	}
	if input.Raw.CompanyID != job.CompanyID {
		t.Errorf("Raw.CompanyID: want %q got %q", job.CompanyID, input.Raw.CompanyID)
	}
	if input.Enriched != result {
		t.Error("Enriched should point to the same result")
	}
}

func TestDropJob_NoSponsorship(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	result := &llmdto.JobPostEnrichResult{Sponsorship: false}
	if reason := w.dropJob(result); reason == "" {
		t.Error("expected drop reason for no sponsorship")
	}
}

func TestDropJob_RequiresCitizen(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	result := &llmdto.JobPostEnrichResult{Sponsorship: true, RequiresCitizen: true}
	if reason := w.dropJob(result); reason == "" {
		t.Error("expected drop reason for requires citizen")
	}
}

func TestDropJob_SecurityClearance(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	result := &llmdto.JobPostEnrichResult{Sponsorship: true, SecurityClearance: true}
	if reason := w.dropJob(result); reason == "" {
		t.Error("expected drop reason for security clearance")
	}
}

func TestDropJob_ExperienceExceedsMax(t *testing.T) {
	opts := makeBaseOptions(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	opts.MaxExpYears = 3
	w := NewWorker(opts)
	exp := 5
	result := &llmdto.JobPostEnrichResult{Sponsorship: true, ExperienceYears: &exp}
	if reason := w.dropJob(result); reason == "" {
		t.Error("expected drop reason for experience exceeds max")
	}
}

func TestDropJob_EligibleJob(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	result := baseEnrichResult()
	if reason := w.dropJob(result); reason != "" {
		t.Errorf("expected no drop reason, got %q", reason)
	}
}

// ── FullContent / skip-fetch tests ───────────────────────────────────────────

type capturingScorer struct {
	result      *llmdto.JobPostEnrichResult
	err         error
	lastInputs  []llmdto.JobPostEnrichInput
	enrichCalls int
	mu          sync.Mutex
}

func (c *capturingScorer) EnrichBatch(_ context.Context, inputs []llmdto.JobPostEnrichInput) ([]*llmdto.JobPostEnrichResult, error) {
	c.mu.Lock()
	c.lastInputs = inputs
	c.enrichCalls++
	c.mu.Unlock()
	if c.err != nil {
		return nil, c.err
	}
	out := make([]*llmdto.JobPostEnrichResult, 0, len(inputs))
	for _, in := range inputs {
		r := *c.result
		r.JobID = in.ID
		out = append(out, &r)
	}
	return out, nil
}

type failingTransport struct {
	called bool
}

func (ft *failingTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	ft.called = true
	return nil, stderrors.New("HTTP client must not be called when FullContent is present")
}

func TestEnrich_SkipsFetchWhenFullContentPresent(t *testing.T) {
	transport := &failingTransport{}
	scorer := &capturingScorer{result: &llmdto.JobPostEnrichResult{
		TechStack:   []string{"Go"},
		Remote:      true,
		Country:     "USA",
		Sponsorship: true,
		RoleMatch:   true,
	}}
	pipeline := &fakePipelineService{}

	w := makeWorker(scorer, pipeline, &fakePublisher{}, &fakeLedger{})
	w.client = &http.Client{Transport: transport}

	job := ats.RawJob{
		ID:        "google-123",
		CompanyID: "company-google",
		Title:     "Senior Go Engineer",
		Location:  "Remote",
		URL:       "https://careers.google.com/jobs/123",
		Adapter:   "google",
		FullContent: &atstypes.FullJobContent{
			Description:   "Go engineer role building large-scale systems.",
			Requirements:  "5 years Go experience required.",
			Compensation:  "$200,000",
			WorkplaceType: "remote",
			Department:    "Engineering",
			PublishedDate: "2026-04-01",
		},
	}

	content, err := w.fetchContent(context.Background(), job)
	if err != nil {
		t.Fatalf("fetchContent returned unexpected error: %v", err)
	}
	if transport.called {
		t.Error("HTTP client should not be called when FullContent is present")
	}
	if !strings.Contains(content.Description, "Go engineer role") {
		t.Errorf("description should be drawn from FullContent; got: %s", content.Description)
	}

	msg, _ := js.NewTestMsg(marshalJob(t, job), 1)
	fj := fetchedJob{
		job:     job,
		content: content,
		msg:     msg,
		ctx:     context.Background(),
	}
	w.processGroup(context.Background(), logger.NewNoop(), []fetchedJob{fj}, "fetch-1", 0, 1)

	if scorer.enrichCalls != 1 {
		t.Errorf("EnrichBatch call count: want 1 got %d", scorer.enrichCalls)
	}
	if len(scorer.lastInputs) != 1 || !strings.Contains(scorer.lastInputs[0].RawText, "Go engineer role") {
		t.Errorf("LLM RawText should contain description text; got: %v", scorer.lastInputs)
	}
	if len(pipeline.markCalls) != 0 {
		t.Errorf("expected no error mark on success, got %v", pipeline.markCalls)
	}
}

func TestFullContentToResponse_FieldMapping(t *testing.T) {
	job := ats.RawJob{
		ID:        "google-456",
		CompanyID: "company-google",
		Title:     "Staff Software Engineer",
		Location:  "Mountain View, CA",
		URL:       "https://careers.google.com/jobs/456",
		Adapter:   "google",
		FullContent: &atstypes.FullJobContent{
			Description:   "Design and build distributed systems.",
			Requirements:  "7+ years of backend experience.",
			Compensation:  "$250,000 - $350,000",
			WorkplaceType: "hybrid",
			Department:    "Infrastructure",
			PublishedDate: "2026-03-15",
		},
	}
	resp := fullContentToResponse(job)
	if resp.ID != job.ID || resp.Title != job.Title || resp.URL != job.URL {
		t.Errorf("RawJob fields not copied correctly")
	}
	if resp.Description != job.FullContent.Description {
		t.Errorf("Description mismatch")
	}
	if resp.Compensation != job.FullContent.Compensation {
		t.Errorf("Compensation mismatch")
	}
}

// ── handleFailure / sendToDLQ tests ──────────────────────────────────────────

// Stage 2 transient error → Nak + writes retry ledger.
func TestHandleFailure_Stage2TransientRoutesToRetryAndLedger(t *testing.T) {
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, publisher, ledger)
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	w.clock = &fakeClock{now: now}

	fj, state := makeTestFetchedJob(t, baseRawJob(), 0)
	ok := w.handleFailure(context.Background(), logger.NewNoop(), fj, "enrich", stderrors.New("LLM timeout"))
	if !ok {
		t.Fatalf("handleFailure returned false unexpectedly")
	}
	if !state.Naked {
		t.Errorf("expected msg Naked (Nak called), got %+v", state)
	}
	if len(ledger.retryBatches) != 1 || ledger.retryBatches[0][0].RetryCount != 1 {
		t.Errorf("expected UpdateRetryBatch with RetryCount=1, got %+v", ledger.retryBatches)
	}
	if !ledger.retryBatches[0][0].NextAttemptAt.Equal(now.Add(time.Minute)) {
		t.Errorf("expected NextAttemptAt=now+1m, got %v", ledger.retryBatches[0][0].NextAttemptAt)
	}
}

// Stage 1 transient error → Nak, NO ledger write.
func TestHandleFailure_Stage1TransientNoLedgerWrite(t *testing.T) {
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	fj, state := makeTestFetchedJob(t, baseRawJob(), 0)
	ok := w.handleFailure(context.Background(), logger.NewNoop(), fj, "fetch", stderrors.New("http 500"))
	if !ok {
		t.Fatalf("handleFailure returned false")
	}
	if !state.Naked {
		t.Errorf("expected msg Naked for Stage-1 retry, got %+v", state)
	}
	if len(ledger.retryBatches) != 0 {
		t.Errorf("expected no UpdateRetryBatch on stage=fetch, got %+v", ledger.retryBatches)
	}
}

// Stage 1 exhausted → DLQ with cause=fetch_failed, msg Termed.
func TestHandleFailure_Stage1ExhaustedDLQFetchFailed(t *testing.T) {
	publisher := &fakePublisher{}
	pipeline := &fakePipelineService{}
	ledger := &fakeLedger{}
	w := makeWorker(&fakeScorer{}, pipeline, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	fj, state := makeTestFetchedJob(t, baseRawJob(), 3)
	before := dlqCount("fetch_failed")
	ok := w.handleFailure(context.Background(), logger.NewNoop(), fj, "fetch", stderrors.New("http 500"))
	if !ok {
		t.Fatalf("handleFailure returned false")
	}
	if !state.Termed {
		t.Errorf("expected msg Termed for exhausted Stage-1, got %+v", state)
	}
	if got := dlqCount("fetch_failed") - before; got != 1 {
		t.Errorf("expected enricher_dlq_publishes_total{cause=fetch_failed} +1, got %v", got)
	}
	if len(publisher.messagesOnSubject(js.SubjectJobsDLQ)) != 1 {
		t.Errorf("expected DLQ publish")
	}
	if len(pipeline.markCalls) != 1 {
		t.Errorf("expected ledger markError, got %v", pipeline.markCalls)
	}
}

// Validation error → DLQ with cause=validation, msg Termed, no retry ledger write.
func TestHandleFailure_ValidationDLQNoLedger(t *testing.T) {
	publisher := &fakePublisher{}
	pipeline := &fakePipelineService{}
	ledger := &fakeLedger{}
	w := makeWorker(&fakeScorer{}, pipeline, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	fj, state := makeTestFetchedJob(t, baseRawJob(), 0)
	before := dlqCount("validation")
	ok := w.handleFailure(context.Background(), logger.NewNoop(), fj, "enrich", domainerr.ErrLLMValidation)
	if !ok {
		t.Fatalf("handleFailure returned false")
	}
	if !state.Termed {
		t.Errorf("expected msg Termed for validation error, got %+v", state)
	}
	if got := dlqCount("validation") - before; got != 1 {
		t.Errorf("expected enricher_dlq_publishes_total{cause=validation} +1, got %v", got)
	}
	if len(ledger.retryBatches) != 0 {
		t.Errorf("expected no retry ledger writes for validation")
	}
}

// Context cancel → skip all dispositions, returns false.
func TestHandleFailure_ContextCanceledSkipsPublish(t *testing.T) {
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fj, state := makeTestFetchedJob(t, baseRawJob(), 0)
	ok := w.handleFailure(ctx, logger.NewNoop(), fj, "enrich", context.Canceled)
	if ok {
		t.Errorf("handleFailure should return false on ctx cancel")
	}
	if state.Naked || state.Termed || state.Acked {
		t.Errorf("expected no disposition on ctx cancel, got %+v", state)
	}
	if len(publisher.records) != 0 {
		t.Errorf("expected no publishes, got %d", len(publisher.records))
	}
	if len(pipeline.markCalls) != 0 {
		t.Errorf("expected no ledger marks, got %v", pipeline.markCalls)
	}
}

// ── processGroup / routeBatchFailure tests ────────────────────────────────────

// Stage 2 whole-group EnrichBatch failure → per-job Nak + UpdateRetryBatch.
func TestProcessGroup_EnrichBatchFailureWholeGroupRetries(t *testing.T) {
	scorer := &fakeScorer{err: stderrors.New("LLM timeout")}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	w := makeWorker(scorer, pipeline, publisher, ledger)
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	w.clock = &fakeClock{now: now}

	fj1, state1 := makeTestFetchedJob(t, ats.RawJob{ID: "job-1", Title: "A"}, 0)
	fj2, state2 := makeTestFetchedJob(t, ats.RawJob{ID: "job-2", Title: "B"}, 0)
	group := []fetchedJob{fj1, fj2}

	routed := w.processGroup(context.Background(), logger.NewNoop(), group, "fetch-1", 0, 1)
	if !routed {
		t.Fatalf("processGroup should return true on successful retry routing")
	}
	if !state1.Naked || !state2.Naked {
		t.Errorf("expected both msgs Naked; state1=%+v state2=%+v", state1, state2)
	}
	if len(ledger.retryBatches) != 2 {
		t.Fatalf("expected 2 UpdateRetryBatch calls (one per job), got %d", len(ledger.retryBatches))
	}
	for _, batch := range ledger.retryBatches {
		if len(batch) != 1 || batch[0].RetryCount != 1 {
			t.Errorf("expected one entry per call with RetryCount=1, got %+v", batch)
		}
		if !batch[0].NextAttemptAt.Equal(now.Add(time.Minute)) {
			t.Errorf("expected NextAttemptAt=now+1m, got %v", batch[0].NextAttemptAt)
		}
	}
}

// Mixed retry_count in one group under transient error: tier-eligible Nak'd,
// exhausted DLQ'd + Termed.
func TestProcessGroup_MixedRetryCounts(t *testing.T) {
	scorer := &fakeScorer{err: stderrors.New("LLM timeout")}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	w := makeWorker(scorer, pipeline, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	fjA, stateA := makeTestFetchedJob(t, ats.RawJob{ID: "job-a"}, 0)
	fjB, stateB := makeTestFetchedJob(t, ats.RawJob{ID: "job-b"}, 3)
	w.processGroup(context.Background(), logger.NewNoop(), []fetchedJob{fjA, fjB}, "fetch-1", 0, 1)

	if !stateA.Naked {
		t.Errorf("expected job-a (retryCount=0) Naked, got %+v", stateA)
	}
	if !stateB.Termed {
		t.Errorf("expected job-b (retryCount=3, exhausted) Termed, got %+v", stateB)
	}
	if len(publisher.messagesOnSubject(js.SubjectJobsDLQ)) != 1 {
		t.Errorf("expected 1 DLQ publish for exhausted job")
	}
	if len(ledger.retryBatches) != 1 {
		t.Errorf("expected UpdateRetryBatch only for tier-eligible, got %d", len(ledger.retryBatches))
	}
}

// Validation error → all jobs DLQ'd cause=validation, Termed, no retry ledger.
func TestProcessGroup_ValidationErrorDLQAll(t *testing.T) {
	scorer := &fakeScorer{err: domainerr.ErrLLMValidation}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	w := makeWorker(scorer, pipeline, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	fj1, state1 := makeTestFetchedJob(t, ats.RawJob{ID: "job-1"}, 0)
	fj2, state2 := makeTestFetchedJob(t, ats.RawJob{ID: "job-2"}, 0)
	before := dlqCount("validation")
	w.processGroup(context.Background(), logger.NewNoop(), []fetchedJob{fj1, fj2}, "fetch-1", 0, 1)

	if got := dlqCount("validation") - before; got != 2 {
		t.Errorf("expected enricher_dlq_publishes_total{cause=validation} +2, got %v", got)
	}
	if !state1.Termed || !state2.Termed {
		t.Errorf("expected both msgs Termed; state1=%+v state2=%+v", state1, state2)
	}
	if len(ledger.retryBatches) != 0 {
		t.Errorf("expected no retry ledger writes, got %+v", ledger.retryBatches)
	}
}

// Missing LLM result → Nak (within budget) then DLQ cause=llm_result_missing (exhausted).
func TestProcessGroup_MissingResultRouting(t *testing.T) {
	scorer := &fakeScorer{
		results: []*llmdto.JobPostEnrichResult{
			{JobID: "job-a", RoleMatch: true, Sponsorship: true, TechStack: []string{"Go"}},
		},
	}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	w := makeWorker(scorer, pipeline, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	fjA, _ := makeTestFetchedJob(t, ats.RawJob{ID: "job-a"}, 0)
	fjB, stateB := makeTestFetchedJob(t, ats.RawJob{ID: "job-b"}, 0)
	w.processGroup(context.Background(), logger.NewNoop(), []fetchedJob{fjA, fjB}, "fetch-1", 0, 1)

	if !stateB.Naked {
		t.Errorf("expected missing job-b Naked (within budget), got %+v", stateB)
	}
	if len(pipeline.savedInputs) != 1 {
		t.Errorf("expected job-a saved, got %d", len(pipeline.savedInputs))
	}

	// Now exhausted: job-b retryCount=3 → DLQ cause=llm_result_missing.
	fjA2, _ := makeTestFetchedJob(t, ats.RawJob{ID: "job-a"}, 0)
	fjB2, stateB2 := makeTestFetchedJob(t, ats.RawJob{ID: "job-b"}, 3)
	before := dlqCount("llm_result_missing")
	w.processGroup(context.Background(), logger.NewNoop(), []fetchedJob{fjA2, fjB2}, "fetch-1", 1, 2)
	if got := dlqCount("llm_result_missing") - before; got != 1 {
		t.Errorf("expected enricher_dlq_publishes_total{cause=llm_result_missing} +1, got %v", got)
	}
	if !stateB2.Termed {
		t.Errorf("expected exhausted job-b2 Termed, got %+v", stateB2)
	}
}

// Group outcome label precedence.
func TestGroupOutcomeStats_LabelPrecedence(t *testing.T) {
	cases := []struct {
		name  string
		stats groupOutcomeStats
		want  string
	}{
		{"error wins over all", groupOutcomeStats{saved: 1, dropped: 1, routeFailures: 1}, "error"},
		{"mixed saved+dropped", groupOutcomeStats{saved: 1, dropped: 1}, "mixed"},
		{"all saved", groupOutcomeStats{saved: 3}, "saved"},
		{"all dropped", groupOutcomeStats{dropped: 3}, "dropped"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.stats.label(); got != tc.want {
				t.Errorf("label: got %q want %q", got, tc.want)
			}
		})
	}
}

// ── processFetch tests ────────────────────────────────────────────────────────

// 3 msgs, batchSize=1 → 3 groups → 2 inter-group sleeps; all Nak'd (missing results).
func TestProcessFetch_SerialGroupSleepsAndNaks(t *testing.T) {
	scorer := &fakeScorer{results: []*llmdto.JobPostEnrichResult{}}
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(scorer, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{}, clk, 1)

	msg1, state1 := makeTestMsg(t, ats.RawJob{ID: "job-1", FullContent: &atstypes.FullJobContent{Description: "x"}}, 1)
	msg2, state2 := makeTestMsg(t, ats.RawJob{ID: "job-2", FullContent: &atstypes.FullJobContent{Description: "x"}}, 1)
	msg3, state3 := makeTestMsg(t, ats.RawJob{ID: "job-3", FullContent: &atstypes.FullJobContent{Description: "x"}}, 1)

	if err := w.processFetch(context.Background(), []*js.Msg{msg1, msg2, msg3}); err != nil {
		t.Fatalf("processFetch: %v", err)
	}

	if len(clk.sleeps) != 2 {
		t.Errorf("expected 2 inter-group sleeps, got %d", len(clk.sleeps))
	}
	for _, d := range clk.sleeps {
		if d != w.llmCallInterval {
			t.Errorf("expected sleep=%v, got %v", w.llmCallInterval, d)
		}
	}
	// All Nak'd (missing result → Nak within budget since deliveryCount=1 → retryCount=0).
	for i, st := range []*js.TestMsgState{state1, state2, state3} {
		if !st.Naked {
			t.Errorf("msg %d: expected Naked (missing result), got %+v", i+1, st)
		}
	}
}

// Single-group fetch: no sleep; one batch; all msgs Nak'd (missing results).
func TestProcessFetch_SingleGroupNoSleep(t *testing.T) {
	scorer := &fakeScorer{results: []*llmdto.JobPostEnrichResult{}}
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(scorer, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{}, clk, 10)

	msg1, state1 := makeTestMsg(t, ats.RawJob{ID: "job-1", FullContent: &atstypes.FullJobContent{Description: "x"}}, 1)
	msg2, state2 := makeTestMsg(t, ats.RawJob{ID: "job-2", FullContent: &atstypes.FullJobContent{Description: "x"}}, 1)

	if err := w.processFetch(context.Background(), []*js.Msg{msg1, msg2}); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	if len(clk.sleeps) != 0 {
		t.Errorf("expected no sleeps for single group, got %d", len(clk.sleeps))
	}
	if !state1.Naked || !state2.Naked {
		t.Errorf("expected both msgs Naked; state1=%+v state2=%+v", state1, state2)
	}
}

// Malformed messages: Term'd immediately in Stage 1, no groups processed.
func TestProcessFetch_MalformedTerminated(t *testing.T) {
	scorer := &fakeScorer{}
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(scorer, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{}, clk, 5)

	msg1, state1 := js.NewTestMsg([]byte("not json"), 1)
	msg2, state2 := js.NewTestMsg([]byte("{{{"), 1)

	if err := w.processFetch(context.Background(), []*js.Msg{msg1, msg2}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if !state1.Termed || !state2.Termed {
		t.Errorf("expected both malformed msgs Termed; state1=%+v state2=%+v", state1, state2)
	}
	if scorer.calls != 0 {
		t.Errorf("expected no EnrichBatch calls for all-malformed batch, got %d", scorer.calls)
	}
}

// Ctx cancel mid-sleep: exactly one group processed (Nak'd), remaining untouched.
// fetchAll uses goroutines so group order is non-deterministic — check counts, not identity.
func TestProcessFetch_CtxCancelMidSleep(t *testing.T) {
	scorer := &fakeScorer{results: []*llmdto.JobPostEnrichResult{}}
	clk := &fakeClock{now: time.Now(), sleepErrOn: 1}
	w := makeWorkerFull(scorer, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{}, clk, 1)

	msg1, state1 := makeTestMsg(t, ats.RawJob{ID: "job-1", FullContent: &atstypes.FullJobContent{Description: "x"}}, 1)
	msg2, state2 := makeTestMsg(t, ats.RawJob{ID: "job-2", FullContent: &atstypes.FullJobContent{Description: "x"}}, 1)
	msg3, state3 := makeTestMsg(t, ats.RawJob{ID: "job-3", FullContent: &atstypes.FullJobContent{Description: "x"}}, 1)

	if err := w.processFetch(context.Background(), []*js.Msg{msg1, msg2, msg3}); err != nil {
		t.Fatalf("expected nil on ctx cancel, got %v", err)
	}

	// Exactly one group (one msg) should have been Nak'd before sleep cancelled.
	// fetchAll runs goroutines so the group order is non-deterministic.
	states := []*js.TestMsgState{state1, state2, state3}
	var nakedCount, disposedCount int
	for _, st := range states {
		if st.Naked {
			nakedCount++
		}
		if st.Acked || st.Termed {
			disposedCount++
		}
	}
	if nakedCount != 1 {
		t.Errorf("expected exactly 1 msg Naked (first group processed), got %d", nakedCount)
	}
	if disposedCount != 0 {
		t.Errorf("expected no Acked/Termed msgs after sleep cancel, got %d", disposedCount)
	}
}

// Ctx cancel mid-EnrichBatch: handleFailure short-circuits → msg untouched.
func TestProcessFetch_CtxCancelMidEnrichBatch(t *testing.T) {
	scorer := &fakeScorer{err: context.Canceled}
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(scorer, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{}, clk, 5)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	msg, state := makeTestMsg(t, ats.RawJob{ID: "job-1", FullContent: &atstypes.FullJobContent{Description: "x"}}, 1)
	_ = w.processFetch(ctx, []*js.Msg{msg})

	if state.Naked || state.Acked || state.Termed {
		t.Errorf("expected msg untouched on ctx cancel, got %+v", state)
	}
}

// UpdateRetryBatch transient failure then success → zero EnricherRetryLedgerFailuresTotal.
func TestHandleFailure_LedgerInlineRetrySucceeds(t *testing.T) {
	publisher := &fakePublisher{}
	ledger := &fakeLedger{failureQueue: []error{stderrors.New("db down")}}
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	before := ledgerFailures()
	fj, state := makeTestFetchedJob(t, baseRawJob(), 0)
	ok := w.handleFailure(context.Background(), logger.NewNoop(), fj, "enrich", stderrors.New("boom"))
	if !ok {
		t.Fatalf("handleFailure returned false")
	}
	if !state.Naked {
		t.Errorf("expected msg Naked, got %+v", state)
	}
	if len(ledger.retryBatches) != 1 {
		t.Errorf("expected one successful UpdateRetryBatch after inline retry, got %d", len(ledger.retryBatches))
	}
	if got := ledgerFailures() - before; got != 0 {
		t.Errorf("expected zero ledger failure counter increments, got %v", got)
	}
}

// UpdateRetryBatch double failure → EnricherRetryLedgerFailuresTotal+1, worker continues.
func TestHandleFailure_LedgerInlineRetryExhausted(t *testing.T) {
	publisher := &fakePublisher{}
	ledger := &fakeLedger{failureQueue: []error{stderrors.New("db1"), stderrors.New("db2")}}
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	before := ledgerFailures()
	fj, state := makeTestFetchedJob(t, baseRawJob(), 0)
	ok := w.handleFailure(context.Background(), logger.NewNoop(), fj, "enrich", stderrors.New("boom"))
	if !ok {
		t.Fatalf("handleFailure returned false on ledger failure; NATS is authoritative")
	}
	if !state.Naked {
		t.Errorf("expected msg Naked, got %+v", state)
	}
	if got := ledgerFailures() - before; got != 1 {
		t.Errorf("expected ledger failure counter +1, got %v", got)
	}
}

// DLQ publish failure → sendToDLQ returns false; msg NOT Termed.
func TestHandleFailure_DLQPublishFailureReturnsFalse(t *testing.T) {
	publisher := &fakePublisher{err: stderrors.New("nats down")}
	ledger := &fakeLedger{}
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	// retryCount=3 → exhausted → sendToDLQ → publish fails → return false.
	fj, state := makeTestFetchedJob(t, baseRawJob(), 3)
	ok := w.handleFailure(context.Background(), logger.NewNoop(), fj, "enrich", stderrors.New("transient"))
	if ok {
		t.Errorf("expected handleFailure=false on DLQ publish error")
	}
	if state.Termed {
		t.Errorf("msg must NOT be Termed if DLQ publish failed (avoid losing message)")
	}
	if len(ledger.retryBatches) != 0 {
		t.Errorf("ledger should not be updated when DLQ publish fails; got %v", ledger.retryBatches)
	}
}

// realClock.Sleep(0) returns immediately.
func TestRealClock_SleepZero(t *testing.T) {
	c := realClock{}
	start := time.Now()
	if err := c.Sleep(context.Background(), 0); err != nil {
		t.Fatalf("Sleep(0): %v", err)
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Errorf("Sleep(0) should return immediately")
	}
}

// ── metric read helpers ───────────────────────────────────────────────────────

func dlqCount(cause string) float64 {
	return counterValue(metrics.EnricherDLQPublishesTotal.WithLabelValues(cause))
}

func ledgerFailures() float64 {
	return counterValue(metrics.EnricherRetryLedgerFailuresTotal)
}

// counterValue reads a prometheus.Counter's current value.
func counterValue(c prometheus.Counter) float64 {
	var m dto.Metric
	_ = c.Write(&m)
	if m.Counter != nil && m.Counter.Value != nil {
		return *m.Counter.Value
	}
	return 0
}
