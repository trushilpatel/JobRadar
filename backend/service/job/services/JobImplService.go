package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/domain"
	"github.com/trushilpatel/jobradar/platform/tracing"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	"github.com/trushilpatel/jobradar/service/job/dto"
)

type implService struct {
	BaseService
	ledger    SeenLedgerWriter
	discard   DiscardStore
	snapshot  SnapshotWriter
	appWriter ApplicationWriter
}

// NewService constructs a Service implementation that orchestrates business logic
// and delegates pure DB operations to next (typically a repositoryService).
func NewService(next Service, ledger SeenLedgerWriter, discard DiscardStore, snapshot SnapshotWriter, appWriter ApplicationWriter) Service {
	return &implService{
		BaseService: NewBaseService(next),
		ledger:      ledger,
		discard:     discard,
		snapshot:    snapshot,
		appWriter:   appWriter,
	}
}

// Dismiss dismisses a job: records discard, blocks re-entry, hides from feed, and backfills snapshot.
func (s *implService) Dismiss(ctx context.Context, id string, input dto.DismissInput) (err error) {
	ctx, span := tracing.Start(ctx, "job.service.Dismiss")
	defer tracing.End(span, &err)

	job, err := s.next.Get(ctx, id)
	if err != nil {
		return err
	}

	if err := s.discard.InsertDiscard(ctx, id, input.Reason, input.Note); err != nil {
		return domain.ErrInternal.WithMessage("failed to discard job")
	}

	if err := s.ledger.UpdateSeenReason(ctx, job.JobPostID, jobcommon.SeenReasonUserDismissed); err != nil {
		return domain.ErrInternal.WithMessage("failed to update seen ledger")
	}

	if err := s.next.Dismiss(ctx, id, input); err != nil {
		return err
	}

	// Non-fatal: snapshot may not exist for older jobs
	_ = s.snapshot.UpdateSnapshotAction(ctx, id, jobcommon.JobStatusDismissed)

	return nil
}

// SetSaved updates the saved flag for a job and non-fatally backfills the snapshot.
func (s *implService) SetSaved(ctx context.Context, id string, saved bool) (err error) {
	ctx, span := tracing.Start(ctx, "job.service.SetSaved")
	defer tracing.End(span, &err)

	if err := s.next.SetSaved(ctx, id, saved); err != nil {
		return err
	}
	if saved {
		_ = s.snapshot.UpdateSnapshotAction(ctx, id, jobcommon.JobStatusSaved)
	}
	return nil
}

// SetStatus updates the status of a job, backfills the snapshot, and auto-inserts
// an application tracker row so the dashboard InterviewingPanel stays in sync.
func (s *implService) SetStatus(ctx context.Context, id, status string) (err error) {
	ctx, span := tracing.Start(ctx, "job.service.SetStatus")
	defer tracing.End(span, &err)

	if err := s.next.SetStatus(ctx, id, status); err != nil {
		return err
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
