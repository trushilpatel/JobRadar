package job

import (
	"github.com/trushilpatel/jobradar/api/request"
	"github.com/trushilpatel/jobradar/api/response"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/store"
)

// Service defines all business operations for the job domain.
type Service interface {
	List(filter JobFilter, p request.Pagination) ([]model.Job, int, error)
	Get(id string) (*model.Job, error)
	Dismiss(id string, input DismissInput) error
	SetSaved(id string, saved bool) error
	SetStatus(id, status string) error
	SetNotes(id, notes string) error
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

// List returns a paginated list of jobs matching the filter.
func (s *service) List(filter JobFilter, p request.Pagination) ([]model.Job, int, error) {
	jobs, total, err := s.jobs.GetJobsByStatusPaged(filter.Status, p.Page, p.Limit)
	if err != nil {
		return nil, 0, response.ErrInternal.WithMessage("failed to fetch jobs")
	}
	return jobs, int(total), nil
}

// Get fetches a single job by ID.
func (s *service) Get(id string) (*model.Job, error) {
	job, err := s.jobs.GetJobByID(id)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, response.ErrNotFound.WithMessage("job not found")
		}
		return nil, response.ErrInternal.WithMessage("failed to fetch job")
	}
	return job, nil
}

// Dismiss dismisses a job: records discard, blocks re-entry, hides from feed, and backfills snapshot.
func (s *service) Dismiss(id string, input DismissInput) error {
	job, err := s.Get(id)
	if err != nil {
		return err
	}

	if err := s.discard.InsertDiscard(id, input.Reason, input.Note); err != nil {
		return response.ErrInternal.WithMessage("failed to discard job")
	}

	if err := s.ledger.UpdateSeenReason(job.JobPostID, model.SeenReasonUserDismissed); err != nil {
		return response.ErrInternal.WithMessage("failed to update seen ledger")
	}

	if err := s.jobs.UpdateJobStatus(id, model.JobStatusDismissed); err != nil {
		return response.ErrInternal.WithMessage("failed to update job status")
	}

	// Non-fatal: snapshot may not exist for older jobs
	_ = s.snapshot.UpdateSnapshotAction(id, model.JobStatusDismissed)

	return nil
}

// SetSaved updates the saved flag for a job and non-fatally backfills the snapshot.
func (s *service) SetSaved(id string, saved bool) error {
	if err := s.jobs.UpdateJobSaved(id, saved); err != nil {
		return response.ErrInternal.WithMessage("failed to update job")
	}
	if saved {
		_ = s.snapshot.UpdateSnapshotAction(id, model.JobStatusSaved)
	}
	return nil
}

// SetStatus updates the status of a job and non-fatally backfills the snapshot if applied.
func (s *service) SetStatus(id, status string) error {
	if err := s.jobs.UpdateJobStatus(id, status); err != nil {
		return response.ErrInternal.WithMessage("failed to update status")
	}
	if status == model.JobStatusApplied {
		_ = s.snapshot.UpdateSnapshotAction(id, model.JobStatusApplied)
	}
	return nil
}

// SetNotes updates the notes for a job.
func (s *service) SetNotes(id, notes string) error {
	if err := s.jobs.UpdateJobNotes(id, notes); err != nil {
		return response.ErrInternal.WithMessage("failed to update notes")
	}
	return nil
}
