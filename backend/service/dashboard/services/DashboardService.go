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
