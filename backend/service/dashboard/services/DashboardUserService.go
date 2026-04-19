package services

import (
	"context"

	"github.com/trushilpatel/jobradar/service/dashboard/dto"
)

// UserDashboardService aggregates all user-facing dashboard panel data.
type UserDashboardService interface {
	GetProgressStats(ctx context.Context) (*dto.ProgressStats, error)
	GetMarketStats(ctx context.Context) (*dto.MarketStats, error)
	GetHeatmap(ctx context.Context) ([]dto.WeeklyCount, error)
	GetInterviews(ctx context.Context) ([]dto.InterviewRow, error)
	GetVelocity(ctx context.Context, limit int) ([]dto.CompanyVelocityRow, error)
	GetTechTrends(ctx context.Context) (*dto.TechTrends, error)
}
