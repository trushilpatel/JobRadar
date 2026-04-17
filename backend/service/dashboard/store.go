package dashboard

import (
	"context"

	"gorm.io/gorm"

	"github.com/trushilpatel/jobradar/infra/tracing"
)

// ProgressStats holds aggregated user progress stats for the My Progress panel.
type ProgressStats struct {
	AppliedCount         int64 `json:"applied_count"`
	AwaitingReply        int64 `json:"awaiting_reply"`
	InterviewingCount    int64 `json:"interviewing_count"`
	InterviewingThisWeek int64 `json:"interviewing_this_week"`
	OfferCount           int64 `json:"offer_count"`
}

// MarketStats holds aggregated market stats for the Market panel.
type MarketStats struct {
	NewSinceYesterday int64 `json:"new_since_yesterday"`
	TotalOpen         int64 `json:"total_open"`
	AddedThisWeek     int64 `json:"added_this_week"`
	AddedLastWeek     int64 `json:"added_last_week"`
	WeeklyDelta       int64 `json:"weekly_delta"`
}

// WeeklyCount holds job count for a single calendar week in the heatmap.
type WeeklyCount struct {
	WeekStart string `json:"week_start"` // "YYYY-MM-DD"
	JobCount  int    `json:"job_count"`
}

// CompanyVelocityRow holds per-company 4-week velocity data.
type CompanyVelocityRow struct {
	CompanyID   string  `json:"company_id"`
	CompanyName string  `json:"company_name"`
	WeekCurrent int     `json:"week_current"`
	Week1       int     `json:"week_1"` // 1 week ago
	Week2       int     `json:"week_2"` // 2 weeks ago
	Week3       int     `json:"week_3"` // 3 weeks ago
	Multiplier  float64 `json:"multiplier"`
	VelocityTag string  `json:"velocity_tag"` // "spike" | "rising" | "normal"
}

// techAdoptionRow is the raw query result for tech tag quarterly job counts, before Go classification.
type techAdoptionRow struct {
	Tech      string
	Q1        int
	Q2        int
	Q3        int
	Q4        int
	TotalJobs int
}

// UserDashboardStore provides all DB queries for the user dashboard panels.
type UserDashboardStore interface {
	GetProgressStats(ctx context.Context) (ProgressStats, error)
	GetMarketStats(ctx context.Context) (MarketStats, error)
	GetWeeklyHiringActivity(ctx context.Context, weeks int) ([]WeeklyCount, error)
	GetCompanyVelocityDetailed(ctx context.Context, limit int) ([]CompanyVelocityRow, error)
	GetTechStackQuarterlyAdoption(ctx context.Context) ([]techAdoptionRow, error)
}

type userDashboardStore struct {
	db *gorm.DB
}

// NewUserDashboardStore constructs a UserDashboardStore implementation.
func NewUserDashboardStore(db *gorm.DB) UserDashboardStore {
	return &userDashboardStore{db: db}
}

func (s *userDashboardStore) GetProgressStats(ctx context.Context) (_ ProgressStats, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.store.GetProgressStats")
	defer tracing.End(span, &err)
	var result ProgressStats
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

func (s *userDashboardStore) GetMarketStats(ctx context.Context) (_ MarketStats, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.store.GetMarketStats")
	defer tracing.End(span, &err)
	var result MarketStats
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

func (s *userDashboardStore) GetWeeklyHiringActivity(ctx context.Context, weeks int) (results []WeeklyCount, err error) {
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

func (s *userDashboardStore) GetCompanyVelocityDetailed(ctx context.Context, limit int) (results []CompanyVelocityRow, err error) {
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

func (s *userDashboardStore) GetTechStackQuarterlyAdoption(ctx context.Context) (results []techAdoptionRow, err error) {
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

// PipelineDBStats holds the DB-sourced counts for the pipeline strip.
type PipelineDBStats struct {
	TotalCompanies  int64
	ActiveCompanies int64
	Queued          int64
	Crawling        int64
	NewJobs24h      int64
	Errors24h       int64
	// PipelinePending: jobs crawled (reason='fetch') but not yet written to job_feed.
	// Used as the DB-based proxy for awaiting_filter when Kafka lag is unavailable.
	PipelinePending int64
}

// PipelineStore provides DB queries for the pipeline strip.
type PipelineStore interface {
	GetPipelineDBStats(ctx context.Context) (PipelineDBStats, error)
}

type pipelineStore struct {
	db *gorm.DB
}

// NewPipelineStore constructs a PipelineStore.
func NewPipelineStore(db *gorm.DB) PipelineStore {
	return &pipelineStore{db: db}
}

func (s *pipelineStore) GetPipelineDBStats(ctx context.Context) (_ PipelineDBStats, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.store.GetPipelineDBStats")
	defer tracing.End(span, &err)
	var result PipelineDBStats
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
