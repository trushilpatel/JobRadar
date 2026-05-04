package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
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

func (s *repositoryService) TechStack(ctx context.Context) (_ any, err error) {
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
		return nil, errors.ErrInternal.WithMessage("failed to fetch tech stack")
	}
	return results, nil
}

func (s *repositoryService) CompanyActivity(ctx context.Context) (_ any, err error) {
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
		WHERE j.added_at >= NOW() - INTERVAL '90 days'
		GROUP BY c.name, DATE_TRUNC('week', j.added_at)
		ORDER BY week DESC, count DESC
		LIMIT 200
	`).Scan(&results).Error
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch company activity")
	}
	return results, nil
}

func (s *repositoryService) Funnel(ctx context.Context) (_ any, err error) {
	ctx, span := tracing.Start(ctx, "insights.repository.Funnel")
	defer tracing.End(span, &err)
	var result struct {
		Enriched  int `gorm:"column:enriched"`
		Saved     int `gorm:"column:saved"`
		Applied   int `gorm:"column:applied"`
		Dismissed int `gorm:"column:dismissed"`
	}
	// enriched = all-time total from job_analytics (append-only, survives dismissal).
	// saved/applied/dismissed = current distribution from job_feed (rows are never deleted).
	// applied includes interviewing and offer — all stages beyond initial application.
	err = s.db.WithContext(ctx).Raw(`
		SELECT
			(SELECT COUNT(*) FROM job_analytics)                                         AS enriched,
			COUNT(*) FILTER (WHERE status = 'saved')                                     AS saved,
			COUNT(*) FILTER (WHERE status IN ('applied', 'interviewing', 'offer'))       AS applied,
			COUNT(*) FILTER (WHERE status = 'dismissed')                                 AS dismissed
		FROM job_feed
	`).Scan(&result).Error
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch funnel")
	}

	return []dto.FunnelStep{
		{Stage: "enriched", Count: result.Enriched},
		{Stage: jobcommon.JobStatusSaved, Count: result.Saved},
		{Stage: jobcommon.JobStatusApplied, Count: result.Applied},
		{Stage: jobcommon.JobStatusDismissed, Count: result.Dismissed},
	}, nil
}

func (s *repositoryService) Location(ctx context.Context) (_ any, err error) {
	ctx, span := tracing.Start(ctx, "insights.repository.Location")
	defer tracing.End(span, &err)
	var countries []dto.LocationCount
	err = s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(country, 'Unknown') AS country, COUNT(*) AS count
		FROM job_analytics
		GROUP BY country
		ORDER BY count DESC
		LIMIT 50
	`).Scan(&countries).Error
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch location breakdown")
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
		return nil, errors.ErrInternal.WithMessage("failed to fetch state breakdown")
	}

	return map[string]any{
		"countries": countries,
		"states":    states,
	}, nil
}

func (s *repositoryService) Sponsorship(ctx context.Context, filter dto.InsightsFilter) (_ any, err error) {
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
		return nil, errors.ErrInternal.WithMessage("failed to fetch sponsorship trend")
	}
	return results, nil
}

func (s *repositoryService) Velocity(ctx context.Context) (_ any, err error) {
	ctx, span := tracing.Start(ctx, "insights.repository.Velocity")
	defer tracing.End(span, &err)
	var results []dto.CompanyVelocity
	err = s.db.WithContext(ctx).Raw(`
		WITH v AS (
			SELECT
				c.id   AS company_id,
				c.name AS company_name,
				COUNT(*) FILTER (
					WHERE s.snapshotted_at >= DATE_TRUNC('week', CURRENT_DATE)
				) AS week_current,
				COUNT(*) FILTER (
					WHERE s.snapshotted_at >= DATE_TRUNC('week', CURRENT_DATE) - INTERVAL '7 days'
					  AND s.snapshotted_at <  DATE_TRUNC('week', CURRENT_DATE)
				) AS week_1,
				COUNT(*) FILTER (
					WHERE s.snapshotted_at >= DATE_TRUNC('week', CURRENT_DATE) - INTERVAL '14 days'
					  AND s.snapshotted_at <  DATE_TRUNC('week', CURRENT_DATE) - INTERVAL '7 days'
				) AS week_2,
				COUNT(*) FILTER (
					WHERE s.snapshotted_at >= DATE_TRUNC('week', CURRENT_DATE) - INTERVAL '21 days'
					  AND s.snapshotted_at <  DATE_TRUNC('week', CURRENT_DATE) - INTERVAL '14 days'
				) AS week_3
			FROM job_analytics s
			JOIN companies c ON s.company_id = c.id
			WHERE s.snapshotted_at >= DATE_TRUNC('week', CURRENT_DATE) - INTERVAL '28 days'
			GROUP BY c.id, c.name
		)
		SELECT * FROM v
		WHERE week_current > 0 OR week_1 > 0
		ORDER BY week_current DESC, (week_1 + week_2 + week_3) DESC
		LIMIT ?
	`, 20).Scan(&results).Error
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch company velocity")
	}
	return results, nil
}

func (s *repositoryService) RoleCategories(ctx context.Context) (_ any, err error) {
	ctx, span := tracing.Start(ctx, "insights.repository.RoleCategories")
	defer tracing.End(span, &err)
	var results []dto.RoleCategoryCount
	err = s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(role_category, 'other') AS category, COUNT(*) AS count
		FROM job_analytics
		GROUP BY role_category
		ORDER BY count DESC
		LIMIT 25
	`).Scan(&results).Error
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch role categories")
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
		return dto.PipelineCounts{}, errors.ErrInternal.WithMessage("failed to fetch pipeline counts")
	}
	return dto.PipelineCounts{Pending: result.Pending, Errors: result.Errors}, nil
}
