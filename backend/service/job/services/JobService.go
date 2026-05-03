package services

import (
	"context"

	"github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
)

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
	BulkDismiss(ctx context.Context, input dto.BulkDismissInput) (int, error)
	GetJobPostIDs(ctx context.Context, ids []string) ([]string, error)
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

// BaseService auto-delegates all Service methods to the next layer in the chain.
// Embed this in ImplService and ValidationService; override only what you own.
type BaseService struct {
	next Service
}

// NewBaseService constructs a BaseService wrapping next.
// Panics if next is nil — a nil next always indicates a wiring error.
func NewBaseService(next Service) BaseService {
	if next == nil {
		panic("BaseService: next must not be nil — check wire setup")
	}
	return BaseService{next: next}
}

func (b *BaseService) List(ctx context.Context, filter dto.JobFilter, page, limit int) ([]jobmodel.Job, int, error) {
	return b.next.List(ctx, filter, page, limit)
}

func (b *BaseService) Get(ctx context.Context, id string) (*jobmodel.Job, error) {
	return b.next.Get(ctx, id)
}

func (b *BaseService) GetCounts(ctx context.Context) (*dto.JobCounts, error) {
	return b.next.GetCounts(ctx)
}

func (b *BaseService) Dismiss(ctx context.Context, id string, input dto.DismissInput) error {
	return b.next.Dismiss(ctx, id, input)
}

func (b *BaseService) SetSaved(ctx context.Context, id string, saved bool) error {
	return b.next.SetSaved(ctx, id, saved)
}

func (b *BaseService) SetStatus(ctx context.Context, id, status string) error {
	return b.next.SetStatus(ctx, id, status)
}

func (b *BaseService) SetNotes(ctx context.Context, id, notes string) error {
	return b.next.SetNotes(ctx, id, notes)
}

func (b *BaseService) MarkApplied(ctx context.Context, id string) error {
	return b.next.MarkApplied(ctx, id)
}

func (b *BaseService) BulkDismiss(ctx context.Context, input dto.BulkDismissInput) (int, error) {
	return b.next.BulkDismiss(ctx, input)
}

func (b *BaseService) GetJobPostIDs(ctx context.Context, ids []string) ([]string, error) {
	return b.next.GetJobPostIDs(ctx, ids)
}
