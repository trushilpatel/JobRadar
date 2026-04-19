package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/domain"
	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/service/insights/dto"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	"gorm.io/gorm"
)

type repositoryService struct {
	db *gorm.DB
}

// NewRepositoryService constructs the terminal InsightsService implementation backed by GORM.
func NewRepositoryService(db *gorm.DB) InsightsService {
	return &repositoryService{db: db}
}

func (s *repositoryService) TechStack(ctx context.Context) (_ interface{}, err error) {
	ctx, span := tracing.Start(ctx, "insights.repository.TechStack")
	defer tracing.End(span, &err)
	var results []dto.TechCount
	err = s.db.WithContext(ctx).Raw(`
		SELECT tech, COUNT(*) AS count
		FROM job_feed, jsonb_array_elements_text(tech_stack) AS tech
		WHERE tech_stack IS NOT NULL AND jsonb_typeof(tech_stack) = 'array'
		GROUP BY tech
		ORDER BY count DESC
		LIMIT ?
	`, 15).Scan(&results).Error
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch tech stack")
	}
	return results, nil
}

func (s *repositoryService) CompanyActivity(ctx context.Context) (_ interface{}, err error) {
	ctx, span := tracing.Start(ctx, "insights.repository.CompanyActivity")
	defer tracing.End(span, &err)
	var results []dto.CompanyActivityItem
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
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch company activity")
	}
	return results, nil
}

func (s *repositoryService) Funnel(ctx context.Context) (_ interface{}, err error) {
	ctx, span := tracing.Start(ctx, "insights.repository.Funnel")
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
		return nil, domain.ErrInternal.WithMessage("failed to fetch funnel")
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
		{Stage: jobcommon.JobStatusSaved, Count: counts[jobcommon.JobStatusSaved]},
		{Stage: jobcommon.JobStatusApplied, Count: counts[jobcommon.JobStatusApplied]},
		{Stage: jobcommon.JobStatusDismissed, Count: counts[jobcommon.JobStatusDismissed]},
	}, nil
}

func (s *repositoryService) Location(ctx context.Context) (_ interface{}, err error) {
	ctx, span := tracing.Start(ctx, "insights.repository.Location")
	defer tracing.End(span, &err)
	var countries []dto.LocationCount
	err = s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(country, 'Unknown') AS country, COUNT(*) AS count
		FROM job_analytics
		GROUP BY country
		ORDER BY count DESC
	`).Scan(&countries).Error
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch location breakdown")
	}

	var states []dto.StateCount
	err = s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(state, 'Unknown') AS state, COUNT(*) AS count
		FROM job_analytics
		WHERE country = 'USA' AND state IS NOT NULL
		GROUP BY state
		ORDER BY count DESC
		LIMIT 20
	`).Scan(&states).Error
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch state breakdown")
	}

	return map[string]interface{}{
		"countries": countries,
		"states":    states,
	}, nil
}

func (s *repositoryService) Sponsorship(ctx context.Context, filter dto.InsightsFilter) (_ interface{}, err error) {
	ctx, span := tracing.Start(ctx, "insights.repository.Sponsorship")
	defer tracing.End(span, &err)
	days := filter.Days
	if days <= 0 {
		days = 30
	}
	var results []dto.SponsorshipDay
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
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch sponsorship trend")
	}
	return results, nil
}

func (s *repositoryService) Velocity(ctx context.Context) (_ interface{}, err error) {
	ctx, span := tracing.Start(ctx, "insights.repository.Velocity")
	defer tracing.End(span, &err)
	var results []dto.CompanyVelocity
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
	`, 20).Scan(&results).Error
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch company velocity")
	}
	return results, nil
}

func (s *repositoryService) RoleCategories(ctx context.Context) (_ interface{}, err error) {
	ctx, span := tracing.Start(ctx, "insights.repository.RoleCategories")
	defer tracing.End(span, &err)
	var results []dto.RoleCategoryCount
	err = s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(role_category, 'other') AS category, COUNT(*) AS count
		FROM job_analytics
		GROUP BY role_category
		ORDER BY count DESC
	`).Scan(&results).Error
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch role categories")
	}
	return results, nil
}

func (s *repositoryService) GetPipelineCounts(ctx context.Context) (_ dto.PipelineCounts, err error) {
	ctx, span := tracing.Start(ctx, "insights.repository.GetPipelineCounts")
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
	if err != nil {
		return dto.PipelineCounts{}, domain.ErrInternal.WithMessage("failed to fetch pipeline counts")
	}
	return dto.PipelineCounts{Pending: result.Pending, Errors: result.Errors}, nil
}
