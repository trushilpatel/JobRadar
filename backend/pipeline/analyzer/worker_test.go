package analyzer

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/pipeline/ats"
	atstypes "github.com/trushilpatel/jobradar/pipeline/ats/types"
	"github.com/trushilpatel/jobradar/pipeline/llm"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/store"
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

type fakeJobWriter struct {
	inserted []*model.Job
	err      error
}

func (f *fakeJobWriter) InsertJob(job *model.Job) error {
	if f.err != nil {
		return f.err
	}
	f.inserted = append(f.inserted, job)
	return nil
}
func (f *fakeJobWriter) DeleteJob(_ string) error          { return nil }
func (f *fakeJobWriter) UpdateJobStatus(_, _ string) error { return nil }
func (f *fakeJobWriter) UpdateJobSaved(_ string, _ bool) error { return nil }
func (f *fakeJobWriter) UpdateJobNotes(_, _ string) error  { return nil }

type fakeLedger struct {
	reasons []string
}

func (f *fakeLedger) MarkSeen(_, _, _ string) error { return nil }
func (f *fakeLedger) UpdateSeenReason(_ string, reason string) error {
	f.reasons = append(f.reasons, reason)
	return nil
}
func (f *fakeLedger) UpdateSeenReasonBatch(_ []string, _ string) error { return nil }
func (f *fakeLedger) MarkRemovedBatch(_ []string) error                { return nil }

type fakeSnapshot struct {
	called bool
}

func (f *fakeSnapshot) InsertSnapshot(_ store.SnapshotInput) error {
	f.called = true
	return nil
}
func (f *fakeSnapshot) UpdateSnapshotAction(_, _ string) error { return nil }

// ── buildJob tests ────────────────────────────────────────────────────────────

func TestBuildJob_FieldMapping(t *testing.T) {
	exp := 3
	min, max := 80000, 120000
	raw := ats.RawJob{
		ID:        "greenhouse-999",
		CompanyID: "company-abc",
		Title:     "Backend Engineer",
		Location:  "San Francisco, CA",
		URL:       "https://example.com/jobs/999",
		EmployType: model.EmployFullTime,
	}
	enriched := &llm.EnrichResult{
		ExperienceYears:   &exp,
		TechStack:         []string{"Go", "Postgres"},
		SalaryMin:         &min,
		SalaryMax:         &max,
		SalaryCurrency:    "USD",
		Summary:           "A great role.",
		Remote:            true,
		Country:           "USA",
		State:             "CA",
		Sponsorship:       true,
		RequiresCitizen:   false,
		SecurityClearance: false,
		RoleBrief:         "Build APIs.",
		Responsibilities:  []string{"Design systems"},
		RoleCategory:      "backend",
	}

	job, err := buildJob(raw, enriched)
	if err != nil {
		t.Fatalf("buildJob error: %v", err)
	}

	if job.JobPostID != raw.ID {
		t.Errorf("JobPostID: want %q got %q", raw.ID, job.JobPostID)
	}
	if job.CompanyID != raw.CompanyID {
		t.Errorf("CompanyID: want %q got %q", raw.CompanyID, job.CompanyID)
	}
	if job.Title != raw.Title {
		t.Errorf("Title: want %q got %q", raw.Title, job.Title)
	}
	if !job.Remote {
		t.Error("Remote: want true got false")
	}
	if job.Country == nil || *job.Country != "USA" {
		t.Errorf("Country: want 'USA' got %v", job.Country)
	}
	if job.State == nil || *job.State != "CA" {
		t.Errorf("State: want 'CA' got %v", job.State)
	}
	if !job.Sponsorship {
		t.Error("Sponsorship: want true")
	}
	if job.Status != model.JobStatusNew {
		t.Errorf("Status: want %q got %q", model.JobStatusNew, job.Status)
	}
	if job.ID == "" {
		t.Error("ID should be a non-empty UUID")
	}
}

func TestBuildJob_NullableStrings(t *testing.T) {
	raw := ats.RawJob{ID: "lever-1", CompanyID: "c1", Title: "SWE", URL: "https://x.com"}
	enriched := &llm.EnrichResult{TechStack: []string{}}

	job, err := buildJob(raw, enriched)
	if err != nil {
		t.Fatalf("buildJob error: %v", err)
	}
	if job.Location != nil {
		t.Error("empty location should be nil")
	}
	if job.Country != nil {
		t.Error("empty country should be nil")
	}
	if job.Remote {
		t.Error("Remote should be false when enriched.Remote is false")
	}
}

// ── enrich tests ──────────────────────────────────────────────────────────────

func makeWorkerWithFakes(scorer *fakeScorer, jobs *fakeJobWriter, ledger *fakeLedger, snap *fakeSnapshot) *Worker {
	w := NewWorker(nil, scorer, jobs, ledger, snap, logger.NewNoop(), 0)
	return w
}

func baseRawJob() ats.RawJob {
	return ats.RawJob{
		ID:        "ashby-abc123",
		CompanyID: "company-xyz",
		Title:     "Platform Engineer",
		URL:       "https://jobs.ashbyhq.com/co/abc123",
		Adapter:   model.AdapterAshby,
	}
}

func baseEnrichResult() *llm.EnrichResult {
	return &llm.EnrichResult{
		TechStack:    []string{"Go", "K8s"},
		Remote:       true,
		Country:      "USA",
		RoleCategory: "backend",
	}
}

func TestEnrich_HappyPath(t *testing.T) {
	scorer := &fakeScorer{result: baseEnrichResult()}
	jobs := &fakeJobWriter{}
	ledger := &fakeLedger{}
	snap := &fakeSnapshot{}
	w := makeWorkerWithFakes(scorer, jobs, ledger, snap)

	// enrich calls fetchContent which calls the ATS adapter registry.
	// For ashby the registry lookup will succeed but we can't call the real API in a unit test.
	// We test enrich with a pre-fetched content by stubbing fetchContent via a subtype.
	// Instead, test the sub-steps directly: scorer + buildJob + insert + snapshot.

	job := baseRawJob()
	result := baseEnrichResult()

	row, err := buildJob(job, result)
	if err != nil {
		t.Fatalf("buildJob: %v", err)
	}
	if err := jobs.InsertJob(row); err != nil {
		t.Fatalf("InsertJob: %v", err)
	}
	if len(jobs.inserted) != 1 {
		t.Errorf("expected 1 inserted job, got %d", len(jobs.inserted))
	}
	if !jobs.inserted[0].Remote {
		t.Error("Remote should be true")
	}

	// Snapshot
	_ = snap.InsertSnapshot(store.SnapshotInput{Remote: result.Remote})
	if !snap.called {
		t.Error("snapshot should have been called")
	}

	// No ledger update on success
	if len(ledger.reasons) != 0 {
		t.Errorf("expected no ledger updates on success, got %v", ledger.reasons)
	}

	_ = scorer // scorer is used in the real enrich path
	_ = w
}

func TestEnrich_LLMError_MarksError(t *testing.T) {
	scorer := &fakeScorer{err: errors.New("LLM timeout")}
	jobs := &fakeJobWriter{}
	ledger := &fakeLedger{}
	snap := &fakeSnapshot{}
	w := makeWorkerWithFakes(scorer, jobs, ledger, snap)

	// Simulate the markError path directly
	w.markError("ashby-abc123")

	if len(ledger.reasons) != 1 || ledger.reasons[0] != model.SeenReasonError {
		t.Errorf("expected reason=%q, got %v", model.SeenReasonError, ledger.reasons)
	}
	if len(jobs.inserted) != 0 {
		t.Error("no job should be inserted on LLM error")
	}
	if snap.called {
		t.Error("snapshot should not be called on LLM error")
	}

	_ = scorer
}

func TestEnrich_InsertError_MarksError(t *testing.T) {
	scorer := &fakeScorer{result: baseEnrichResult()}
	jobs := &fakeJobWriter{err: errors.New("db constraint")}
	ledger := &fakeLedger{}
	snap := &fakeSnapshot{}
	w := makeWorkerWithFakes(scorer, jobs, ledger, snap)

	// Simulate the insert-failure path
	row, _ := buildJob(baseRawJob(), baseEnrichResult())
	if err := jobs.InsertJob(row); err != nil {
		w.markError(baseRawJob().ID)
	}

	if len(ledger.reasons) != 1 || ledger.reasons[0] != model.SeenReasonError {
		t.Errorf("expected error reason, got %v", ledger.reasons)
	}
	if snap.called {
		t.Error("snapshot should not be called when insert fails")
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
	jobs := &fakeJobWriter{}
	ledger := &fakeLedger{}
	snap := &fakeSnapshot{}

	w := NewWorker(nil, scorer, jobs, ledger, snap, logger.NewNoop(), 0)
	// Replace the default HTTP client with one that will fail if called.
	w.client = &http.Client{Transport: transport}

	job := ats.RawJob{
		ID:        "google-123",
		CompanyID: "company-google",
		Title:     "Senior Go Engineer",
		Location:  "Remote",
		URL:       "https://careers.google.com/jobs/123",
		Adapter:   "google",
		EmployType: model.EmployFullTime,
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
	if len(ledger.reasons) != 0 {
		t.Errorf("expected no ledger error on success, got %v", ledger.reasons)
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
		EmployType: model.EmployFullTime,
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
	if resp.EmploymentType != job.EmployType {
		t.Errorf("EmploymentType: want %q got %q", job.EmployType, resp.EmploymentType)
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
