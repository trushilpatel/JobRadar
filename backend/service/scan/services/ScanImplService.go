package services

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/service/company"
	scancommon "github.com/trushilpatel/jobradar/service/scan/common"
	"github.com/trushilpatel/jobradar/service/scan/dto"
)

// Config holds per-service configuration for the scan domain.
type Config struct {
	ScanIntervalMinutes int
}

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

func (s *schedulerService) EnqueueDue(ctx context.Context, intervalMinutes int) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "scan.service.EnqueueDue")
	defer tracing.End(span, &err)
	n, err := s.queue.EnqueueDue(ctx, intervalMinutes, scancommon.QueueSourceScheduler)
	if err != nil {
		return 0, errors.ErrInternal.WithMessage("failed to enqueue due companies")
	}
	return n, nil
}

func (s *schedulerService) ResetStuck(ctx context.Context, staleDur time.Duration) (err error) {
	ctx, span := tracing.Start(ctx, "scan.service.ResetStuck")
	defer tracing.End(span, &err)
	if err := s.queue.ResetStuck(ctx, staleDur); err != nil {
		return errors.ErrInternal.WithMessage("failed to reset stuck queue entries")
	}
	return nil
}

func (s *schedulerService) PurgeInactive(ctx context.Context) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "scan.service.PurgeInactive")
	defer tracing.End(span, &err)
	n, err := s.queue.PurgeInactive(ctx)
	if err != nil {
		return 0, errors.ErrInternal.WithMessage("failed to purge inactive company queue entries")
	}
	return n, nil
}

// implService is the business-logic decorator. It embeds BaseService so all
// methods not overridden here are auto-delegated to the next layer.
type implService struct {
	BaseService
	companySvc company.Service
	config     Config
}

// NewService constructs the ImplService decorator.
func NewService(next Service, companySvc company.Service, cfg Config) Service {
	return &implService{
		BaseService: NewBaseService(next),
		companySvc:  companySvc,
		config:      cfg,
	}
}

// TriggerScan verifies the company exists, then delegates enqueue to next.
func (s *implService) TriggerScan(ctx context.Context, companyID string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.impl.TriggerScan")
	defer tracing.End(span, &err)
	if _, err := s.companySvc.Get(ctx, companyID); err != nil {
		return err // already a ServiceError from company.Service
	}
	return s.next.TriggerScan(ctx, companyID)
}

// ListQueue fetches raw entries from next, then enriches them with company metadata.
// Only the companies referenced by the queue entries are fetched.
func (s *implService) ListQueue(ctx context.Context, filter dto.QueueFilter) (_ []dto.QueueEntryView, err error) {
	ctx, span := tracing.Start(ctx, "scan.impl.ListQueue")
	defer tracing.End(span, &err)

	views, err := s.next.ListQueue(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Collect the unique company IDs referenced by the queue entries.
	seen := make(map[string]struct{}, len(views))
	ids := make([]string, 0, len(views))
	for _, v := range views {
		if _, ok := seen[v.CompanyID]; !ok {
			seen[v.CompanyID] = struct{}{}
			ids = append(ids, v.CompanyID)
		}
	}

	companies, err := s.companySvc.GetByIDs(ctx, ids)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch companies")
	}

	companyMap := make(map[string]company.CompanyView, len(companies))
	for _, c := range companies {
		companyMap[c.ID] = c
	}

	for i, v := range views {
		if c, ok := companyMap[v.CompanyID]; ok {
			views[i].CompanyName = c.Name
			views[i].AdapterType = c.AdapterType
			if c.LastScanAt != nil {
				t := c.LastScanAt.Add(time.Duration(s.config.ScanIntervalMinutes) * time.Minute)
				views[i].NextScanAt = &t
			}
		}
	}

	return views, nil
}
