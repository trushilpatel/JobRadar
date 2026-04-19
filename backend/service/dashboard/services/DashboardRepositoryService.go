package services

import (
	"context"

	"gorm.io/gorm"

	"github.com/trushilpatel/jobradar/platform/tracing"
	"github.com/trushilpatel/jobradar/service/dashboard/dto"
)

// UserDashboardStore provides all DB queries for the user dashboard panels.
type UserDashboardStore interface {
	GetProgressStats(ctx context.Context) (dto.ProgressStats, error)
	GetMarketStats(ctx context.Context) (dto.MarketStats, error)
	GetWeeklyHiringActivity(ctx context.Context, weeks int) ([]dto.WeeklyCount, error)
	GetCompanyVelocityDetailed(ctx context.Context, limit int) ([]dto.CompanyVelocityRow, error)
	GetTechStackQuarterlyAdoption(ctx context.Context) ([]dto.TechAdoptionRow, error)
}

type userDashboardRepository struct {
	db *gorm.DB
}

// NewUserDashboardStore constructs a UserDashboardStore implementation.
func NewUserDashboardStore(db *gorm.DB) UserDashboardStore {
	return &userDashboardRepository{db: db}
}

func (s *userDashboardRepository) GetProgressStats(ctx context.Context) (_ dto.ProgressStats, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.store.GetProgressStats")
	defer tracing.End(span, &err)
	var result dto.ProgressStats
	err = s.db.WithContext(ctx).Raw(`
		WITH status_counts AS (
			SELECT
				COUNT(*) FILTER (WHERE status = 'applied')      AS applied_count,
				COUNT(*) FILTER (WHERE status = 'interviewing') AS interviewing_count,
				COUNT(*) FILTER (WHERE status = 'offer')        AS offer_count
			FROM job_feed
		),
		tracker_stats AS (
			SELECT
				COUNT(DISTINCT ja.job_result_id) FILTER (
					WHERE ja.stage IN ('phone_screen', 'technical', 'final_round')
					  AND ja.stage_at >= DATE_TRUNC('week', NOW())
				) AS interviewing_this_week,
				COUNT(DISTINCT j.id) FILTER (
					WHERE j.status = 'applied'
					  AND NOT EXISTS (
						  SELECT 1 FROM job_application_tracker ja2
						  WHERE ja2.job_result_id = j.id
							AND ja2.stage != 'applied'
						  )
				) AS awaiting_reply
			FROM job_application_tracker ja
			JOIN job_feed j ON ja.job_result_id = j.id
		)
		SELECT
			sc.applied_count,
			sc.interviewing_count,
			sc.offer_count,
			ts.awaiting_reply,
			ts.interviewing_this_week
		FROM status_counts sc, tracker_stats ts
	`).Scan(&result).Error
	return result, err
}

func (s *userDashboardRepository) GetMarketStats(ctx context.Context) (_ dto.MarketStats, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.store.GetMarketStats")
	defer tracing.End(span, &err)
	var result dto.MarketStats
	err = s.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(*) FILTER (
				WHERE added_at >= CURRENT_DATE - INTERVAL '1 day'
			) AS new_since_yesterday,
			COUNT(*) FILTER (
				WHERE status IN ('new', 'saved', 'applied', 'interviewing', 'offer')
			) AS total_open,
			COUNT(*) FILTER (
				WHERE added_at >= DATE_TRUNC('week', NOW())
			) AS added_this_week,
			COUNT(*) FILTER (
				WHERE added_at >= DATE_TRUNC('week', NOW()) - INTERVAL '7 days'
				  AND added_at <  DATE_TRUNC('week', NOW())
			) AS added_last_week
		FROM job_feed
	`).Scan(&result).Error
	if err != nil {
		return result, err
	}
	result.WeeklyDelta = result.AddedThisWeek - result.AddedLastWeek
	return result, nil
}

func (s *userDashboardRepository) GetWeeklyHiringActivity(ctx context.Context, weeks int) (results []dto.WeeklyCount, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.store.GetWeeklyHiringActivity")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Raw(`
		SELECT
			DATE_TRUNC('week', first_seen_at)::date::text AS week_start,
			COUNT(*)                                       AS job_count
		FROM job_pipeline_ledger
		WHERE first_seen_at >= NOW() - (? * INTERVAL '1 week')
		GROUP BY DATE_TRUNC('week', first_seen_at)
		ORDER BY week_start ASC
	`, weeks).Scan(&results).Error
	return results, err
}

func (s *userDashboardRepository) GetCompanyVelocityDetailed(ctx context.Context, limit int) (results []dto.CompanyVelocityRow, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.store.GetCompanyVelocityDetailed")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Raw(`
		SELECT
			c.id   AS company_id,
			c.name AS company_name,
			COUNT(*) FILTER (
				WHERE ja.snapshotted_at >= DATE_TRUNC('week', NOW())
			) AS week_current,
			COUNT(*) FILTER (
				WHERE ja.snapshotted_at >= DATE_TRUNC('week', NOW()) - INTERVAL '7 days'
				  AND ja.snapshotted_at <  DATE_TRUNC('week', NOW())
			) AS week_1,
			COUNT(*) FILTER (
				WHERE ja.snapshotted_at >= DATE_TRUNC('week', NOW()) - INTERVAL '14 days'
				  AND ja.snapshotted_at <  DATE_TRUNC('week', NOW()) - INTERVAL '7 days'
			) AS week_2,
			COUNT(*) FILTER (
				WHERE ja.snapshotted_at >= DATE_TRUNC('week', NOW()) - INTERVAL '21 days'
				  AND ja.snapshotted_at <  DATE_TRUNC('week', NOW()) - INTERVAL '14 days'
			) AS week_3
		FROM job_analytics ja
		JOIN companies c ON ja.company_id = c.id
		WHERE ja.snapshotted_at >= DATE_TRUNC('week', NOW()) - INTERVAL '28 days'
		GROUP BY c.id, c.name
		HAVING COUNT(*) FILTER (
			WHERE ja.snapshotted_at >= DATE_TRUNC('week', NOW())
		) > 0
		ORDER BY week_current DESC
		LIMIT ?
	`, limit).Scan(&results).Error
	return results, err
}

func (s *userDashboardRepository) GetTechStackQuarterlyAdoption(ctx context.Context) (results []dto.TechAdoptionRow, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.store.GetTechStackQuarterlyAdoption")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Raw(`
		WITH quarterly_counts AS (
			SELECT
				tech,
				COUNT(*) FILTER (
					WHERE tagged_at >= DATE_TRUNC('quarter', NOW() - INTERVAL '9 months')
					  AND tagged_at <  DATE_TRUNC('quarter', NOW() - INTERVAL '6 months')
				) AS q1,
				COUNT(*) FILTER (
					WHERE tagged_at >= DATE_TRUNC('quarter', NOW() - INTERVAL '6 months')
					  AND tagged_at <  DATE_TRUNC('quarter', NOW() - INTERVAL '3 months')
				) AS q2,
				COUNT(*) FILTER (
					WHERE tagged_at >= DATE_TRUNC('quarter', NOW() - INTERVAL '3 months')
					  AND tagged_at <  DATE_TRUNC('quarter', NOW())
				) AS q3,
				COUNT(*) FILTER (
					WHERE tagged_at >= DATE_TRUNC('quarter', NOW())
				) AS q4
			FROM job_tech_tags
			WHERE tagged_at >= DATE_TRUNC('quarter', NOW() - INTERVAL '9 months')
			GROUP BY tech
		),
		total_q4 AS (
			SELECT COUNT(DISTINCT job_id) AS total
			FROM job_tech_tags
			WHERE tagged_at >= DATE_TRUNC('quarter', NOW())
		)
		SELECT
			qc.tech,
			qc.q1, qc.q2, qc.q3, qc.q4,
			COALESCE(t.total, 0) AS total_jobs
		FROM quarterly_counts qc
		CROSS JOIN total_q4 t
		WHERE qc.q4 > 0
		ORDER BY qc.q4 DESC
	`).Scan(&results).Error
	return results, err
}

// PipelineStore provides DB queries for the pipeline strip.
type PipelineStore interface {
	GetPipelineDBStats(ctx context.Context) (dto.PipelineDBStats, error)
}

type pipelineRepository struct {
	db *gorm.DB
}

// NewPipelineStore constructs a PipelineStore.
func NewPipelineStore(db *gorm.DB) PipelineStore {
	return &pipelineRepository{db: db}
}

func (s *pipelineRepository) GetPipelineDBStats(ctx context.Context) (_ dto.PipelineDBStats, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.store.GetPipelineDBStats")
	defer tracing.End(span, &err)
	var result dto.PipelineDBStats
	err = s.db.WithContext(ctx).Raw(`
		SELECT
			(SELECT COUNT(*) FROM companies)                                                                              AS total_companies,
			(SELECT COUNT(*) FROM companies WHERE active = true)                                                          AS active_companies,
			(SELECT COUNT(*) FROM company_scan_queue WHERE status = 'queued')                                             AS queued,
			(SELECT COUNT(*) FROM company_scan_queue WHERE status = 'processing')                                         AS crawling,
			(SELECT COUNT(*) FROM job_feed WHERE added_at >= NOW() - INTERVAL '24 hours' AND status != 'dismissed')      AS new_jobs_24h,
			(SELECT COUNT(*) FROM job_pipeline_ledger WHERE reason = 'error' AND first_seen_at >= NOW() - INTERVAL '24 hours') AS errors_24h,
			(SELECT COUNT(*) FROM job_pipeline_ledger WHERE reason = 'fetch' AND job_id NOT IN (SELECT job_post_id FROM job_feed)) AS pipeline_pending
	`).Scan(&result).Error
	return result, err
}
