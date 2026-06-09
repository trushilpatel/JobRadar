package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// ledgerRepositoryService implements LedgerStore as a thin façade over the
// existing job.LedgerService (JobSeenLedgerRepository).
// Terminal — no next, no BaseNodeService.
type ledgerRepositoryService struct {
	inner jobsvc.LedgerService
}

// NewLedgerRepositoryService constructs a LedgerStore backed by the existing
// job pipeline ledger via the job.LedgerService façade.
func NewLedgerRepositoryService(inner jobsvc.LedgerService) LedgerStore {
	return &ledgerRepositoryService{inner: inner}
}

func (s *ledgerRepositoryService) HasSeenBatch(ctx context.Context, jobIDs []string) (_ map[string]bool, err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.store.HasSeenBatch")
	defer tracing.End(span, &err)

	if len(jobIDs) == 0 {
		return map[string]bool{}, nil
	}
	result, err := s.inner.HasSeenBatch(ctx, jobIDs)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("has seen batch: " + err.Error())
	}
	return result, nil
}

func (s *ledgerRepositoryService) MarkSeenBatch(ctx context.Context, jobIDs []string, companyID, reason string) (err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.store.MarkSeenBatch")
	defer tracing.End(span, &err)

	if len(jobIDs) == 0 {
		return nil
	}
	if err := s.inner.MarkSeenBatch(ctx, jobIDs, companyID, reason); err != nil {
		return errors.ErrInternal.WithMessage("mark seen batch: " + err.Error())
	}
	return nil
}

func (s *ledgerRepositoryService) MarkRemovedBatch(ctx context.Context, jobIDs []string) (err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.store.MarkRemovedBatch")
	defer tracing.End(span, &err)

	if len(jobIDs) == 0 {
		return nil
	}
	if err := s.inner.MarkRemovedBatch(ctx, jobIDs); err != nil {
		return errors.ErrInternal.WithMessage("mark removed batch: " + err.Error())
	}
	return nil
}

func (s *ledgerRepositoryService) GetActiveJobIDsByCompany(ctx context.Context, companyID string) (_ []string, err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.store.GetActiveJobIDsByCompany")
	defer tracing.End(span, &err)

	ids, err := s.inner.GetActiveJobIDsByCompany(ctx, companyID)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("get active job ids: " + err.Error())
	}
	return ids, nil
}
