package services

import (
	"context"
	"time"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	scancommon "github.com/trushilpatel/jobradar/service/scan/common"
)

// schedulerService implements SchedulerService for the scheduler worker.
// It uses ScanQueueStore directly — not part of the REST decorator chain.
type schedulerService struct {
	queue ScanQueueStore
}

// NewSchedulerService constructs the minimal service the scheduler worker needs.
// Use this in cmd/company_scan_scheduler — no company or log dependencies required.
func NewSchedulerService(queue ScanQueueStore) SchedulerService {
	return &schedulerService{queue: queue}
}

func (s *schedulerService) EnqueueDue(ctx context.Context, intervalMinutes int) (_ SchedulerCycleResult, err error) {
	ctx, span := tracing.Start(ctx, "scan.service.EnqueueDue")
	defer tracing.End(span, &err)
	res, err := s.queue.EnqueueDue(ctx, intervalMinutes, scancommon.QueueSourceScheduler)
	if err != nil {
		return SchedulerCycleResult{}, apperr.ErrInternal.WithMessage("failed to enqueue due companies")
	}
	return SchedulerCycleResult(res), nil
}

func (s *schedulerService) ResetStuck(ctx context.Context, staleDur time.Duration) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "scan.service.ResetStuck")
	defer tracing.End(span, &err)
	n, err := s.queue.ResetStuck(ctx, staleDur)
	if err != nil {
		return 0, apperr.ErrInternal.WithMessage("failed to reset stuck queue entries")
	}
	return n, nil
}

func (s *schedulerService) PurgeInactive(ctx context.Context) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "scan.service.PurgeInactive")
	defer tracing.End(span, &err)
	n, err := s.queue.PurgeInactive(ctx)
	if err != nil {
		return 0, apperr.ErrInternal.WithMessage("failed to purge inactive company queue entries")
	}
	return n, nil
}
