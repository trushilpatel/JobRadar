package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// --- Test doubles (local to this file) ---

type pipelineJobWriter struct {
	inserted  *jobmodel.Job
	insertErr error
	deleteErr error
	statusErr error
	savedErr  error
	notesErr  error
}

func (f *pipelineJobWriter) InsertJob(_ context.Context, job *jobmodel.Job) error {
	f.inserted = job
	return f.insertErr
}
func (f *pipelineJobWriter) DeleteJob(_ context.Context, _ string) error          { return f.deleteErr }
func (f *pipelineJobWriter) UpdateJobStatus(_ context.Context, _, _ string) error { return f.statusErr }
func (f *pipelineJobWriter) UpdateJobSaved(_ context.Context, _ string, _ bool) error {
	return f.savedErr
}
func (f *pipelineJobWriter) UpdateJobNotes(_ context.Context, _, _ string) error { return f.notesErr }

type pipelineLedgerStore struct {
	hasSeen           bool
	hasSeenErr        error
	updateReasonJobID string
	updateReasonVal   string
	updateReasonErr   error
}

func (f *pipelineLedgerStore) HasSeen(_ context.Context, _ string) (bool, error) {
	return f.hasSeen, f.hasSeenErr
}
func (f *pipelineLedgerStore) UpdateSeenReason(_ context.Context, jobID, reason string) error {
	f.updateReasonJobID = jobID
	f.updateReasonVal = reason
	return f.updateReasonErr
}

type pipelineSnapshotWriter struct {
	insertCalled bool
	insertErr    error
	updateCalled bool
	updateErr    error
}

func (f *pipelineSnapshotWriter) InsertSnapshot(_ context.Context, _ jobdto.SnapshotInput) error {
	f.insertCalled = true
	return f.insertErr
}
func (f *pipelineSnapshotWriter) UpdateSnapshotAction(_ context.Context, _, _ string) error {
	f.updateCalled = true
	return f.updateErr
}

type pipelineTechTagWriter struct {
	insertErr error
}

func (f *pipelineTechTagWriter) InsertTechTags(_ context.Context, _, _ string, _ []string, _ time.Time) error {
	return f.insertErr
}

// Compile-time interface checks.
var _ jobsvc.JobWriter = (*pipelineJobWriter)(nil)
var _ jobsvc.SnapshotWriter = (*pipelineSnapshotWriter)(nil)
var _ jobsvc.TechTagWriter = (*pipelineTechTagWriter)(nil)

// --- Helpers ---

func makeInput() jobdto.EnrichedJobInput {
	expYears := 3
	salMin := 100000
	salMax := 150000
	return jobdto.EnrichedJobInput{
		Raw: ats.RawJob{
			ID:         "greenhouse-123",
			CompanyID:  "company-abc",
			Title:      "Software Engineer",
			Location:   "San Francisco, CA",
			URL:        "https://example.com/jobs/123",
			EmployType: jobcommon.EmployFullTime,
		},
		Enriched: &llmdto.JobPostEnrichResult{
			ExperienceYears:   &expYears,
			TechStack:         []string{"Go", "PostgreSQL"},
			SalaryMin:         &salMin,
			SalaryMax:         &salMax,
			SalaryCurrency:    "USD",
			Summary:           "Great role",
			Remote:            true,
			Country:           "USA",
			State:             "CA",
			Sponsorship:       true,
			RequiresCitizen:   false,
			SecurityClearance: false,
			RoleBrief:         "Backend engineer on platform team",
			Responsibilities:  []string{"Design APIs", "Code reviews"},
			RoleCategory:      "backend",
		},
	}
}

// --- Tests ---

func TestSaveEnrichedJob_HappyPath(t *testing.T) {
	jobs := &pipelineJobWriter{}
	ledger := &pipelineLedgerStore{}
	snapshot := &pipelineSnapshotWriter{}

	svc := jobsvc.NewPipelineService(jobs, ledger, snapshot, &pipelineTechTagWriter{})
	input := makeInput()

	if err := svc.SaveEnrichedJob(context.Background(), input); err != nil {
		t.Fatalf("SaveEnrichedJob() error = %v", err)
	}

	// Job was inserted.
	if jobs.inserted == nil {
		t.Fatal("expected InsertJob to be called, got nil")
	}
	if jobs.inserted.JobPostID != input.Raw.ID {
		t.Errorf("JobPostID = %q, want %q", jobs.inserted.JobPostID, input.Raw.ID)
	}
	if jobs.inserted.Status != jobcommon.JobStatusNew {
		t.Errorf("Status = %q, want %q", jobs.inserted.Status, jobcommon.JobStatusNew)
	}
	if jobs.inserted.ID == "" {
		t.Error("expected a non-empty job ID (uuid)")
	}

	// Snapshot was written.
	if !snapshot.insertCalled {
		t.Error("expected InsertSnapshot to be called")
	}

	// Ledger was marked.
	if ledger.updateReasonJobID != input.Raw.ID {
		t.Errorf("ledger marked job ID = %q, want %q", ledger.updateReasonJobID, input.Raw.ID)
	}
	if ledger.updateReasonVal != jobcommon.SeenReasonFetch {
		t.Errorf("ledger marked reason = %q, want %q", ledger.updateReasonVal, jobcommon.SeenReasonFetch)
	}
}

func TestSaveEnrichedJob_InsertJobFails_ReturnsError(t *testing.T) {
	jobs := &pipelineJobWriter{insertErr: errors.New("db error")}
	ledger := &pipelineLedgerStore{}
	snapshot := &pipelineSnapshotWriter{}

	svc := jobsvc.NewPipelineService(jobs, ledger, snapshot, &pipelineTechTagWriter{})

	if err := svc.SaveEnrichedJob(context.Background(), makeInput()); err == nil {
		t.Fatal("expected error when InsertJob fails")
	}

	// Snapshot and ledger should NOT be touched if insert failed.
	if snapshot.insertCalled {
		t.Error("expected InsertSnapshot NOT to be called after insert failure")
	}
	if ledger.updateReasonJobID != "" {
		t.Error("expected ledger NOT to be updated after insert failure")
	}
}

func TestSaveEnrichedJob_SnapshotFailIsNonFatal(t *testing.T) {
	jobs := &pipelineJobWriter{}
	ledger := &pipelineLedgerStore{}
	snapshot := &pipelineSnapshotWriter{insertErr: errors.New("snapshot error")}

	svc := jobsvc.NewPipelineService(jobs, ledger, snapshot, &pipelineTechTagWriter{})

	// Snapshot failure must not bubble up — job is already saved.
	if err := svc.SaveEnrichedJob(context.Background(), makeInput()); err != nil {
		t.Fatalf("SaveEnrichedJob() should succeed even if snapshot fails, got: %v", err)
	}

	if jobs.inserted == nil {
		t.Error("expected job to be inserted despite snapshot failure")
	}
	// Ledger should still be marked.
	if ledger.updateReasonVal != jobcommon.SeenReasonFetch {
		t.Errorf("ledger reason = %q, want %q", ledger.updateReasonVal, jobcommon.SeenReasonFetch)
	}
}

func TestSaveEnrichedJob_NullableFields_EmptyStrings(t *testing.T) {
	jobs := &pipelineJobWriter{}
	ledger := &pipelineLedgerStore{}
	snapshot := &pipelineSnapshotWriter{}

	svc := jobsvc.NewPipelineService(jobs, ledger, snapshot, &pipelineTechTagWriter{})

	input := makeInput()
	input.Raw.Location = ""
	input.Raw.EmployType = ""
	input.Enriched.Country = ""
	input.Enriched.State = ""
	input.Enriched.Summary = ""
	input.Enriched.RoleBrief = ""

	if err := svc.SaveEnrichedJob(context.Background(), input); err != nil {
		t.Fatalf("SaveEnrichedJob() error = %v", err)
	}

	j := jobs.inserted
	if j.Location != nil {
		t.Errorf("Location should be nil for empty string, got %v", j.Location)
	}
	if j.EmployType != nil {
		t.Errorf("EmployType should be nil for empty string, got %v", j.EmployType)
	}
	if j.Country != nil {
		t.Errorf("Country should be nil for empty string, got %v", j.Country)
	}
	if j.State != nil {
		t.Errorf("State should be nil for empty string, got %v", j.State)
	}
	if j.LLMSummary != nil {
		t.Errorf("LLMSummary should be nil for empty string, got %v", j.LLMSummary)
	}
	if j.RoleBrief != nil {
		t.Errorf("RoleBrief should be nil for empty string, got %v", j.RoleBrief)
	}
}

func TestMarkLedgerError_CallsLedgerWithErrorReason(t *testing.T) {
	jobs := &pipelineJobWriter{}
	ledger := &pipelineLedgerStore{}
	snapshot := &pipelineSnapshotWriter{}

	svc := jobsvc.NewPipelineService(jobs, ledger, snapshot, &pipelineTechTagWriter{})

	if err := svc.MarkLedgerError(context.Background(), "job-xyz"); err != nil {
		t.Fatalf("MarkLedgerError() error = %v", err)
	}

	if ledger.updateReasonJobID != "job-xyz" {
		t.Errorf("ledger job ID = %q, want %q", ledger.updateReasonJobID, "job-xyz")
	}
	if ledger.updateReasonVal != jobcommon.SeenReasonError {
		t.Errorf("ledger reason = %q, want %q", ledger.updateReasonVal, jobcommon.SeenReasonError)
	}
}

func TestMarkLedgerError_PropagatesLedgerError(t *testing.T) {
	ledger := &pipelineLedgerStore{updateReasonErr: errors.New("ledger failure")}
	svc := jobsvc.NewPipelineService(&pipelineJobWriter{}, ledger, &pipelineSnapshotWriter{}, &pipelineTechTagWriter{})

	if err := svc.MarkLedgerError(context.Background(), "job-xyz"); err == nil {
		t.Fatal("expected error when ledger fails")
	}
}

func TestHasSeenJob_DelegatesToLedger(t *testing.T) {
	ledger := &pipelineLedgerStore{hasSeen: true}
	svc := jobsvc.NewPipelineService(&pipelineJobWriter{}, ledger, &pipelineSnapshotWriter{}, &pipelineTechTagWriter{})

	seen, err := svc.HasSeenJob(context.Background(), "job-abc")
	if err != nil {
		t.Fatalf("HasSeenJob() error = %v", err)
	}
	if !seen {
		t.Error("expected HasSeenJob() to return true")
	}
}

func TestHasSeenJob_NotSeen(t *testing.T) {
	ledger := &pipelineLedgerStore{hasSeen: false}
	svc := jobsvc.NewPipelineService(&pipelineJobWriter{}, ledger, &pipelineSnapshotWriter{}, &pipelineTechTagWriter{})

	seen, err := svc.HasSeenJob(context.Background(), "job-new")
	if err != nil {
		t.Fatalf("HasSeenJob() error = %v", err)
	}
	if seen {
		t.Error("expected HasSeenJob() to return false")
	}
}

func TestHasSeenJob_PropagatesError(t *testing.T) {
	ledger := &pipelineLedgerStore{hasSeenErr: errors.New("db error")}
	svc := jobsvc.NewPipelineService(&pipelineJobWriter{}, ledger, &pipelineSnapshotWriter{}, &pipelineTechTagWriter{})

	_, err := svc.HasSeenJob(context.Background(), "job-abc")
	if err == nil {
		t.Fatal("expected error from HasSeenJob when ledger fails")
	}
}
