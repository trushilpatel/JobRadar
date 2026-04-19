package services

import (
	"context"
	"gorm.io/gorm"

	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/service/insights/dto"
)

type repositoryService struct {
	db *gorm.DB
}

// NewRepositoryService constructs a Store implementation.
func NewRepositoryService(db *gorm.DB) Store {
	return &repositoryService{db: db}
}

func (s *repositoryService) GetTechStackTrends(ctx context.Context, limit int) (results []dto.TechCount, err error) {
	ctx, span := tracing.Start(ctx, "insights.store.GetTechStackTrends")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Raw(`
		SELECT tech, COUNT(*) AS count
		FROM job_feed, jsonb_array_elements_text(tech_stack) AS tech
		WHERE tech_stack IS NOT NULL AND jsonb_typeof(tech_stack) = 'array'
		GROUP BY tech
		ORDER BY count DESC
		LIMIT ?
	`, limit).Scan(&results).Error
	return results, err
}

func (s *repositoryService) GetCompanyActivity(ctx context.Context) (results []dto.CompanyActivityItem, err error) {
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

func (s *repositoryService) GetPipelineFunnel(ctx context.Context) (_ []dto.FunnelStep, err error) {
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

	return []dto.FunnelStep{
		{Stage: "enriched", Count: total},
		{Stage: model.JobStatusSaved, Count: counts[model.JobStatusSaved]},
		{Stage: model.JobStatusApplied, Count: counts[model.JobStatusApplied]},
		{Stage: model.JobStatusDismissed, Count: counts[model.JobStatusDismissed]},
	}, nil
}

func (s *repositoryService) GetLocationBreakdown(ctx context.Context) (results []dto.LocationCount, err error) {
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

func (s *repositoryService) GetUSAStateBreakdown(ctx context.Context) (results []dto.StateCount, err error) {
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

func (s *repositoryService) GetSponsorshipTrend(ctx context.Context, days int) (results []dto.SponsorshipDay, err error) {
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

func (s *repositoryService) GetCompanyVelocity(ctx context.Context, limit int) (results []dto.CompanyVelocity, err error) {
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

func (s *repositoryService) GetRoleCategoryBreakdown(ctx context.Context) (results []dto.RoleCategoryCount, err error) {
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

func (s *repositoryService) GetPipelineCounts(ctx context.Context) (_ dto.PipelineCounts, err error) {
	ctx, span := tracing.Start(ctx, "insights.store.GetPipelineCounts")
	defer tracing.End(span, &err)
	var result struct {
		Pending int64
		Errors  int64
	}
	err = s.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(*) FILTER (WHERE l.reason = 'fetch' AND j.job_post_id IS NULL) AS pending,
			COUNT(*) FILTER (WHERE l.reason = 'error') AS errors
		FROM job_pipeline_ledger l
		LEFT JOIN job_feed j ON j.job_post_id = l.job_id
	`).Scan(&result).Error
	return dto.PipelineCounts{Pending: result.Pending, Errors: result.Errors}, err
}
