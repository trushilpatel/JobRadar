package job

import (
	"context"

	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/store"
)

// Service defines all business operations for the job domain.
type Service interface {
	List(ctx context.Context, filter JobFilter, page, limit int) ([]model.Job, int, error)
	Get(ctx context.Context, id string) (*model.Job, error)
	GetCounts(ctx context.Context) (*JobCounts, error)
	Dismiss(ctx context.Context, id string, input DismissInput) error
	SetSaved(ctx context.Context, id string, saved bool) error
	SetStatus(ctx context.Context, id, status string) error
	SetNotes(ctx context.Context, id, notes string) error
}

type service struct {
	jobs     JobStore
	ledger   SeenLedgerWriter
	discard  DiscardStore
	snapshot SnapshotWriter
}

// NewService constructs a Service implementation.
func NewService(jobs JobStore, ledger SeenLedgerWriter, discard DiscardStore, snapshot SnapshotWriter) Service {
	return &service{
		jobs:     jobs,
		ledger:   ledger,
		discard:  discard,
		snapshot: snapshot,
	}
}

// List returns a paginated, filtered list of jobs — all filtering happens at the DB level.
func (s *service) List(ctx context.Context, filter JobFilter, page, limit int) ([]model.Job, int, error) {
	jobs, total, err := s.jobs.GetJobs(ctx, filter, page, limit)
	if err != nil {
		return nil, 0, domain.ErrInternal.WithMessage("failed to fetch jobs")
	}
	return jobs, int(total), nil
}

// GetCounts returns a count of jobs per active status.
func (s *service) GetCounts(ctx context.Context) (*JobCounts, error) {
	counts, err := s.jobs.GetJobCounts(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch job counts")
	}
	return counts, nil
}

// Get fetches a single job by ID.
func (s *service) Get(ctx context.Context, id string) (*model.Job, error) {
	job, err := s.jobs.GetJobByID(ctx, id)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, domain.ErrNotFound.WithMessage("job not found")
		}
		return nil, domain.ErrInternal.WithMessage("failed to fetch job")
	}
	return job, nil
}

// Dismiss dismisses a job: records discard, blocks re-entry, hides from feed, and backfills snapshot.
func (s *service) Dismiss(ctx context.Context, id string, input DismissInput) error {
	job, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	if err := s.discard.InsertDiscard(ctx, id, input.Reason, input.Note); err != nil {
		return domain.ErrInternal.WithMessage("failed to discard job")
	}

	if err := s.ledger.UpdateSeenReason(ctx, job.JobPostID, model.SeenReasonUserDismissed); err != nil {
		return domain.ErrInternal.WithMessage("failed to update seen ledger")
	}

	if err := s.jobs.UpdateJobStatus(ctx, id, model.JobStatusDismissed); err != nil {
		return domain.ErrInternal.WithMessage("failed to update job status")
	}

	// Non-fatal: snapshot may not exist for older jobs
	_ = s.snapshot.UpdateSnapshotAction(ctx, id, model.JobStatusDismissed)

	return nil
}

// SetSaved updates the saved flag for a job and non-fatally backfills the snapshot.
func (s *service) SetSaved(ctx context.Context, id string, saved bool) error {
	if err := s.jobs.UpdateJobSaved(ctx, id, saved); err != nil {
		return domain.ErrInternal.WithMessage("failed to update job")
	}
	if saved {
		_ = s.snapshot.UpdateSnapshotAction(ctx, id, model.JobStatusSaved)
	}
	return nil
}

// SetStatus updates the status of a job and non-fatally backfills the snapshot if applied.
func (s *service) SetStatus(ctx context.Context, id, status string) error {
	if err := s.jobs.UpdateJobStatus(ctx, id, status); err != nil {
		return domain.ErrInternal.WithMessage("failed to update status")
	}
	if status == model.JobStatusApplied {
		_ = s.snapshot.UpdateSnapshotAction(ctx, id, model.JobStatusApplied)
	}
	return nil
}

// SetNotes updates the notes for a job.
func (s *service) SetNotes(ctx context.Context, id, notes string) error {
	if err := s.jobs.UpdateJobNotes(ctx, id, notes); err != nil {
		return domain.ErrInternal.WithMessage("failed to update notes")
	}
	return nil
}
