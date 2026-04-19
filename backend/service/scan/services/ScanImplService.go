package services

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/common/domain"
	"github.com/trushilpatel/jobradar/infra/tracing"
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
		return 0, domain.ErrInternal.WithMessage("failed to enqueue due companies")
	}
	return n, nil
}

func (s *schedulerService) ResetStuck(ctx context.Context, staleDur time.Duration) (err error) {
	ctx, span := tracing.Start(ctx, "scan.service.ResetStuck")
	defer tracing.End(span, &err)
	if err := s.queue.ResetStuck(ctx, staleDur); err != nil {
		return domain.ErrInternal.WithMessage("failed to reset stuck queue entries")
	}
	return nil
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

// TriggerScanAll enqueues every active company for an immediate manual scan.
func (s *implService) TriggerScanAll(ctx context.Context) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "scan.impl.TriggerScanAll")
	defer tracing.End(span, &err)
	companies, err := s.companySvc.GetAll(ctx)
	if err != nil {
		return 0, domain.ErrInternal.WithMessage("failed to fetch companies")
	}
	var count int
	for _, c := range companies {
		if !c.Active {
			continue
		}
		if err := s.next.TriggerScan(ctx, c.ID); err != nil {
			continue
		}
		count++
	}
	return count, nil
}

// ListQueue fetches raw entries from next, then enriches them with company metadata.
func (s *implService) ListQueue(ctx context.Context, filter dto.QueueFilter) (_ []dto.QueueEntryView, err error) {
	ctx, span := tracing.Start(ctx, "scan.impl.ListQueue")
	defer tracing.End(span, &err)

	views, err := s.next.ListQueue(ctx, filter)
	if err != nil {
		return nil, err
	}

	allCompanies, err := s.companySvc.GetAll(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch companies")
	}

	companyMap := make(map[string]company.CompanyView, len(allCompanies))
	for _, c := range allCompanies {
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
