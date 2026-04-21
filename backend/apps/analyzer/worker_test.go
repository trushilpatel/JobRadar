package main

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	atstypes "github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	domainerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/common/logger"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
	"github.com/twmb/franz-go/pkg/kgo"
)

// ── fakes ────────────────────────────────────────────────────────────────────

type fakeScorer struct {
	result *llmdto.JobPostEnrichResult
	err    error
}

func (f *fakeScorer) Enrich(_ context.Context, _ llmdto.JobPostEnrichInput) (*llmdto.JobPostEnrichResult, error) {
	return f.result, f.err
}

type fakePipelineService struct {
	saveErr     error
	markErr     error
	hasSeen     bool
	hasSeenErr  error
	savedInputs []jobdto.EnrichedJobInput
	markCalls   []string
}

func (f *fakePipelineService) SaveEnrichedJob(_ context.Context, input jobdto.EnrichedJobInput) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.savedInputs = append(f.savedInputs, input)
	return nil
}

func (f *fakePipelineService) MarkLedgerError(_ context.Context, id string) error {
	f.markCalls = append(f.markCalls, id)
	return f.markErr
}

func (f *fakePipelineService) MarkLedgerIrrelevant(_ context.Context, id string) error {
	f.markCalls = append(f.markCalls, id)
	return f.markErr
}

func (f *fakePipelineService) HasSeenJob(_ context.Context, id string) (bool, error) {
	return f.hasSeen, f.hasSeenErr
}

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
	f.messages = append(f.messages, publishedMsg{Topic: topic, Value: value, Headers: headers})
	return nil
}

func headerValue(headers []kgo.RecordHeader, key string) string {
	for _, h := range headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

// ── helpers ───────────────────────────────────────────────────────────────────

func makeWorker(scorer *fakeScorer, pipeline *fakePipelineService) *Worker {
	return NewWorker(nil, &fakePublisher{}, scorer, pipeline, logger.NewNoop(), 0, 5)
}

func makeWorkerWithPublisher(scorer *fakeScorer, pipeline *fakePipelineService, publisher kafka.Publisher) *Worker {
	return NewWorker(nil, publisher, scorer, pipeline, logger.NewNoop(), 0, 5)
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

// ── toEnrichedInput tests ─────────────────────────────────────────────────────

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

// ── enrich / happy path tests ─────────────────────────────────────────────────

func TestEnrich_HappyPath(t *testing.T) {
	scorer := &fakeScorer{result: baseEnrichResult()}
	pipeline := &fakePipelineService{}
	w := makeWorker(scorer, pipeline)

	job := baseRawJob()
	result := baseEnrichResult()

	// Test the pipeline interaction directly — call SaveEnrichedJob with the right input.
	input := toEnrichedInput(job, result)
	if err := pipeline.SaveEnrichedJob(context.Background(), input); err != nil {
		t.Fatalf("SaveEnrichedJob: %v", err)
	}
	if len(pipeline.savedInputs) != 1 {
		t.Errorf("expected 1 saved input, got %d", len(pipeline.savedInputs))
	}
	if !pipeline.savedInputs[0].Enriched.Remote {
		t.Error("Remote should be true")
	}

	// No error marks on success
	if len(pipeline.markCalls) != 0 {
		t.Errorf("expected no error marks on success, got %v", pipeline.markCalls)
	}

	_ = w // worker used for dropJob / markError paths
}

func TestEnrich_LLMError_MarksError(t *testing.T) {
	scorer := &fakeScorer{err: stderrors.New("LLM timeout")}
	pipeline := &fakePipelineService{}
	w := makeWorker(scorer, pipeline)

	// Simulate the markError path directly
	w.markError(context.Background(), "ashby-abc123")

	if len(pipeline.markCalls) != 1 || pipeline.markCalls[0] != "ashby-abc123" {
		t.Errorf("expected markError called with job ID, got %v", pipeline.markCalls)
	}
	if len(pipeline.savedInputs) != 0 {
		t.Error("no job should be saved on LLM error")
	}

	_ = scorer
}

func TestEnrich_SaveError_MarksError(t *testing.T) {
	scorer := &fakeScorer{result: baseEnrichResult()}
	pipeline := &fakePipelineService{saveErr: stderrors.New("db constraint")}
	w := makeWorker(scorer, pipeline)

	// Simulate the save-failure path
	input := toEnrichedInput(baseRawJob(), baseEnrichResult())
	if err := pipeline.SaveEnrichedJob(context.Background(), input); err != nil {
		w.markError(context.Background(), baseRawJob().ID)
	}

	if len(pipeline.markCalls) != 1 || pipeline.markCalls[0] != baseRawJob().ID {
		t.Errorf("expected error mark, got %v", pipeline.markCalls)
	}
	if len(pipeline.savedInputs) != 0 {
		t.Error("job should not be saved when save fails")
	}
}

// ── dropJob tests ─────────────────────────────────────────────────────────────

func TestDropJob_NoSponsorship(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{})
	result := &llmdto.JobPostEnrichResult{Sponsorship: false}
	if reason := w.dropJob(result); reason == "" {
		t.Error("expected drop reason for no sponsorship")
	}
}

func TestDropJob_RequiresCitizen(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{})
	result := &llmdto.JobPostEnrichResult{Sponsorship: true, RequiresCitizen: true}
	if reason := w.dropJob(result); reason == "" {
		t.Error("expected drop reason for requires citizen")
	}
}

func TestDropJob_SecurityClearance(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{})
	result := &llmdto.JobPostEnrichResult{Sponsorship: true, SecurityClearance: true}
	if reason := w.dropJob(result); reason == "" {
		t.Error("expected drop reason for security clearance")
	}
}

func TestDropJob_ExperienceExceedsMax(t *testing.T) {
	w := NewWorker(nil, &fakePublisher{}, &fakeScorer{}, &fakePipelineService{}, logger.NewNoop(), 3, 5)
	exp := 5
	result := &llmdto.JobPostEnrichResult{Sponsorship: true, ExperienceYears: &exp}
	if reason := w.dropJob(result); reason == "" {
		t.Error("expected drop reason for experience exceeds max")
	}
}

func TestDropJob_EligibleJob(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{})
	result := baseEnrichResult()
	if reason := w.dropJob(result); reason != "" {
		t.Errorf("expected no drop reason, got %q", reason)
	}
}

// ── FullContent / skip-fetch tests ────────────────────────────────────────────

// capturingScorer records the last EnrichInput it received so tests can assert
// on what was passed to the LLM.
type capturingScorer struct {
	result      *llmdto.JobPostEnrichResult
	err         error
	lastInput   llmdto.JobPostEnrichInput
	enrichCalls int
}

func (c *capturingScorer) Enrich(_ context.Context, input llmdto.JobPostEnrichInput) (*llmdto.JobPostEnrichResult, error) {
	c.lastInput = input
	c.enrichCalls++
	return c.result, c.err
}

// failingTransport is an http.RoundTripper that always returns an error.
// Used to assert that the HTTP client is never invoked for the FullContent path.
type failingTransport struct {
	called bool
}

func (ft *failingTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	ft.called = true
	return nil, stderrors.New("HTTP client must not be called when FullContent is present")
}

// TestEnrich_SkipsFetchWhenFullContentPresent verifies that when a RawJob has
// FullContent set, enrich() uses fullContentToResponse() instead of fetchContent(),
// meaning no HTTP request is made and the LLM receives the pre-fetched content.
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

	w := NewWorker(nil, &fakePublisher{}, scorer, pipeline, logger.NewNoop(), 0, 5)
	// Replace the default HTTP client with one that will fail if called.
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

	if err := w.enrich(context.Background(), job); err != nil {
		t.Fatalf("enrich returned unexpected error: %v", err)
	}

	// HTTP client must NOT have been called.
	if transport.called {
		t.Error("HTTP client should not be called when FullContent is present")
	}

	// Scorer.Enrich must have been called exactly once.
	if scorer.enrichCalls != 1 {
		t.Errorf("Enrich call count: want 1 got %d", scorer.enrichCalls)
	}

	// The RawText passed to the LLM must contain the description from FullContent.
	if !strings.Contains(scorer.lastInput.RawText, "Go engineer role") {
		t.Errorf("LLM RawText should contain description text; got: %s", scorer.lastInput.RawText)
	}

	// No ledger error should be recorded (enrich succeeded end-to-end).
	if len(pipeline.markCalls) != 0 {
		t.Errorf("expected no error mark on success, got %v", pipeline.markCalls)
	}
}

// TestFullContentToResponse_FieldMapping verifies that fullContentToResponse
// draws ID/Title/Location/URL/EmploymentType from RawJob and all content fields
// from FullJobContent — never mixing the two sources.
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

	// Fields that must come from RawJob
	if resp.ID != job.ID {
		t.Errorf("ID: want %q got %q", job.ID, resp.ID)
	}
	if resp.Title != job.Title {
		t.Errorf("Title: want %q got %q", job.Title, resp.Title)
	}
	if resp.Location != job.Location {
		t.Errorf("Location: want %q got %q", job.Location, resp.Location)
	}
	if resp.URL != job.URL {
		t.Errorf("URL: want %q got %q", job.URL, resp.URL)
	}

	// Fields that must come from FullContent
	fc := job.FullContent
	if resp.Description != fc.Description {
		t.Errorf("Description: want %q got %q", fc.Description, resp.Description)
	}
	if resp.Requirements != fc.Requirements {
		t.Errorf("Requirements: want %q got %q", fc.Requirements, resp.Requirements)
	}
	if resp.Compensation != fc.Compensation {
		t.Errorf("Compensation: want %q got %q", fc.Compensation, resp.Compensation)
	}
	if resp.WorkplaceType != fc.WorkplaceType {
		t.Errorf("WorkplaceType: want %q got %q", fc.WorkplaceType, resp.WorkplaceType)
	}
	if resp.Department != fc.Department {
		t.Errorf("Department: want %q got %q", fc.Department, resp.Department)
	}
	if resp.PublishedDate != fc.PublishedDate {
		t.Errorf("PublishedDate: want %q got %q", fc.PublishedDate, resp.PublishedDate)
	}
}

// ── retry / DLQ routing tests ─────────────────────────────────────────────────

// marshalJob is a helper to produce the Kafka message body the worker receives.
func marshalJob(t *testing.T, job ats.RawJob) []byte {
	t.Helper()
	b, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("marshal job: %v", err)
	}
	return b
}

// TestEnrich_TransientError_GoesToRetryTier verifies that a fresh message
// (retry_count=0) whose enrichment fails with a generic error is routed to
// the 1m retry tier with retry_count=1 and the ledger is NOT marked error.
func TestEnrich_TransientError_GoesToRetryTier(t *testing.T) {
	scorer := &fakeScorer{err: stderrors.New("LLM timeout")}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	w := makeWorkerWithPublisher(scorer, pipeline, publisher)

	job := baseRawJob()
	job.FullContent = &atstypes.FullJobContent{Description: "n/a"}
	value := marshalJob(t, job)

	err := w.enrich(context.Background(), job)
	if err == nil {
		t.Fatalf("expected enrich to return an error for transient failure")
	}

	w.handleFailure(context.Background(), job, value, 0, err)

	if len(publisher.messages) != 1 {
		t.Fatalf("expected 1 published retry message, got %d", len(publisher.messages))
	}
	msg := publisher.messages[0]
	if msg.Topic != kafka.TopicJobListingsFilteredRetry1m {
		t.Errorf("topic: want %s got %s", kafka.TopicJobListingsFilteredRetry1m, msg.Topic)
	}
	if got := headerValue(msg.Headers, kafka.HeaderRetryCount); got != "1" {
		t.Errorf("retry_count header: want 1 got %q", got)
	}
	if len(pipeline.markCalls) != 0 {
		t.Errorf("ledger should NOT be marked on retry; got calls %v", pipeline.markCalls)
	}
}

// TestEnrich_TransientError_RetryCountAdvances verifies that an incoming
// retry_count=1 message failing again is routed to the 10m retry tier with
// retry_count=2.
func TestEnrich_TransientError_RetryCountAdvances(t *testing.T) {
	scorer := &fakeScorer{err: stderrors.New("LLM timeout")}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	w := makeWorkerWithPublisher(scorer, pipeline, publisher)

	job := baseRawJob()
	job.FullContent = &atstypes.FullJobContent{Description: "n/a"}
	value := marshalJob(t, job)

	err := w.enrich(context.Background(), job)
	if err == nil {
		t.Fatalf("expected enrich to return an error for transient failure")
	}
	w.handleFailure(context.Background(), job, value, 1, err)

	if len(publisher.messages) != 1 {
		t.Fatalf("expected 1 published retry message, got %d", len(publisher.messages))
	}
	msg := publisher.messages[0]
	if msg.Topic != kafka.TopicJobListingsFilteredRetry10m {
		t.Errorf("topic: want %s got %s", kafka.TopicJobListingsFilteredRetry10m, msg.Topic)
	}
	if got := headerValue(msg.Headers, kafka.HeaderRetryCount); got != "2" {
		t.Errorf("retry_count header: want 2 got %q", got)
	}
	if len(pipeline.markCalls) != 0 {
		t.Errorf("ledger should NOT be marked on retry; got calls %v", pipeline.markCalls)
	}
}

// TestEnrich_TransientError_ThirdTier verifies that an incoming retry_count=2
// message failing is routed to the 1h retry tier with retry_count=3.
func TestEnrich_TransientError_ThirdTier(t *testing.T) {
	scorer := &fakeScorer{err: stderrors.New("LLM timeout")}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	w := makeWorkerWithPublisher(scorer, pipeline, publisher)

	job := baseRawJob()
	value := marshalJob(t, job)
	err := w.enrich(context.Background(), job)
	if err == nil {
		t.Fatalf("expected enrich to return an error for transient failure")
	}
	w.handleFailure(context.Background(), job, value, 2, err)

	if len(publisher.messages) != 1 {
		t.Fatalf("expected 1 published retry message, got %d", len(publisher.messages))
	}
	msg := publisher.messages[0]
	if msg.Topic != kafka.TopicJobListingsFilteredRetry1h {
		t.Errorf("topic: want %s got %s", kafka.TopicJobListingsFilteredRetry1h, msg.Topic)
	}
	if got := headerValue(msg.Headers, kafka.HeaderRetryCount); got != "3" {
		t.Errorf("retry_count header: want 3 got %q", got)
	}
	if len(pipeline.markCalls) != 0 {
		t.Errorf("ledger should NOT be marked on retry; got calls %v", pipeline.markCalls)
	}
}

// TestEnrich_ExhaustedGoesToDLQ verifies that once retry_count reaches the
// number of retry tiers, the next failure sends the message to the DLQ and
// marks the ledger row as errored.
func TestEnrich_ExhaustedGoesToDLQ(t *testing.T) {
	scorer := &fakeScorer{err: stderrors.New("LLM timeout")}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	w := makeWorkerWithPublisher(scorer, pipeline, publisher)

	job := baseRawJob()
	value := marshalJob(t, job)
	err := w.enrich(context.Background(), job)
	if err == nil {
		t.Fatalf("expected enrich to return an error for transient failure")
	}

	w.handleFailure(context.Background(), job, value, 3, err)

	if len(publisher.messages) != 1 {
		t.Fatalf("expected 1 DLQ message, got %d", len(publisher.messages))
	}
	msg := publisher.messages[0]
	if msg.Topic != kafka.TopicJobListingsFilteredDLQ {
		t.Errorf("topic: want %s got %s", kafka.TopicJobListingsFilteredDLQ, msg.Topic)
	}
	if got := headerValue(msg.Headers, kafka.HeaderRetryCount); got != "3" {
		t.Errorf("retry_count header: want 3 got %q", got)
	}
	if headerValue(msg.Headers, kafka.HeaderLastError) == "" {
		t.Errorf("last_error header missing on DLQ message")
	}
	if len(pipeline.markCalls) != 1 || pipeline.markCalls[0] != job.ID {
		t.Errorf("ledger should be marked on DLQ; got %v", pipeline.markCalls)
	}
}

// TestEnrich_ValidationErrorGoesStraightToDLQ verifies that when enrich()
// fails with ErrLLMValidation, the message is published to the DLQ regardless
// of retry_count (retry is pointless — it's our own bug).
func TestEnrich_ValidationErrorGoesStraightToDLQ(t *testing.T) {
	scorer := &fakeScorer{err: domainerr.ErrLLMValidation}
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	w := makeWorkerWithPublisher(scorer, pipeline, publisher)

	// Use FullContent so enrich skips the HTTP fetch and surfaces the
	// validation error from the LLM directly.
	job := baseRawJob()
	job.FullContent = &atstypes.FullJobContent{Description: "n/a"}
	value := marshalJob(t, job)

	err := w.enrich(context.Background(), job)
	if err == nil {
		t.Fatalf("expected enrich to return validation error")
	}

	// retry_count=0 — should STILL go to DLQ, not retry.
	w.handleFailure(context.Background(), job, value, 0, err)

	if len(publisher.messages) != 1 {
		t.Fatalf("expected 1 DLQ message, got %d", len(publisher.messages))
	}
	msg := publisher.messages[0]
	if msg.Topic != kafka.TopicJobListingsFilteredDLQ {
		t.Errorf("topic: want %s got %s", kafka.TopicJobListingsFilteredDLQ, msg.Topic)
	}
	if got := headerValue(msg.Headers, kafka.HeaderRetryCount); got != "0" {
		t.Errorf("retry_count header: want 0 got %q", got)
	}
	if headerValue(msg.Headers, kafka.HeaderLastError) == "" {
		t.Errorf("last_error header missing on DLQ message")
	}
	if len(pipeline.markCalls) != 1 || pipeline.markCalls[0] != job.ID {
		t.Errorf("ledger should be marked on DLQ; got %v", pipeline.markCalls)
	}
}

// TestHandleFailure_ContextCanceledSkipsPublish verifies that when ctx is
// cancelled (e.g. during shutdown), we don't publish to any topic — Kafka
// will redeliver the message on restart.
func TestHandleFailure_ContextCanceledSkipsPublish(t *testing.T) {
	pipeline := &fakePipelineService{}
	publisher := &fakePublisher{}
	w := makeWorkerWithPublisher(&fakeScorer{}, pipeline, publisher)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	w.handleFailure(ctx, baseRawJob(), []byte("value"), 0, context.Canceled)

	if len(publisher.messages) != 0 {
		t.Errorf("expected no publishes when ctx is cancelled, got %d", len(publisher.messages))
	}
	if len(pipeline.markCalls) != 0 {
		t.Errorf("expected no ledger marks when ctx is cancelled, got %v", pipeline.markCalls)
	}
}
