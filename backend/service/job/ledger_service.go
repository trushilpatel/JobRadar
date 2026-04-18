package job

import (
	"context"

	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/infra/tracing"
)

// LedgerService is the interface pipeline workers use for job deduplication and removal tracking.
type LedgerService interface {
	HasSeenBatch(ctx context.Context, jobIDs []string) (map[string]bool, error)
	MarkSeenBatch(ctx context.Context, jobIDs []string, companyID, reason string) error
	GetActiveJobIDsByCompany(ctx context.Context, companyID string) ([]string, error)
	MarkRemovedBatch(ctx context.Context, jobIDs []string) error
}

type ledgerService struct {
	store SeenLedgerStore
}

// NewLedgerService returns a LedgerService backed by the seen ledger store.
func NewLedgerService(s SeenLedgerStore) LedgerService {
	return &ledgerService{store: s}
}

func (s *ledgerService) HasSeenBatch(ctx context.Context, jobIDs []string) (_ map[string]bool, err error) {
	ctx, span := tracing.Start(ctx, "job.ledgerservice.HasSeenBatch")
	defer tracing.End(span, &err)
	result, err := s.store.HasSeenBatch(ctx, jobIDs)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to check seen batch")
	}
	return result, nil
}

func (s *ledgerService) MarkSeenBatch(ctx context.Context, jobIDs []string, companyID, reason string) (err error) {
	ctx, span := tracing.Start(ctx, "job.ledgerservice.MarkSeenBatch")
	defer tracing.End(span, &err)
	if err := s.store.MarkSeenBatch(ctx, jobIDs, companyID, reason); err != nil {
		return domain.ErrInternal.WithMessage("failed to mark seen batch")
	}
	return nil
}

func (s *ledgerService) GetActiveJobIDsByCompany(ctx context.Context, companyID string) (_ []string, err error) {
	ctx, span := tracing.Start(ctx, "job.ledgerservice.GetActiveJobIDsByCompany")
	defer tracing.End(span, &err)
	ids, err := s.store.GetActiveJobIDsByCompany(ctx, companyID)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to get active job IDs")
	}
	return ids, nil
}

func (s *ledgerService) MarkRemovedBatch(ctx context.Context, jobIDs []string) (err error) {
	ctx, span := tracing.Start(ctx, "job.ledgerservice.MarkRemovedBatch")
	defer tracing.End(span, &err)
	if err := s.store.MarkRemovedBatch(ctx, jobIDs); err != nil {
		return domain.ErrInternal.WithMessage("failed to mark removed batch")
	}
	return nil
}
