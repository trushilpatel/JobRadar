package services

import (
	"context"

	"gorm.io/gorm"

	"github.com/trushilpatel/jobradar/common/tracing"
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
	// Phase 2: status moved from jobs to user_job_state (per-user).
	// applied/saved/dismissed counts come from user_job_state; interviewing/offer
	// stages are tracked in job_application_tracker. awaiting_reply = applied
	// jobs with no advanced tracker stage.
	err = s.db.WithContext(ctx).Raw(`
		WITH status_counts AS (
			SELECT
				COUNT(*) FILTER (WHERE status = 'applied')  AS applied_count,
				COUNT(*)                                     AS interviewing_count,
				0                                            AS offer_count
			FROM user_job_state
			WHERE status = 'applied'
		),
		tracker_stats AS (
			SELECT
				COUNT(DISTINCT ja.job_result_id) FILTER (
					WHERE ja.stage IN ('phone_screen', 'technical', 'final_round')
					  AND ja.stage_at >= DATE_TRUNC('week', NOW())
				) AS interviewing_this_week,
				COUNT(DISTINCT ja.job_result_id) FILTER (
					WHERE advanced.job_result_id IS NULL
				) AS awaiting_reply
			FROM job_application_tracker ja
			LEFT JOIN (
				SELECT DISTINCT job_result_id
				FROM job_application_tracker
				WHERE stage != 'applied'
			) advanced ON advanced.job_result_id = ja.job_result_id
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
	// Phase 2: status removed from jobs table (moved to user_job_state).
	// total_open = all jobs not dismissed by this user (global count without
	// per-user filter at the market stats level — these are market signals).
	err = s.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(*) FILTER (
				WHERE added_at >= CURRENT_DATE - INTERVAL '1 day'
			) AS new_since_yesterday,
			COUNT(*)  AS total_open,
			COUNT(*) FILTER (
				WHERE added_at >= DATE_TRUNC('week', NOW())
			) AS added_this_week,
			COUNT(*) FILTER (
				WHERE added_at >= DATE_TRUNC('week', NOW()) - INTERVAL '7 days'
				  AND added_at <  DATE_TRUNC('week', NOW())
			) AS added_last_week
		FROM jobs
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
		SELECT week_start, job_count
		FROM mv_weekly_hiring_activity
		WHERE week_start >= (NOW() - (? * INTERVAL '1 week'))::date::text
		ORDER BY week_start ASC
	`, weeks).Scan(&results).Error
	return results, err
}

func (s *userDashboardRepository) GetCompanyVelocityDetailed(ctx context.Context, limit int) (results []dto.CompanyVelocityRow, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.store.GetCompanyVelocityDetailed")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Raw(`
		WITH v AS (
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
		)
		SELECT * FROM v
		ORDER BY week_current DESC, (week_1 + week_2 + week_3) DESC
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
		SELECT total_companies, active_companies, queued, crawling, new_jobs_24h, errors_24h, pipeline_pending, pipeline_filtered
		FROM mv_pipeline_db_stats
	`).Scan(&result).Error
	return result, err
}

// StatsRefresher refreshes the dashboard materialized views on a schedule.
type StatsRefresher interface {
	RefreshPipelineStats(ctx context.Context) error
	RefreshHeatmap(ctx context.Context) error
}

type dashboardRefresher struct{ db *gorm.DB }

// NewDashboardRefresher constructs a StatsRefresher backed by the given DB connection.
func NewDashboardRefresher(db *gorm.DB) StatsRefresher {
	return &dashboardRefresher{db: db}
}

func (r *dashboardRefresher) RefreshPipelineStats(ctx context.Context) error {
	return r.db.WithContext(ctx).Exec(`REFRESH MATERIALIZED VIEW CONCURRENTLY mv_pipeline_db_stats`).Error
}

func (r *dashboardRefresher) RefreshHeatmap(ctx context.Context) error {
	return r.db.WithContext(ctx).Exec(`REFRESH MATERIALIZED VIEW CONCURRENTLY mv_weekly_hiring_activity`).Error
}
