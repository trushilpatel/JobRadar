package dashboard

import (
	"math"

	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/service/application"
)

// UserDashboardService aggregates all user-facing dashboard panel data.
type UserDashboardService interface {
	GetProgressStats() (*ProgressStats, error)
	GetMarketStats() (*MarketStats, error)
	GetHeatmap() ([]WeeklyCount, error)
	GetInterviews() ([]InterviewRow, error)
	GetVelocity(limit int) ([]CompanyVelocityRow, error)
	GetTechTrends() (*TechTrends, error)
}

type userDashboardService struct {
	store    UserDashboardStore
	appStore application.Store
}

// NewUserDashboardService constructs a UserDashboardService.
func NewUserDashboardService(store UserDashboardStore, appStore application.Store) UserDashboardService {
	return &userDashboardService{store: store, appStore: appStore}
}

func (s *userDashboardService) GetProgressStats() (*ProgressStats, error) {
	stats, err := s.store.GetProgressStats()
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch progress stats")
	}
	return &stats, nil
}

func (s *userDashboardService) GetMarketStats() (*MarketStats, error) {
	stats, err := s.store.GetMarketStats()
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch market stats")
	}
	return &stats, nil
}

func (s *userDashboardService) GetHeatmap() ([]WeeklyCount, error) {
	counts, err := s.store.GetWeeklyHiringActivity(52)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch heatmap data")
	}
	return counts, nil
}

// GetInterviews returns active applications mapped to InterviewRow, excluding rejected.
func (s *userDashboardService) GetInterviews() ([]InterviewRow, error) {
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
func (s *userDashboardService) GetVelocity(limit int) ([]CompanyVelocityRow, error) {
	rows, err := s.store.GetCompanyVelocityDetailed(limit)
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

// GetTechTrends returns quarterly tech adoption classified into Rising and New Entrant lists.
func (s *userDashboardService) GetTechTrends() (*TechTrends, error) {
	rows, err := s.store.GetTechStackQuarterlyAdoption()
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch tech trends")
	}

	var totalCompanies int
	if len(rows) > 0 {
		totalCompanies = rows[0].TotalCompanies
	}

	var rising, newEntrants []TechTrendRow

	for _, r := range rows {
		if totalCompanies == 0 {
			continue
		}

		q4Pct := float64(r.Q4) / float64(totalCompanies) * 100
		q2Pct := float64(r.Q2) / float64(totalCompanies) * 100

		row := TechTrendRow{
			Tech:        r.Tech,
			Q1:          r.Q1,
			Q2:          r.Q2,
			Q3:          r.Q3,
			Q4:          r.Q4,
			AdoptionPct: math.Round(q4Pct*10) / 10,
		}

		// Rising: >15% adoption, was present last quarter, grew >15% QoQ
		if q4Pct > 15 && r.Q3 > 0 {
			growth := float64(r.Q4-r.Q3) / float64(r.Q3)
			if growth > 0.15 {
				row.GrowthPct = math.Round(growth*1000) / 10 // percentage
				rising = append(rising, row)
			}
		}

		// New entrant: >15% adoption now, was <5% two quarters ago
		if q4Pct > 15 && q2Pct < 5 {
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
		TotalCompanies: totalCompanies,
		Rising:         rising,
		NewEntrants:    newEntrants,
	}, nil
}
