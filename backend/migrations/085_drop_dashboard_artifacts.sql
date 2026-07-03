-- +goose Up
-- Drop dashboard materialized views (replaced by ad-hoc analytics queries on job_analytics).
-- The underlying tables (job_analytics, job_application_tracker, etc.) are kept intact.
DROP MATERIALIZED VIEW IF EXISTS mv_weekly_hiring_activity;
DROP MATERIALIZED VIEW IF EXISTS mv_pipeline_db_stats;

-- Drop dashboard-only indexes that served the materialized view refresh queries.
DROP INDEX IF EXISTS idx_jat_user_stage_at;
DROP INDEX IF EXISTS idx_jobs_added_at_brin;

-- +goose Down
-- Recreate indexes first (materialized views depend on them for refresh performance).
CREATE INDEX IF NOT EXISTS idx_jat_user_stage_at ON job_application_tracker (user_id, stage_updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_jobs_added_at_brin ON jobs USING brin (added_at);

-- Recreate materialized views (empty — data will be populated on next REFRESH).
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_weekly_hiring_activity AS
SELECT
    date_trunc('week', ja.created_at) AS week,
    c.name                            AS company,
    COUNT(*)                          AS job_count
FROM job_analytics ja
JOIN jobs j ON j.id = ja.job_id
JOIN companies c ON c.id = j.company_id
GROUP BY 1, 2;

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_pipeline_db_stats AS
SELECT
    (SELECT COUNT(*) FROM jobs)                                              AS total_jobs,
    (SELECT COUNT(*) FROM job_pipeline_ledger WHERE status = 'filtered')    AS filtered_jobs,
    (SELECT COUNT(*) FROM job_pipeline_ledger WHERE status = 'error')       AS error_jobs,
    (SELECT COUNT(*) FROM companies WHERE status = 'active')                AS active_companies;
