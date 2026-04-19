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

// schedulerService implements SchedulerService. Embedded by service so the
// logic lives in one place and both constructors share it.
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

type service struct {
	*schedulerService
	scanLog    ScanLogReader
	companySvc company.Service
	config     Config
}

// NewService constructs a Service implementation.
func NewService(queue ScanQueueStore, scanLog ScanLogReader, companySvc company.Service, cfg Config) Service {
	return &service{
		schedulerService: &schedulerService{queue: queue},
		scanLog:          scanLog,
		companySvc:       companySvc,
		config:           cfg,
	}
}

// TriggerScan enqueues a manual scan for a company. Verifies the company exists first.
func (s *service) TriggerScan(ctx context.Context, companyID string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.service.TriggerScan")
	defer tracing.End(span, &err)
	if _, err := s.companySvc.Get(ctx, companyID); err != nil {
		return err // already a ServiceError from company.Service
	}
	if err := s.queue.Enqueue(ctx, companyID, scancommon.QueueSourceManual); err != nil {
		return domain.ErrInternal.WithMessage("failed to trigger scan")
	}
	return nil
}

// TriggerScanAll enqueues a manual scan for every active company.
// Returns the number of companies successfully enqueued.
func (s *service) TriggerScanAll(ctx context.Context) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "scan.service.TriggerScanAll")
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
		if err := s.queue.Enqueue(ctx, c.ID, scancommon.QueueSourceManual); err != nil {
			continue
		}
		count++
	}
	return count, nil
}

// GetStats returns a summary of scan history for the given company.
func (s *service) GetStats(ctx context.Context, companyID string) (_ *dto.ScanStatsView, err error) {
	ctx, span := tracing.Start(ctx, "scan.service.GetStats")
	defer tracing.End(span, &err)
	stats, err := s.scanLog.GetScanStats(ctx, companyID)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch scan stats")
	}
	return &stats, nil
}

// ListQueue returns all active queue entries enriched with company name, adapter, and next_scan_at.
func (s *service) ListQueue(ctx context.Context, filter dto.QueueFilter) (_ []dto.QueueEntryView, err error) {
	ctx, span := tracing.Start(ctx, "scan.service.ListQueue")
	defer tracing.End(span, &err)
	items, err := s.queue.ListQueue(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch queue")
	}

	allCompanies, err := s.companySvc.GetAll(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch companies")
	}

	companyMap := make(map[string]company.CompanyView, len(allCompanies))
	for _, c := range allCompanies {
		companyMap[c.ID] = c
	}

	views := make([]dto.QueueEntryView, 0, len(items))
	for _, item := range items {
		// Apply status filter if set
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		// Apply source filter if set
		if filter.Source != "" && item.Source != filter.Source {
			continue
		}

		v := dto.QueueEntryView{
			ID:        item.ID,
			CompanyID: item.CompanyID,
			Status:    item.Status,
			Source:    item.Source,
			Priority:  item.Priority,
			QueuedAt:  item.QueuedAt,
			StartedAt: item.StartedAt,
		}
		if c, ok := companyMap[item.CompanyID]; ok {
			v.CompanyName = c.Name
			v.AdapterType = c.AdapterType
			if c.LastScanAt != nil {
				t := c.LastScanAt.Add(time.Duration(s.config.ScanIntervalMinutes) * time.Minute)
				v.NextScanAt = &t
			}
		}
		views = append(views, v)
	}

	return views, nil
}

// RemoveFromQueue removes a queued entry by ID.
func (s *service) RemoveFromQueue(ctx context.Context, entryID string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.service.RemoveFromQueue")
	defer tracing.End(span, &err)
	if err := s.queue.Remove(ctx, entryID); err != nil {
		return domain.ErrInternal.WithMessage("failed to remove queue entry")
	}
	return nil
}

// MoveUp raises a queue entry one position.
func (s *service) MoveUp(ctx context.Context, entryID string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.service.MoveUp")
	defer tracing.End(span, &err)
	if err := s.queue.MoveUp(ctx, entryID); err != nil {
		return domain.ErrInternal.WithMessage("failed to move queue entry")
	}
	return nil
}

// MoveDown lowers a queue entry one position.
func (s *service) MoveDown(ctx context.Context, entryID string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.service.MoveDown")
	defer tracing.End(span, &err)
	if err := s.queue.MoveDown(ctx, entryID); err != nil {
		return domain.ErrInternal.WithMessage("failed to move queue entry")
	}
	return nil
}
