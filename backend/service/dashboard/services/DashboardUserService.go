package services

import (
	"context"

	appsvc "github.com/trushilpatel/jobradar/service/application/services"
	"github.com/trushilpatel/jobradar/service/dashboard/dto"
)

// ApplicationListReader is the narrow read interface the dashboard needs from the application domain.
type ApplicationListReader interface {
	ListAll(page, limit int) ([]appsvc.ApplicationWithJob, int, error)
}

// UserDashboardService aggregates all user-facing dashboard panel data.
type UserDashboardService interface {
	GetProgressStats(ctx context.Context) (*dto.ProgressStats, error)
	GetMarketStats(ctx context.Context) (*dto.MarketStats, error)
	GetHeatmap(ctx context.Context) ([]dto.WeeklyCount, error)
	GetInterviews(ctx context.Context) ([]dto.InterviewRow, error)
	GetVelocity(ctx context.Context, limit int) ([]dto.CompanyVelocityRow, error)
	GetTechTrends(ctx context.Context) (*dto.TechTrends, error)
}

// UserDashboardBaseService delegates all UserDashboardService methods to next.
type UserDashboardBaseService struct{ next UserDashboardService }

// NewUserDashboardBaseService constructs a UserDashboardBaseService.
func NewUserDashboardBaseService(next UserDashboardService) UserDashboardBaseService {
	if next == nil {
		panic("UserDashboardBaseService: next must not be nil")
	}
	return UserDashboardBaseService{next: next}
}

func (b *UserDashboardBaseService) GetProgressStats(ctx context.Context) (*dto.ProgressStats, error) {
	return b.next.GetProgressStats(ctx)
}

func (b *UserDashboardBaseService) GetMarketStats(ctx context.Context) (*dto.MarketStats, error) {
	return b.next.GetMarketStats(ctx)
}

func (b *UserDashboardBaseService) GetHeatmap(ctx context.Context) ([]dto.WeeklyCount, error) {
	return b.next.GetHeatmap(ctx)
}

func (b *UserDashboardBaseService) GetInterviews(ctx context.Context) ([]dto.InterviewRow, error) {
	return b.next.GetInterviews(ctx)
}

func (b *UserDashboardBaseService) GetVelocity(ctx context.Context, limit int) ([]dto.CompanyVelocityRow, error) {
	return b.next.GetVelocity(ctx, limit)
}

func (b *UserDashboardBaseService) GetTechTrends(ctx context.Context) (*dto.TechTrends, error) {
	return b.next.GetTechTrends(ctx)
}
