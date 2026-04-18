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
	MoveUp(ctx context.Context, entryID string) error
	MoveDown(ctx context.Context, entryID string) error
}
