package services

import (
	"context"

	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	"github.com/trushilpatel/jobradar/service/job/dto"
)

// Validator validates user-submitted input for the job domain.
type Validator interface {
	ValidateList(filter dto.JobFilter, page, limit int) error
	ValidateDismiss(input dto.DismissInput) error
	ValidateSetStatus(status string) error
	ValidateSetNotes(input dto.SetNotesInput) error
}

// Service defines all business operations for the job domain.
type Service interface {
	List(ctx context.Context, filter dto.JobFilter, page, limit int) ([]jobmodel.Job, int, error)
	Get(ctx context.Context, id string) (*jobmodel.Job, error)
	GetCounts(ctx context.Context) (*dto.JobCounts, error)
	Dismiss(ctx context.Context, id string, input dto.DismissInput) error
	SetSaved(ctx context.Context, id string, saved bool) error
	SetStatus(ctx context.Context, id, status string) error
	SetNotes(ctx context.Context, id, notes string) error
	// MarkApplied advances the job status to "applied" without requiring callers to
	// import job domain constants.
	MarkApplied(ctx context.Context, id string) error
}

// LedgerService is the interface pipeline workers use for job dedup and removal tracking.
type LedgerService interface {
	HasSeenBatch(ctx context.Context, jobIDs []string) (map[string]bool, error)
	MarkSeenBatch(ctx context.Context, jobIDs []string, companyID, reason string) error
	GetActiveJobIDsByCompany(ctx context.Context, companyID string) ([]string, error)
	MarkRemovedBatch(ctx context.Context, jobIDs []string) error
}

// ApplicationWriter is a narrow write interface used by the job service to
// auto-insert application tracker rows when a job is advanced to applied/interviewing/offer.
// Each method encapsulates the stage constant so callers don't need to import
// application domain constants.
type ApplicationWriter interface {
	InsertAppliedApplication(jobResultID string) error
	InsertInterviewingApplication(jobResultID string) error
	InsertOfferApplication(jobResultID string) error
}

// PipelineService is the interface the analyzer worker depends on.
type PipelineService interface {
	// SaveEnrichedJob inserts the job into job_feed, writes to job_analytics snapshot,
	// and marks the pipeline ledger entry as done.
	// The caller is responsible for eligibility filtering before calling this method.
	SaveEnrichedJob(ctx context.Context, input dto.EnrichedJobInput) error

	// MarkLedgerError marks a pipeline ledger entry with reason='error'.
	MarkLedgerError(ctx context.Context, jobID string) error

	// MarkLedgerIrrelevant marks a pipeline ledger entry with reason='llm_irrelevant'.
	MarkLedgerIrrelevant(ctx context.Context, jobID string) error

	// HasSeenJob returns true if the job has already been processed (dedup check).
	HasSeenJob(ctx context.Context, jobID string) (bool, error)
}
