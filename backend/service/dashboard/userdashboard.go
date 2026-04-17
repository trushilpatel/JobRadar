package dashboard

import (
	"context"
	"math"

	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/service/application"
)

// UserDashboardService aggregates all user-facing dashboard panel data.
type UserDashboardService interface {
	GetProgressStats(ctx context.Context) (*ProgressStats, error)
	GetMarketStats(ctx context.Context) (*MarketStats, error)
	GetHeatmap(ctx context.Context) ([]WeeklyCount, error)
	GetInterviews(ctx context.Context) ([]InterviewRow, error)
	GetVelocity(ctx context.Context, limit int) ([]CompanyVelocityRow, error)
	GetTechTrends(ctx context.Context) (*TechTrends, error)
}

type userDashboardService struct {
	store    UserDashboardStore
	appStore application.Store
}

// NewUserDashboardService constructs a UserDashboardService.
func NewUserDashboardService(store UserDashboardStore, appStore application.Store) UserDashboardService {
	return &userDashboardService{store: store, appStore: appStore}
}

func (s *userDashboardService) GetProgressStats(ctx context.Context) (_ *ProgressStats, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.userdashboard.GetProgressStats")
	defer tracing.End(span, &err)
	stats, err := s.store.GetProgressStats(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch progress stats")
	}
	return &stats, nil
}

func (s *userDashboardService) GetMarketStats(ctx context.Context) (_ *MarketStats, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.userdashboard.GetMarketStats")
	defer tracing.End(span, &err)
	stats, err := s.store.GetMarketStats(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch market stats")
	}
	return &stats, nil
}

func (s *userDashboardService) GetHeatmap(ctx context.Context) (_ []WeeklyCount, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.userdashboard.GetHeatmap")
	defer tracing.End(span, &err)
	counts, err := s.store.GetWeeklyHiringActivity(ctx, 52)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch heatmap data")
	}
	return counts, nil
}

// GetInterviews returns active applications mapped to InterviewRow, excluding rejected.
func (s *userDashboardService) GetInterviews(ctx context.Context) (_ []InterviewRow, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.userdashboard.GetInterviews")
	defer tracing.End(span, &err)
	apps, err := s.appStore.GetLatestApplicationsWithJob()
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch interviews")
	}

	rows := make([]InterviewRow, 0, len(apps))
	for _, a := range apps {
		if a.Stage == model.StageRejected {
			continue
		}
		rows = append(rows, InterviewRow{
			ApplicationID: a.ID,
			JobID:         a.JobResultID,
			CompanyName:   a.CompanyName,
			JobTitle:      a.JobTitle,
			Stage:         a.Stage,
			DisplayStage:  toDisplayStage(a.Stage),
			StageAt:       a.StageAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	return rows, nil
}

// toDisplayStage maps a raw DB application stage to its UI display label.
func toDisplayStage(stage string) string {
	switch stage {
	case model.StageApplied:
		return model.DisplayStageWaiting
	case model.StagePhoneScreen, model.StageTechnical, model.StageFinalRound:
		return model.DisplayStageInterviewing
	case model.StageOffer:
		return model.DisplayStageOffer
	default:
		return stage
	}
}

// GetVelocity returns top-N companies ranked by current-week job count with 4-week sparklines.
func (s *userDashboardService) GetVelocity(ctx context.Context, limit int) (_ []CompanyVelocityRow, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.userdashboard.GetVelocity")
	defer tracing.End(span, &err)
	rows, err := s.store.GetCompanyVelocityDetailed(ctx, limit)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch velocity data")
	}

	for i := range rows {
		rows[i].Multiplier, rows[i].VelocityTag = classifyVelocity(
			rows[i].WeekCurrent,
			rows[i].Week1,
			rows[i].Week2,
			rows[i].Week3,
		)
	}
	return rows, nil
}

// classifyVelocity computes a multiplier and velocity tag from 4-week counts.
func classifyVelocity(current, w1, w2, w3 int) (float64, string) {
	avgPrior := float64(w1+w2+w3) / 3.0
	var multiplier float64
	if avgPrior == 0 {
		multiplier = 1.0
	} else {
		multiplier = float64(current) / avgPrior
	}
	multiplier = math.Round(multiplier*10) / 10

	var tag string
	switch {
	case multiplier >= 2.5:
		tag = model.VelocitySpike
	case multiplier >= 1.3:
		tag = model.VelocityRising
	default:
		tag = model.VelocityNormal
	}
	return multiplier, tag
}

// GetTechTrends returns quarterly tech job-count trends classified into Rising and New Entrant lists.
func (s *userDashboardService) GetTechTrends(ctx context.Context) (_ *TechTrends, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.userdashboard.GetTechTrends")
	defer tracing.End(span, &err)
	rows, err := s.store.GetTechStackQuarterlyAdoption(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch tech trends")
	}

	var totalJobs int
	if len(rows) > 0 {
		totalJobs = rows[0].TotalJobs
	}

	var rising, newEntrants []TechTrendRow

	for _, r := range rows {
		if totalJobs == 0 {
			continue
		}

		q4Pct := float64(r.Q4) / float64(totalJobs) * 100
		q2Pct := float64(r.Q2) / float64(totalJobs) * 100

		row := TechTrendRow{
			Tech:        r.Tech,
			Q1:          r.Q1,
			Q2:          r.Q2,
			Q3:          r.Q3,
			Q4:          r.Q4,
			AdoptionPct: math.Round(q4Pct*10) / 10,
		}

		// Rising: appears in >5% of this quarter's jobs, was present last quarter, grew >15% QoQ
		if q4Pct > 5 && r.Q3 > 0 {
			growth := float64(r.Q4-r.Q3) / float64(r.Q3)
			if growth > 0.15 {
				row.GrowthPct = math.Round(growth*1000) / 10
				rising = append(rising, row)
			}
		}

		// New entrant: >5% of this quarter's jobs, was <1% two quarters ago
		if q4Pct > 5 && q2Pct < 1 {
			newEntrants = append(newEntrants, row)
		}
	}

	// Cap at 10 per list (rows already ordered by q4 DESC from query)
	if len(rising) > 10 {
		rising = rising[:10]
	}
	if len(newEntrants) > 10 {
		newEntrants = newEntrants[:10]
	}

	return &TechTrends{
		TotalJobs:   totalJobs,
		Rising:      rising,
		NewEntrants: newEntrants,
	}, nil
}
