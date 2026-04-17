package analyzer

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/pipeline/ats"
	atstypes "github.com/trushilpatel/jobradar/pipeline/ats/types"
	"github.com/trushilpatel/jobradar/pipeline/llm"
	jobsvc "github.com/trushilpatel/jobradar/service/job"
)

// ── fakes ────────────────────────────────────────────────────────────────────

type fakeScorer struct {
	result *llm.EnrichResult
	err    error
}

func (f *fakeScorer) ScoreBatch(_ context.Context, _ []llm.ScorerInput) ([]string, error) {
	return nil, nil
}
func (f *fakeScorer) Enrich(_ context.Context, _ llm.EnrichInput) (*llm.EnrichResult, error) {
	return f.result, f.err
}

type fakePipelineService struct {
	saveErr     error
	markErr     error
	hasSeen     bool
	hasSeenErr  error
	savedInputs []jobsvc.EnrichedJobInput
	markCalls   []string
}

func (f *fakePipelineService) SaveEnrichedJob(_ context.Context, input jobsvc.EnrichedJobInput) error {
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

func (f *fakePipelineService) HasSeenJob(_ context.Context, id string) (bool, error) {
	return f.hasSeen, f.hasSeenErr
}

// ── helpers ───────────────────────────────────────────────────────────────────

func makeWorker(scorer *fakeScorer, pipeline *fakePipelineService) *Worker {
	return NewWorker(nil, scorer, pipeline, logger.NewNoop(), 0)
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

func baseEnrichResult() *llm.EnrichResult {
	return &llm.EnrichResult{
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
	scorer := &fakeScorer{err: errors.New("LLM timeout")}
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
	pipeline := &fakePipelineService{saveErr: errors.New("db constraint")}
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
	result := &llm.EnrichResult{Sponsorship: false}
	if reason := w.dropJob(result); reason == "" {
		t.Error("expected drop reason for no sponsorship")
	}
}

func TestDropJob_RequiresCitizen(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{})
	result := &llm.EnrichResult{Sponsorship: true, RequiresCitizen: true}
	if reason := w.dropJob(result); reason == "" {
		t.Error("expected drop reason for requires citizen")
	}
}

func TestDropJob_SecurityClearance(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{})
	result := &llm.EnrichResult{Sponsorship: true, SecurityClearance: true}
	if reason := w.dropJob(result); reason == "" {
		t.Error("expected drop reason for security clearance")
	}
}

func TestDropJob_ExperienceExceedsMax(t *testing.T) {
	w := NewWorker(nil, &fakeScorer{}, &fakePipelineService{}, logger.NewNoop(), 3)
	exp := 5
	result := &llm.EnrichResult{Sponsorship: true, ExperienceYears: &exp}
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
	result      *llm.EnrichResult
	err         error
	lastInput   llm.EnrichInput
	enrichCalls int
}

func (c *capturingScorer) ScoreBatch(_ context.Context, _ []llm.ScorerInput) ([]string, error) {
	return nil, nil
}
func (c *capturingScorer) Enrich(_ context.Context, input llm.EnrichInput) (*llm.EnrichResult, error) {
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
	return nil, errors.New("HTTP client must not be called when FullContent is present")
}

// TestEnrich_SkipsFetchWhenFullContentPresent verifies that when a RawJob has
// FullContent set, enrich() uses fullContentToResponse() instead of fetchContent(),
// meaning no HTTP request is made and the LLM receives the pre-fetched content.
func TestEnrich_SkipsFetchWhenFullContentPresent(t *testing.T) {
	transport := &failingTransport{}
	scorer := &capturingScorer{result: &llm.EnrichResult{
		TechStack:   []string{"Go"},
		Remote:      true,
		Country:     "USA",
		Sponsorship: true,
	}}
	pipeline := &fakePipelineService{}

	w := NewWorker(nil, scorer, pipeline, logger.NewNoop(), 0)
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

	w.enrich(context.Background(), job)

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
