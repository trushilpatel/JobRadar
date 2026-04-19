package services

import (
	"context"

	"github.com/trushilpatel/jobradar/service/dashboard/dto"
)

// DashboardService defines all dashboard aggregation operations.
type DashboardService interface {
	GetStats(ctx context.Context) (*dto.StatsView, error)
	GetActivity(ctx context.Context, days int) ([]dto.ActivityView, error)
}

// DashboardBaseService delegates all DashboardService methods to next.
type DashboardBaseService struct{ next DashboardService }

// NewDashboardBaseService constructs a DashboardBaseService.
func NewDashboardBaseService(next DashboardService) DashboardBaseService {
	if next == nil {
		panic("DashboardBaseService: next must not be nil")
	}
	return DashboardBaseService{next: next}
}

func (b *DashboardBaseService) GetStats(ctx context.Context) (*dto.StatsView, error) {
	return b.next.GetStats(ctx)
}

func (b *DashboardBaseService) GetActivity(ctx context.Context, days int) ([]dto.ActivityView, error) {
	return b.next.GetActivity(ctx, days)
}
