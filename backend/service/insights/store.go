package insights

import (
	"context"

	"gorm.io/gorm"

	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/model"
)

// --- Output types (moved from store/insights.go) ---

// TechCount holds the count of jobs mentioning a given technology.
type TechCount struct {
	Tech  string `json:"tech"`
	Count int    `json:"count"`
}

// CompanyActivityItem holds weekly job count per company.
type CompanyActivityItem struct {
	CompanyName string `json:"company_name"`
	Week        string `json:"week"`
	Count       int    `json:"count"`
}

// FunnelStep holds a single stage in the enrichment → action funnel.
type FunnelStep struct {
	Stage string `json:"stage"`
	Count int    `json:"count"`
}

// LocationCount holds job counts by country.
type LocationCount struct {
	Country string `json:"country"`
	Count   int    `json:"count"`
}

// StateCount holds job counts by US state.
type StateCount struct {
	State string `json:"state"`
	Count int    `json:"count"`
}

// SponsorshipDay holds daily sponsorship vs total enriched counts.
type SponsorshipDay struct {
	Date      string `json:"date"`
	Total     int    `json:"total"`
	Sponsored int    `json:"sponsored"`
}

// CompanyVelocity holds per-company job counts for the current and prior week.
type CompanyVelocity struct {
	CompanyID   string `json:"company_id"`
	CompanyName string `json:"company_name"`
	ThisWeek    int    `json:"this_week"`
	LastWeek    int    `json:"last_week"`
	Delta       int    `json:"delta"`
}

// RoleCategoryCount holds job counts per role category.
type RoleCategoryCount struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

// PipelineCounts holds the current depth of each pipeline stage.
type PipelineCounts struct {
	Pending int64 `json:"pending"`
	Errors  int64 `json:"errors"`
}

// --- Interface ---

// Store provides aggregated analytics over the job pipeline.
type Store interface {
	GetTechStackTrends(ctx context.Context, limit int) ([]TechCount, error)
	GetCompanyActivity(ctx context.Context) ([]CompanyActivityItem, error)
	GetPipelineFunnel(ctx context.Context) ([]FunnelStep, error)
	GetLocationBreakdown(ctx context.Context) ([]LocationCount, error)
	GetUSAStateBreakdown(ctx context.Context) ([]StateCount, error)
	GetSponsorshipTrend(ctx context.Context, days int) ([]SponsorshipDay, error)
	GetCompanyVelocity(ctx context.Context, limit int) ([]CompanyVelocity, error)
	GetRoleCategoryBreakdown(ctx context.Context) ([]RoleCategoryCount, error)
	GetPipelineCounts(ctx context.Context) (PipelineCounts, error)
}

// --- GORM implementation ---

type insightsStore struct {
	db *gorm.DB
}

// NewStore constructs a Store implementation.
func NewStore(db *gorm.DB) Store {
	return &insightsStore{db: db}
}

func (s *insightsStore) GetTechStackTrends(ctx context.Context, limit int) (results []TechCount, err error) {
	ctx, span := tracing.Start(ctx, "insights.store.GetTechStackTrends")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Raw(`
		SELECT tech, COUNT(*) AS count
		FROM job_feed, jsonb_array_elements_text(tech_stack) AS tech
		WHERE tech_stack IS NOT NULL
		GROUP BY tech
		ORDER BY count DESC
		LIMIT ?
	`, limit).Scan(&results).Error
	return results, err
}

func (s *insightsStore) GetCompanyActivity(ctx context.Context) (results []CompanyActivityItem, err error) {
	ctx, span := tracing.Start(ctx, "insights.store.GetCompanyActivity")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Raw(`
		SELECT
			c.name AS company_name,
			TO_CHAR(DATE_TRUNC('week', j.added_at), 'YYYY-MM-DD') AS week,
			COUNT(*) AS count
		FROM job_feed j
		JOIN companies c ON j.company_id = c.id
		GROUP BY c.name, DATE_TRUNC('week', j.added_at)
		ORDER BY week DESC, count DESC
	`).Scan(&results).Error
	return results, err
}

func (s *insightsStore) GetPipelineFunnel(ctx context.Context) (_ []FunnelStep, err error) {
	ctx, span := tracing.Start(ctx, "insights.store.GetPipelineFunnel")
	defer tracing.End(span, &err)
	var rows []struct {
		Action *string
		Count  int
	}
	err = s.db.WithContext(ctx).Raw(`
		SELECT user_action AS action, COUNT(*) AS count
		FROM job_analytics
		GROUP BY user_action
	`).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	counts := map[string]int{}
	total := 0
	for _, r := range rows {
		total += r.Count
		if r.Action != nil {
			counts[*r.Action] = r.Count
		}
	}

	return []FunnelStep{
		{Stage: "enriched", Count: total},
		{Stage: model.JobStatusSaved, Count: counts[model.JobStatusSaved]},
		{Stage: model.JobStatusApplied, Count: counts[model.JobStatusApplied]},
		{Stage: model.JobStatusDismissed, Count: counts[model.JobStatusDismissed]},
	}, nil
}

func (s *insightsStore) GetLocationBreakdown(ctx context.Context) (results []LocationCount, err error) {
	ctx, span := tracing.Start(ctx, "insights.store.GetLocationBreakdown")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(country, 'Unknown') AS country, COUNT(*) AS count
		FROM job_analytics
		GROUP BY country
		ORDER BY count DESC
	`).Scan(&results).Error
	return results, err
}

func (s *insightsStore) GetUSAStateBreakdown(ctx context.Context) (results []StateCount, err error) {
	ctx, span := tracing.Start(ctx, "insights.store.GetUSAStateBreakdown")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(state, 'Unknown') AS state, COUNT(*) AS count
		FROM job_analytics
		WHERE country = 'USA' AND state IS NOT NULL
		GROUP BY state
		ORDER BY count DESC
		LIMIT 20
	`).Scan(&results).Error
	return results, err
}

func (s *insightsStore) GetSponsorshipTrend(ctx context.Context, days int) (results []SponsorshipDay, err error) {
	ctx, span := tracing.Start(ctx, "insights.store.GetSponsorshipTrend")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Raw(`
		SELECT
			TO_CHAR(snapshotted_at, 'YYYY-MM-DD') AS date,
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE sponsorship = true) AS sponsored
		FROM job_analytics
		WHERE snapshotted_at >= CURRENT_DATE - ? * INTERVAL '1 day'
		GROUP BY snapshotted_at
		ORDER BY snapshotted_at ASC
	`, days).Scan(&results).Error
	return results, err
}

func (s *insightsStore) GetCompanyVelocity(ctx context.Context, limit int) (results []CompanyVelocity, err error) {
	ctx, span := tracing.Start(ctx, "insights.store.GetCompanyVelocity")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Raw(`
		WITH weekly AS (
			SELECT
				s.company_id,
				c.name AS company_name,
				COUNT(*) FILTER (
					WHERE s.snapshotted_at >= DATE_TRUNC('week', CURRENT_DATE)
				) AS this_week,
				COUNT(*) FILTER (
					WHERE s.snapshotted_at >= DATE_TRUNC('week', CURRENT_DATE) - INTERVAL '7 days'
					  AND s.snapshotted_at <  DATE_TRUNC('week', CURRENT_DATE)
				) AS last_week
			FROM job_analytics s
			JOIN companies c ON s.company_id = c.id
			WHERE s.snapshotted_at >= DATE_TRUNC('week', CURRENT_DATE) - INTERVAL '7 days'
			GROUP BY s.company_id, c.name
		)
		SELECT *, (this_week - last_week) AS delta
		FROM weekly
		WHERE this_week > 0 OR last_week > 0
		ORDER BY delta DESC, this_week DESC
		LIMIT ?
	`, limit).Scan(&results).Error
	return results, err
}

func (s *insightsStore) GetRoleCategoryBreakdown(ctx context.Context) (results []RoleCategoryCount, err error) {
	ctx, span := tracing.Start(ctx, "insights.store.GetRoleCategoryBreakdown")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(role_category, 'other') AS category, COUNT(*) AS count
		FROM job_analytics
		GROUP BY role_category
		ORDER BY count DESC
	`).Scan(&results).Error
	return results, err
}

func (s *insightsStore) GetPipelineCounts(ctx context.Context) (_ PipelineCounts, err error) {
	ctx, span := tracing.Start(ctx, "insights.store.GetPipelineCounts")
	defer tracing.End(span, &err)
	var result struct {
		Pending int64
		Errors  int64
	}
	err = s.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(*) FILTER (
				WHERE l.reason = 'fetch'
				AND NOT EXISTS (SELECT 1 FROM job_feed j WHERE j.job_post_id = l.job_id)
			) AS pending,
			COUNT(*) FILTER (WHERE l.reason = 'error') AS errors
		FROM job_pipeline_ledger l
	`).Scan(&result).Error
	return PipelineCounts{Pending: result.Pending, Errors: result.Errors}, err
}
