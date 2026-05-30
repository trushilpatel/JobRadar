package services

import (
	"context"
	"math"
	"sort"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/identity"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/service/dashboard/dto"
	insightscommon "github.com/trushilpatel/jobradar/service/insights/common"
)

// userDashboardImplService is the terminal implementation of UserDashboardService.
// It aggregates DB data from UserDashboardStore and cross-service data from ApplicationListReader.
type userDashboardImplService struct {
	store    UserDashboardStore
	appStore ApplicationListReader
}

// NewUserDashboardService constructs the terminal UserDashboardService implementation.
func NewUserDashboardService(store UserDashboardStore, appStore ApplicationListReader) UserDashboardService {
	return &userDashboardImplService{store: store, appStore: appStore}
}

func (s *userDashboardImplService) GetProgressStats(ctx context.Context) (_ *dto.ProgressStats, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.userdashboard.GetProgressStats")
	defer tracing.End(span, &err)
	userID := identity.UserIDFrom(ctx)
	stats, err := s.store.GetProgressStats(ctx, userID)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch progress stats")
	}
	return &stats, nil
}

func (s *userDashboardImplService) GetMarketStats(ctx context.Context) (_ *dto.MarketStats, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.userdashboard.GetMarketStats")
	defer tracing.End(span, &err)
	stats, err := s.store.GetMarketStats(ctx)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch market stats")
	}
	return &stats, nil
}

func (s *userDashboardImplService) GetHeatmap(ctx context.Context) (_ []dto.WeeklyCount, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.userdashboard.GetHeatmap")
	defer tracing.End(span, &err)
	counts, err := s.store.GetWeeklyHiringActivity(ctx, 52)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch heatmap data")
	}
	return counts, nil
}

// GetInterviews returns active applications mapped to InterviewRow, excluding rejected.
func (s *userDashboardImplService) GetInterviews(ctx context.Context) (_ []dto.InterviewRow, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.userdashboard.GetInterviews")
	defer tracing.End(span, &err)
	apps, err := s.appStore.ListActive(ctx)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch interviews")
	}

	rows := make([]dto.InterviewRow, 0, len(apps))
	for _, a := range apps {
		rows = append(rows, dto.InterviewRow{
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

// displayStageMap maps DB stage keys to their UI display labels.
// Defined locally to avoid coupling to the application domain's constants package.
var displayStageMap = map[string]string{
	"applied":      "Waiting",
	"phone_screen": "Interviewing",
	"technical":    "Interviewing",
	"final_round":  "Interviewing",
	"offer":        "Offer received",
}

// toDisplayStage maps a raw DB application stage to its UI display label.
func toDisplayStage(stage string) string {
	if label, ok := displayStageMap[stage]; ok {
		return label
	}
	return stage
}

// GetVelocity returns top-N companies ranked by current-week job count with 4-week sparklines.
func (s *userDashboardImplService) GetVelocity(ctx context.Context, limit int) (_ []dto.CompanyVelocityRow, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.userdashboard.GetVelocity")
	defer tracing.End(span, &err)
	rows, err := s.store.GetCompanyVelocityDetailed(ctx, limit)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch velocity data")
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
// When the prior three weeks are all zero (new company, or first data in the
// window) and the current week has postings, we treat it as a spike rather
// than normal — a multiplier of 1.0 (the old fallback) was always below the
// 1.3 rising threshold, causing every new company to render "—" in the UI.
func classifyVelocity(current, w1, w2, w3 int) (float64, string) {
	avgPrior := float64(w1+w2+w3) / 3.0
	var multiplier float64
	switch {
	case avgPrior == 0 && current > 0:
		// No baseline yet but jobs posted this week — classify as spike.
		multiplier = 3.0
	case avgPrior == 0:
		// No baseline and no current week postings — truly inactive.
		multiplier = 0
	default:
		multiplier = float64(current) / avgPrior
	}
	multiplier = math.Round(multiplier*10) / 10

	var tag string
	switch {
	case multiplier >= 2.5:
		tag = insightscommon.VelocitySpike
	case multiplier >= 1.3:
		tag = insightscommon.VelocityRising
	default:
		tag = insightscommon.VelocityNormal
	}
	return multiplier, tag
}

// GetTechTrends returns monthly tech job-count trends classified into Rising and New Entrant lists.
func (s *userDashboardImplService) GetTechTrends(ctx context.Context) (_ *dto.TechTrends, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.userdashboard.GetTechTrends")
	defer tracing.End(span, &err)
	rows, err := s.store.GetTechStackMonthlyAdoption(ctx)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch tech trends")
	}

	var totalJobs int
	if len(rows) > 0 {
		totalJobs = rows[0].TotalJobs
	}

	var rising, newEntrants []dto.TechTrendRow

	for _, r := range rows {
		if totalJobs == 0 {
			continue
		}

		m4Pct := float64(r.M4) / float64(totalJobs) * 100
		m2Pct := float64(r.M2) / float64(totalJobs) * 100

		row := dto.TechTrendRow{
			Tech:        r.Tech,
			M1:          r.M1,
			M2:          r.M2,
			M3:          r.M3,
			M4:          r.M4,
			AdoptionPct: math.Round(m4Pct*10) / 10,
		}

		// Rising: appears in >=2% of this month's jobs AND m4 >= 10, and either
		// grew >15% MoM or is brand-new this month (M3=0).
		if m4Pct >= 2.0 && r.M4 >= 10 {
			if r.M3 > 0 {
				growth := float64(r.M4-r.M3) / float64(r.M3)
				if growth > 0.15 {
					row.GrowthPct = math.Round(growth*1000) / 10
					rising = append(rising, row)
				}
			} else {
				row.IsNew = true
				rising = append(rising, row)
			}
		}

		// New entrant: >=2% of this month's jobs AND m4>=10, was <1% two months ago.
		if m4Pct >= 2.0 && r.M4 >= 10 && m2Pct < 1.0 {
			newEntrants = append(newEntrants, row)
		}
	}

	// Sort Rising by momentum score (growth_pct * log(m4+1)) descending.
	// IsNew rows have growth_pct=0 so they always rank below non-new rows.
	// Among IsNew rows they are ordered by log(m4+1) descending.
	sort.Slice(rising, func(i, j int) bool {
		scoreI := rising[i].GrowthPct * math.Log(float64(rising[i].M4)+1)
		scoreJ := rising[j].GrowthPct * math.Log(float64(rising[j].M4)+1)
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return math.Log(float64(rising[i].M4)+1) > math.Log(float64(rising[j].M4)+1)
	})

	// Cap at 10 per list.
	if len(rising) > 10 {
		rising = rising[:10]
	}
	if len(newEntrants) > 10 {
		newEntrants = newEntrants[:10]
	}

	return &dto.TechTrends{
		TotalJobs:   totalJobs,
		Rising:      rising,
		NewEntrants: newEntrants,
	}, nil
}
