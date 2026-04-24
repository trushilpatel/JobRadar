package main

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	atstypes "github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	domainerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
	"github.com/twmb/franz-go/pkg/kgo"
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

// Compile-time assertion: fakePipelineService must satisfy the full
// PipelineService interface used by the worker.
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

// publishedMsg captures a single call to the fake publisher so tests can
// inspect the topic, payload, and headers routed on failure.
type publishedMsg struct {
	Topic   string
	Value   []byte
	Headers []kgo.RecordHeader
}

type fakePublisher struct {
	mu       sync.Mutex
	err      error
	messages []publishedMsg
}

func (f *fakePublisher) Publish(_ context.Context, topic string, value []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.messages = append(f.messages, publishedMsg{Topic: topic, Value: value})
	return nil
}

func (f *fakePublisher) PublishWithHeaders(_ context.Context, topic string, value []byte, headers []kgo.RecordHeader) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	dup := make([]kgo.RecordHeader, len(headers))
	copy(dup, headers)
	f.messages = append(f.messages, publishedMsg{Topic: topic, Value: value, Headers: dup})
	return nil
}

func (f *fakePublisher) messagesOnTopic(topic string) []publishedMsg {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []publishedMsg
	for _, m := range f.messages {
		if m.Topic == topic {
			out = append(out, m)
		}
	}
	return out
}

// committedBatch records one CommitRecords call.
type committedBatch struct {
	records []*kgo.Record
}

type fakeCommitter struct {
	mu       sync.Mutex
	batches  []committedBatch
	errQueue []error
}

func (f *fakeCommitter) CommitRecords(_ context.Context, records ...*kgo.Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	dup := make([]*kgo.Record, len(records))
	copy(dup, records)
	f.batches = append(f.batches, committedBatch{records: dup})
	if len(f.errQueue) > 0 {
		e := f.errQueue[0]
		f.errQueue = f.errQueue[1:]
		return e
	}
	return nil
}

type fakeClock struct {
	mu       sync.Mutex
	now      time.Time
	sleeps   []time.Duration
	sleepErr error
	// When true, returns ctx.Err() only on the Nth sleep (1-indexed).
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

func makeBaseOptions(scorer llmEnricher, pipeline jobsvc.PipelineService, publisher kafka.Publisher, ledger EnricherLedger) WorkerOptions {
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
	}
}

// llmEnricher is a local alias matching the scorer interface. Lets the
// helpers accept our fakes without pulling in llmcore just for the name.
type llmEnricher = interface {
	EnrichBatch(ctx context.Context, inputs []llmdto.JobPostEnrichInput) ([]*llmdto.JobPostEnrichResult, error)
}

func makeWorker(scorer llmEnricher, pipeline jobsvc.PipelineService, publisher kafka.Publisher, ledger EnricherLedger) *Worker {
	w := NewWorker(makeBaseOptions(scorer, pipeline, publisher, ledger))
	return w
}

// makeWorkerFull wires a worker with a fake committer + fake clock for
// processFetch-level tests.
func makeWorkerFull(scorer llmEnricher, pipeline jobsvc.PipelineService, publisher kafka.Publisher, ledger EnricherLedger, committer recordCommitter, clk clock, llmBatchSize int) *Worker {
	opts := makeBaseOptions(scorer, pipeline, publisher, ledger)
	opts.LLMBatchSize = llmBatchSize
	w := NewWorker(opts)
	w.committer = committer
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

// makeRawRecord builds a kgo.Record with a JSON-encoded job and the retry_count
// header. Used to drive processFetch end-to-end.
func makeRawRecord(t *testing.T, job ats.RawJob, retryCount int, partition int32, offset int64) *kgo.Record {
	t.Helper()
	payload, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	headers := []kgo.RecordHeader{
		{Key: kafka.HeaderRetryCount, Value: []byte(strconv.Itoa(retryCount))},
	}
	return &kgo.Record{
		Value:     payload,
		Headers:   headers,
		Partition: partition,
		Offset:    offset,
	}
}

// makeFetchedJob produces a fetchedJob around a RawJob with an attached record
// for direct processGroup tests.
func makeFetchedJob(t *testing.T, job ats.RawJob, retryCount int, partition int32, offset int64) fetchedJob {
	return fetchedJob{
		job:        job,
		content:    ats.FetchJobContentResponse{ID: job.ID, Title: job.Title},
		record:     makeRawRecord(t, job, retryCount, partition, offset),
		retryCount: retryCount,
		ctx:        context.Background(),
	}
}

// marshalJob is a helper to produce the Kafka message body the worker receives.
func marshalJob(t *testing.T, job ats.RawJob) []byte {
	t.Helper()
	b, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("marshal job: %v", err)
	}
	return b
}

// ── toEnrichedInput / dropJob tests (retained) ────────────────────────────────

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

// ── FullContent / skip-fetch tests (retained) ────────────────────────────────

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

	fj := fetchedJob{
		job:     job,
		content: content,
		record:  &kgo.Record{Value: marshalJob(t, job)},
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

// ── handleFailure / sendToDLQ tests ───────────────────────────────────────────

// Stage 2 transient error → routes to retry tier AND writes retry ledger.
func TestHandleFailure_Stage2TransientRoutesToRetryAndLedger(t *testing.T) {
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, publisher, ledger)
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	w.clock = &fakeClock{now: now}

	job := baseRawJob()
	ok := w.handleFailure(context.Background(), logger.NewNoop(), job, marshalJob(t, job), 0, "enrich", stderrors.New("LLM timeout"))
	if !ok {
		t.Fatalf("handleFailure returned false unexpectedly")
	}
	if len(publisher.messagesOnTopic(kafka.TopicJobListingsFilteredRetry1m)) != 1 {
		t.Errorf("expected 1 publish on retry.1m")
	}
	if len(ledger.retryBatches) != 1 || ledger.retryBatches[0][0].RetryCount != 1 {
		t.Errorf("expected UpdateRetryBatch with RetryCount=1, got %+v", ledger.retryBatches)
	}
	if !ledger.retryBatches[0][0].NextAttemptAt.Equal(now.Add(time.Minute)) {
		t.Errorf("expected NextAttemptAt=now+1m, got %v", ledger.retryBatches[0][0].NextAttemptAt)
	}
}

// Stage 1 transient error → retry tier, NO ledger write.
func TestHandleFailure_Stage1TransientNoLedgerWrite(t *testing.T) {
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	job := baseRawJob()
	ok := w.handleFailure(context.Background(), logger.NewNoop(), job, marshalJob(t, job), 0, "fetch", stderrors.New("http 500"))
	if !ok {
		t.Fatalf("handleFailure returned false")
	}
	if len(publisher.messagesOnTopic(kafka.TopicJobListingsFilteredRetry1m)) != 1 {
		t.Errorf("expected retry.1m publish")
	}
	if len(ledger.retryBatches) != 0 {
		t.Errorf("expected no UpdateRetryBatch on stage=fetch, got %+v", ledger.retryBatches)
	}
}

// Stage 1 exhausted → DLQ with cause=fetch_failed.
func TestHandleFailure_Stage1ExhaustedDLQFetchFailed(t *testing.T) {
	publisher := &fakePublisher{}
	pipeline := &fakePipelineService{}
	ledger := &fakeLedger{}
	w := makeWorker(&fakeScorer{}, pipeline, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	job := baseRawJob()
	before := dlqCount("fetch_failed")
	ok := w.handleFailure(context.Background(), logger.NewNoop(), job, marshalJob(t, job), 3, "fetch", stderrors.New("http 500"))
	if !ok {
		t.Fatalf("handleFailure returned false")
	}
	if len(publisher.messagesOnTopic(kafka.TopicJobListingsFilteredDLQ)) != 1 {
		t.Errorf("expected DLQ publish")
	}
	if got := dlqCount("fetch_failed") - before; got != 1 {
		t.Errorf("expected enricher_dlq_publishes_total{cause=fetch_failed} +1, got %v", got)
	}
	if len(pipeline.markCalls) != 1 {
		t.Errorf("expected ledger markError, got %v", pipeline.markCalls)
	}
}

// Validation error → DLQ with cause=validation, no retry ledger write.
func TestHandleFailure_ValidationDLQNoLedger(t *testing.T) {
	publisher := &fakePublisher{}
	pipeline := &fakePipelineService{}
	ledger := &fakeLedger{}
	w := makeWorker(&fakeScorer{}, pipeline, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	job := baseRawJob()
	before := dlqCount("validation")
	ok := w.handleFailure(context.Background(), logger.NewNoop(), job, marshalJob(t, job), 0, "enrich", domainerr.ErrLLMValidation)
	if !ok {
		t.Fatalf("handleFailure returned false")
	}
	if got := dlqCount("validation") - before; got != 1 {
		t.Errorf("expected enricher_dlq_publishes_total{cause=validation} +1, got %v", got)
	}
	if len(ledger.retryBatches) != 0 {
		t.Errorf("expected no retry ledger writes for validation")
	}
}

// Context cancel → skip all publishes, returns false.
func TestHandleFailure_ContextCanceledSkipsPublish(t *testing.T) {
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ok := w.handleFailure(ctx, logger.NewNoop(), baseRawJob(), []byte("value"), 0, "enrich", context.Canceled)
	if ok {
		t.Errorf("handleFailure should return false on ctx cancel")
	}
	if len(publisher.messages) != 0 {
		t.Errorf("expected no publishes, got %d", len(publisher.messages))
	}
	if len(pipeline.markCalls) != 0 {
		t.Errorf("expected no ledger marks, got %v", pipeline.markCalls)
	}
}

// ── processGroup / routeBatchFailure tests ────────────────────────────────────

// Stage 2 whole-group EnrichBatch failure → per-job retry via UpdateRetryBatch;
// label=error; tier=1m; stage=enrich.
func TestProcessGroup_EnrichBatchFailureWholeGroupRetries(t *testing.T) {
	scorer := &fakeScorer{err: stderrors.New("LLM timeout")}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	w := makeWorker(scorer, pipeline, publisher, ledger)
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	w.clock = &fakeClock{now: now}

	group := []fetchedJob{
		makeFetchedJob(t, ats.RawJob{ID: "job-1", Title: "A"}, 0, 0, 1),
		makeFetchedJob(t, ats.RawJob{ID: "job-2", Title: "B"}, 0, 0, 2),
	}
	routed := w.processGroup(context.Background(), logger.NewNoop(), group, "fetch-1", 0, 1)
	if !routed {
		t.Fatalf("processGroup should return true on successful retry routing")
	}

	retries := publisher.messagesOnTopic(kafka.TopicJobListingsFilteredRetry1m)
	if len(retries) != 2 {
		t.Fatalf("expected 2 retry publishes, got %d", len(retries))
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

// Mixed retry_count in one group under transient error: tier-eligible retried,
// exhausted DLQ'd.
func TestProcessGroup_MixedRetryCounts(t *testing.T) {
	scorer := &fakeScorer{err: stderrors.New("LLM timeout")}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	w := makeWorker(scorer, pipeline, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	group := []fetchedJob{
		makeFetchedJob(t, ats.RawJob{ID: "job-a"}, 0, 0, 1),
		makeFetchedJob(t, ats.RawJob{ID: "job-b"}, 3, 0, 2),
	}
	w.processGroup(context.Background(), logger.NewNoop(), group, "fetch-1", 0, 1)

	if len(publisher.messagesOnTopic(kafka.TopicJobListingsFilteredRetry1m)) != 1 {
		t.Errorf("expected 1 retry publish for tier-eligible job")
	}
	if len(publisher.messagesOnTopic(kafka.TopicJobListingsFilteredDLQ)) != 1 {
		t.Errorf("expected 1 DLQ publish for exhausted job")
	}
	if len(ledger.retryBatches) != 1 {
		t.Errorf("expected UpdateRetryBatch only for tier-eligible, got %d", len(ledger.retryBatches))
	}
}

// Validation error → all jobs DLQ'd cause=validation, no retry ledger.
func TestProcessGroup_ValidationErrorDLQAll(t *testing.T) {
	scorer := &fakeScorer{err: domainerr.ErrLLMValidation}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	w := makeWorker(scorer, pipeline, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	group := []fetchedJob{
		makeFetchedJob(t, ats.RawJob{ID: "job-1"}, 0, 0, 1),
		makeFetchedJob(t, ats.RawJob{ID: "job-2"}, 0, 0, 2),
	}
	before := dlqCount("validation")
	w.processGroup(context.Background(), logger.NewNoop(), group, "fetch-1", 0, 1)

	if got := dlqCount("validation") - before; got != 2 {
		t.Errorf("expected enricher_dlq_publishes_total{cause=validation} +2, got %v", got)
	}
	if len(ledger.retryBatches) != 0 {
		t.Errorf("expected no retry ledger writes, got %+v", ledger.retryBatches)
	}
}

// Missing LLM result → retry tier (within budget) then DLQ cause=llm_result_missing at exhausted budget.
func TestProcessGroup_MissingResultRouting(t *testing.T) {
	// Return a result only for job-a; job-b missing.
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

	group := []fetchedJob{
		makeFetchedJob(t, ats.RawJob{ID: "job-a"}, 0, 0, 1),
		makeFetchedJob(t, ats.RawJob{ID: "job-b"}, 0, 0, 2),
	}
	w.processGroup(context.Background(), logger.NewNoop(), group, "fetch-1", 0, 1)

	if len(publisher.messagesOnTopic(kafka.TopicJobListingsFilteredRetry1m)) != 1 {
		t.Errorf("expected missing job routed to retry.1m")
	}
	if len(pipeline.savedInputs) != 1 {
		t.Errorf("expected job-a saved, got %d", len(pipeline.savedInputs))
	}

	// Now exhausted: set retryCount=3 → cause=llm_result_missing DLQ.
	group2 := []fetchedJob{
		makeFetchedJob(t, ats.RawJob{ID: "job-a"}, 0, 0, 3),
		makeFetchedJob(t, ats.RawJob{ID: "job-b"}, 3, 0, 4),
	}
	before := dlqCount("llm_result_missing")
	w.processGroup(context.Background(), logger.NewNoop(), group2, "fetch-1", 1, 2)
	if got := dlqCount("llm_result_missing") - before; got != 1 {
		t.Errorf("expected enricher_dlq_publishes_total{cause=llm_result_missing} +1, got %v", got)
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

// N scoreable groups: N-1 sleeps of llmCallInterval each, CommitRecords called N times.
func TestProcessFetch_SerialGroupExecutionAndCommits(t *testing.T) {
	scorer := &fakeScorer{results: []*llmdto.JobPostEnrichResult{}} // all missing → routed as retry but we just care about commit shape
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	commit := &fakeCommitter{}
	clk := &fakeClock{now: time.Now()}

	// batchSize=1 → 3 records produce 3 groups → 2 inter-group sleeps.
	w := makeWorkerFull(scorer, pipeline, publisher, ledger, commit, clk, 1)

	records := []*kgo.Record{
		makeRawRecord(t, ats.RawJob{ID: "job-1", FullContent: &atstypes.FullJobContent{Description: "x"}}, 0, 0, 10),
		makeRawRecord(t, ats.RawJob{ID: "job-2", FullContent: &atstypes.FullJobContent{Description: "x"}}, 0, 0, 11),
		makeRawRecord(t, ats.RawJob{ID: "job-3", FullContent: &atstypes.FullJobContent{Description: "x"}}, 0, 0, 12),
	}

	if err := w.processFetch(context.Background(), records); err != nil {
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
	if len(commit.batches) != 3 {
		t.Fatalf("expected 3 CommitRecords calls, got %d", len(commit.batches))
	}
	for i, want := range []int64{10, 11, 12} {
		got := commit.batches[i].records
		if len(got) != 1 || got[0].Offset != want {
			t.Errorf("commit batch %d: expected offset %d, got %+v", i, want, got)
		}
	}
}

// Single-group fetch: no sleep; one CommitRecords call.
func TestProcessFetch_SingleGroupNoSleep(t *testing.T) {
	scorer := &fakeScorer{results: []*llmdto.JobPostEnrichResult{}}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	commit := &fakeCommitter{}
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(scorer, pipeline, publisher, ledger, commit, clk, 10)

	records := []*kgo.Record{
		makeRawRecord(t, ats.RawJob{ID: "job-1", FullContent: &atstypes.FullJobContent{Description: "x"}}, 0, 0, 10),
		makeRawRecord(t, ats.RawJob{ID: "job-2", FullContent: &atstypes.FullJobContent{Description: "x"}}, 0, 0, 11),
	}

	if err := w.processFetch(context.Background(), records); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	if len(clk.sleeps) != 0 {
		t.Errorf("expected no sleeps, got %d", len(clk.sleeps))
	}
	if len(commit.batches) != 1 {
		t.Errorf("expected 1 commit, got %d", len(commit.batches))
	}
}

// Zero fetched (all Stage 1 failures): returns nil, no commits.
func TestProcessFetch_ZeroFetchedNoCommits(t *testing.T) {
	scorer := &fakeScorer{}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	commit := &fakeCommitter{}
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(scorer, pipeline, publisher, ledger, commit, clk, 5)

	// Malformed records — all rejected in fetchOne.
	records := []*kgo.Record{
		{Value: []byte("not json"), Partition: 0, Offset: 1},
		{Value: []byte("{{{"), Partition: 0, Offset: 2},
	}
	if err := w.processFetch(context.Background(), records); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if len(commit.batches) != 0 {
		t.Errorf("expected no commit attempts, got %d", len(commit.batches))
	}
}

// Ctx cancel mid-sleep: loop exits; only the already-committed first group remains committed.
func TestProcessFetch_CtxCancelMidSleep(t *testing.T) {
	scorer := &fakeScorer{results: []*llmdto.JobPostEnrichResult{}}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	commit := &fakeCommitter{}
	clk := &fakeClock{now: time.Now(), sleepErrOn: 1}
	w := makeWorkerFull(scorer, pipeline, publisher, ledger, commit, clk, 1)

	records := []*kgo.Record{
		makeRawRecord(t, ats.RawJob{ID: "job-1", FullContent: &atstypes.FullJobContent{Description: "x"}}, 0, 0, 10),
		makeRawRecord(t, ats.RawJob{ID: "job-2", FullContent: &atstypes.FullJobContent{Description: "x"}}, 0, 0, 11),
		makeRawRecord(t, ats.RawJob{ID: "job-3", FullContent: &atstypes.FullJobContent{Description: "x"}}, 0, 0, 12),
	}
	if err := w.processFetch(context.Background(), records); err != nil {
		t.Fatalf("expected nil on ctx cancel, got %v", err)
	}
	if len(commit.batches) != 1 {
		t.Errorf("expected exactly 1 commit before ctx cancel, got %d", len(commit.batches))
	}
}

// Ctx cancel mid-EnrichBatch (scorer returns ctx.Canceled) → processGroup
// returns routed=false → CommitRecords NOT called for that group.
func TestProcessFetch_CtxCancelMidEnrichBatchSkipsCommit(t *testing.T) {
	// Scorer returns context.Canceled as if EnrichBatch was cancelled.
	// Then handleFailure inside routeBatchFailure short-circuits (publish
	// blocked by ctx.Err() check inside handleFailure).
	scorer := &fakeScorer{err: context.Canceled}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	commit := &fakeCommitter{}
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(scorer, pipeline, publisher, ledger, commit, clk, 5)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before call — handleFailure short-circuits, publish_failed > 0.

	records := []*kgo.Record{
		makeRawRecord(t, ats.RawJob{ID: "job-1", FullContent: &atstypes.FullJobContent{Description: "x"}}, 0, 0, 10),
	}
	_ = w.processFetch(ctx, records)

	if len(commit.batches) != 0 {
		t.Errorf("expected NO commit when ctx cancelled mid-batch, got %d", len(commit.batches))
	}
}

// CommitRecords error: counter incremented, next group still runs.
func TestProcessFetch_CommitRecordsErrorContinues(t *testing.T) {
	scorer := &fakeScorer{results: []*llmdto.JobPostEnrichResult{}}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	commit := &fakeCommitter{errQueue: []error{stderrors.New("commit boom")}}
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(scorer, pipeline, publisher, ledger, commit, clk, 1)

	records := []*kgo.Record{
		makeRawRecord(t, ats.RawJob{ID: "job-1", FullContent: &atstypes.FullJobContent{Description: "x"}}, 0, 0, 10),
		makeRawRecord(t, ats.RawJob{ID: "job-2", FullContent: &atstypes.FullJobContent{Description: "x"}}, 0, 0, 11),
	}
	before := commitFailures()
	if err := w.processFetch(context.Background(), records); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	if len(commit.batches) != 2 {
		t.Errorf("expected 2 commit attempts, got %d", len(commit.batches))
	}
	if got := commitFailures() - before; got != 1 {
		t.Errorf("expected commit failure counter +1, got %v", got)
	}
}

// UpdateRetryBatch transient failure then success → zero EnricherRetryLedgerFailuresTotal.
func TestHandleFailure_LedgerInlineRetrySucceeds(t *testing.T) {
	publisher := &fakePublisher{}
	// First call fails, second succeeds.
	ledger := &fakeLedger{failureQueue: []error{stderrors.New("db down")}}
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, publisher, ledger)
	w.clock = &fakeClock{now: time.Now()}

	before := ledgerFailures()
	job := baseRawJob()
	ok := w.handleFailure(context.Background(), logger.NewNoop(), job, marshalJob(t, job), 0, "enrich", stderrors.New("boom"))
	if !ok {
		t.Fatalf("handleFailure returned false")
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
	job := baseRawJob()
	ok := w.handleFailure(context.Background(), logger.NewNoop(), job, marshalJob(t, job), 0, "enrich", stderrors.New("boom"))
	if !ok {
		t.Fatalf("handleFailure returned false on ledger failure; Kafka is authoritative")
	}
	if got := ledgerFailures() - before; got != 1 {
		t.Errorf("expected ledger failure counter +1, got %v", got)
	}
}

// Partial-partition group: CommitRecords sees records from both partitions.
func TestProcessFetch_PartialPartitionGroup(t *testing.T) {
	scorer := &fakeScorer{results: []*llmdto.JobPostEnrichResult{}}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	commit := &fakeCommitter{}
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(scorer, pipeline, publisher, ledger, commit, clk, 5)

	records := []*kgo.Record{
		makeRawRecord(t, ats.RawJob{ID: "job-a", FullContent: &atstypes.FullJobContent{Description: "x"}}, 0, 0, 10),
		makeRawRecord(t, ats.RawJob{ID: "job-b", FullContent: &atstypes.FullJobContent{Description: "x"}}, 0, 1, 20),
	}
	if err := w.processFetch(context.Background(), records); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	if len(commit.batches) != 1 {
		t.Fatalf("expected 1 commit batch, got %d", len(commit.batches))
	}
	got := commit.batches[0].records
	if len(got) != 2 {
		t.Fatalf("expected 2 records committed, got %d", len(got))
	}
	partitions := map[int32]int64{}
	for _, r := range got {
		partitions[r.Partition] = r.Offset
	}
	if partitions[0] != 10 || partitions[1] != 20 {
		t.Errorf("expected {0:10, 1:20}, got %+v", partitions)
	}
}

// sortFetched unit test: shuffled input across 2 partitions → sorted by (partition, offset).
func TestSortFetched_Ordering(t *testing.T) {
	in := []fetchedJob{
		{record: &kgo.Record{Partition: 1, Offset: 5}},
		{record: &kgo.Record{Partition: 0, Offset: 10}},
		{record: &kgo.Record{Partition: 1, Offset: 2}},
		{record: &kgo.Record{Partition: 0, Offset: 1}},
	}
	sortFetched(in)
	want := []struct {
		p int32
		o int64
	}{
		{0, 1}, {0, 10}, {1, 2}, {1, 5},
	}
	for i, w := range want {
		if in[i].record.Partition != w.p || in[i].record.Offset != w.o {
			t.Errorf("pos %d: want (%d,%d) got (%d,%d)", i, w.p, w.o, in[i].record.Partition, in[i].record.Offset)
		}
	}
}

// End-to-end: shuffled Stage 1 outputs → per-partition commits monotonically advance.
func TestProcessFetch_OffsetOrderingMonotonic(t *testing.T) {
	scorer := &fakeScorer{results: []*llmdto.JobPostEnrichResult{}}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	ledger := &fakeLedger{}
	commit := &fakeCommitter{}
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(scorer, pipeline, publisher, ledger, commit, clk, 1)

	// Deliberately shuffled offsets per partition.
	records := []*kgo.Record{
		makeRawRecord(t, ats.RawJob{ID: "p0-b", FullContent: &atstypes.FullJobContent{Description: "x"}}, 0, 0, 20),
		makeRawRecord(t, ats.RawJob{ID: "p1-a", FullContent: &atstypes.FullJobContent{Description: "x"}}, 0, 1, 5),
		makeRawRecord(t, ats.RawJob{ID: "p0-a", FullContent: &atstypes.FullJobContent{Description: "x"}}, 0, 0, 10),
		makeRawRecord(t, ats.RawJob{ID: "p1-b", FullContent: &atstypes.FullJobContent{Description: "x"}}, 0, 1, 15),
	}
	if err := w.processFetch(context.Background(), records); err != nil {
		t.Fatalf("processFetch: %v", err)
	}

	// Per-partition last offset seen must be monotonic across commits.
	lastOff := map[int32]int64{}
	for i, b := range commit.batches {
		for _, r := range b.records {
			if prev, ok := lastOff[r.Partition]; ok && r.Offset <= prev {
				t.Errorf("commit %d: partition %d offset %d not > prev %d", i, r.Partition, r.Offset, prev)
			}
			if r.Offset > lastOff[r.Partition] {
				lastOff[r.Partition] = r.Offset
			}
		}
	}
}

// llmCallInterval=0: no wall-clock wait. realClock.Sleep returns immediately.
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

// ── metric read helpers (go-prometheus) ──────────────────────────────────────

func dlqCount(cause string) float64 {
	return counterValue(metrics.EnricherDLQPublishesTotal.WithLabelValues(cause))
}

func commitFailures() float64 {
	return counterValue(metrics.EnricherCommitFailuresTotal)
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
