package services

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/service/scan/dto"
)

// SchedulerService is the narrow interface the scheduler worker depends on.
type SchedulerService interface {
	EnqueueDue(ctx context.Context, intervalMinutes int) (int, error)
	ResetStuck(ctx context.Context, staleDur time.Duration) error
}

// Service defines all business operations for the scan domain.
type Service interface {
	SchedulerService
	TriggerScan(ctx context.Context, companyID string) error
	TriggerScanAll(ctx context.Context) (int, error)
	GetStats(ctx context.Context, companyID string) (*dto.ScanStatsView, error)
	ListQueue(ctx context.Context, filter dto.QueueFilter) ([]dto.QueueEntryView, error)
	RemoveFromQueue(ctx context.Context, entryID string) error
}

// BaseService auto-delegates all Service methods to next.
// Embed this in decorators and override only the methods they own.
type BaseService struct {
	next Service
}

// NewBaseService constructs a BaseService wrapping next.
// Panics if next is nil to surface wiring mistakes early.
func NewBaseService(next Service) BaseService {
	if next == nil {
		panic("BaseService: next must not be nil — check wire setup")
	}
	return BaseService{next: next}
}

func (b *BaseService) EnqueueDue(ctx context.Context, intervalMinutes int) (int, error) {
	return b.next.EnqueueDue(ctx, intervalMinutes)
}

func (b *BaseService) ResetStuck(ctx context.Context, staleDur time.Duration) error {
	return b.next.ResetStuck(ctx, staleDur)
}

func (b *BaseService) TriggerScan(ctx context.Context, companyID string) error {
	return b.next.TriggerScan(ctx, companyID)
}

func (b *BaseService) TriggerScanAll(ctx context.Context) (int, error) {
	return b.next.TriggerScanAll(ctx)
}

func (b *BaseService) GetStats(ctx context.Context, companyID string) (*dto.ScanStatsView, error) {
	return b.next.GetStats(ctx, companyID)
}

func (b *BaseService) ListQueue(ctx context.Context, filter dto.QueueFilter) ([]dto.QueueEntryView, error) {
	return b.next.ListQueue(ctx, filter)
}

func (b *BaseService) RemoveFromQueue(ctx context.Context, entryID string) error {
	return b.next.RemoveFromQueue(ctx, entryID)
}
