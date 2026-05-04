-- +goose Up
-- Recreate mv_weekly_hiring_activity from job_feed (new unique jobs) instead of
-- job_pipeline_ledger (all pipeline entries including errors and retries).
DROP MATERIALIZED VIEW IF EXISTS mv_weekly_hiring_activity CASCADE;

CREATE MATERIALIZED VIEW mv_weekly_hiring_activity AS
SELECT
    DATE_TRUNC('week', added_at)::date::text AS week_start,
    COUNT(*)                                  AS job_count
FROM job_feed
GROUP BY DATE_TRUNC('week', added_at)
WITH DATA;

CREATE UNIQUE INDEX idx_mv_weekly_hiring_week ON mv_weekly_hiring_activity(week_start);

-- +goose Down
DROP MATERIALIZED VIEW IF EXISTS mv_weekly_hiring_activity CASCADE;

CREATE MATERIALIZED VIEW mv_weekly_hiring_activity AS
SELECT
    DATE_TRUNC('week', first_seen_at)::date::text AS week_start,
    COUNT(*)                                       AS job_count
FROM job_pipeline_ledger
GROUP BY DATE_TRUNC('week', first_seen_at)
WITH DATA;

CREATE UNIQUE INDEX idx_mv_weekly_hiring_week ON mv_weekly_hiring_activity(week_start);
