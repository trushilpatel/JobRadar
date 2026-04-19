package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/domain"
	"github.com/trushilpatel/jobradar/infra/tracing"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	"github.com/trushilpatel/jobradar/service/job/dto"
	"github.com/trushilpatel/jobradar/store"
)

type service struct {
	jobs      JobStore
	ledger    SeenLedgerWriter
	discard   DiscardStore
	snapshot  SnapshotWriter
	appWriter ApplicationWriter
}

// NewService constructs a Service implementation.
func NewService(jobs JobStore, ledger SeenLedgerWriter, discard DiscardStore, snapshot SnapshotWriter, appWriter ApplicationWriter) Service {
	return &service{
		jobs:      jobs,
		ledger:    ledger,
		discard:   discard,
		snapshot:  snapshot,
		appWriter: appWriter,
	}
}

// List returns a paginated, filtered list of jobs — all filtering happens at the DB level.
func (s *service) List(ctx context.Context, filter dto.JobFilter, page, limit int) (_ []jobmodel.Job, _ int, err error) {
	ctx, span := tracing.Start(ctx, "job.service.List")
	defer tracing.End(span, &err)
	jobs, total, err := s.jobs.GetJobs(ctx, filter, page, limit)
	if err != nil {
		return nil, 0, domain.ErrInternal.WithMessage("failed to fetch jobs")
	}
	return jobs, int(total), nil
}

// GetCounts returns a count of jobs per active status.
func (s *service) GetCounts(ctx context.Context) (_ *dto.JobCounts, err error) {
	ctx, span := tracing.Start(ctx, "job.service.GetCounts")
	defer tracing.End(span, &err)
	counts, err := s.jobs.GetJobCounts(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch job counts")
	}
	return counts, nil
}

// Get fetches a single job by ID.
func (s *service) Get(ctx context.Context, id string) (_ *jobmodel.Job, err error) {
	ctx, span := tracing.Start(ctx, "job.service.Get")
	defer tracing.End(span, &err)
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
func (s *service) Dismiss(ctx context.Context, id string, input dto.DismissInput) (err error) {
	ctx, span := tracing.Start(ctx, "job.service.Dismiss")
	defer tracing.End(span, &err)
	job, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	if err := s.discard.InsertDiscard(ctx, id, input.Reason, input.Note); err != nil {
		return domain.ErrInternal.WithMessage("failed to discard job")
	}

	if err := s.ledger.UpdateSeenReason(ctx, job.JobPostID, jobcommon.SeenReasonUserDismissed); err != nil {
		return domain.ErrInternal.WithMessage("failed to update seen ledger")
	}

	if err := s.jobs.UpdateJobStatus(ctx, id, jobcommon.JobStatusDismissed); err != nil {
		return domain.ErrInternal.WithMessage("failed to update job status")
	}

	// Non-fatal: snapshot may not exist for older jobs
	_ = s.snapshot.UpdateSnapshotAction(ctx, id, jobcommon.JobStatusDismissed)

	return nil
}

// SetSaved updates the saved flag for a job and non-fatally backfills the snapshot.
func (s *service) SetSaved(ctx context.Context, id string, saved bool) (err error) {
	ctx, span := tracing.Start(ctx, "job.service.SetSaved")
	defer tracing.End(span, &err)
	if err := s.jobs.UpdateJobSaved(ctx, id, saved); err != nil {
		return domain.ErrInternal.WithMessage("failed to update job")
	}
	if saved {
		_ = s.snapshot.UpdateSnapshotAction(ctx, id, jobcommon.JobStatusSaved)
	}
	return nil
}

// SetStatus updates the status of a job, backfills the snapshot, and auto-inserts
// an application tracker row so the dashboard InterviewingPanel stays in sync.
func (s *service) SetStatus(ctx context.Context, id, status string) (err error) {
	ctx, span := tracing.Start(ctx, "job.service.SetStatus")
	defer tracing.End(span, &err)
	if err := s.jobs.UpdateJobStatus(ctx, id, status); err != nil {
		return domain.ErrInternal.WithMessage("failed to update status")
	}
	switch status {
	case jobcommon.JobStatusApplied:
		_ = s.snapshot.UpdateSnapshotAction(ctx, id, jobcommon.JobStatusApplied)
		_ = s.appWriter.InsertAppliedApplication(id)
	case jobcommon.JobStatusInterviewing:
		_ = s.appWriter.InsertInterviewingApplication(id)
	case jobcommon.JobStatusOffer:
		_ = s.appWriter.InsertOfferApplication(id)
	}
	return nil
}

// SetNotes updates the notes for a job.
func (s *service) SetNotes(ctx context.Context, id, notes string) (err error) {
	ctx, span := tracing.Start(ctx, "job.service.SetNotes")
	defer tracing.End(span, &err)
	if err := s.jobs.UpdateJobNotes(ctx, id, notes); err != nil {
		return domain.ErrInternal.WithMessage("failed to update notes")
	}
	return nil
}

// MarkApplied advances the job status to "applied". It is a convenience method
// so callers (e.g. application service) do not need to import job domain constants.
func (s *service) MarkApplied(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "job.service.MarkApplied")
	defer tracing.End(span, &err)
	return s.SetStatus(ctx, id, jobcommon.JobStatusApplied)
}
