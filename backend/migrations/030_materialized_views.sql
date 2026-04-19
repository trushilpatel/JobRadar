-- +goose Up

-- Heatmap: pre-aggregate job_pipeline_ledger by week — refreshed every 5 min by scheduler
CREATE MATERIALIZED VIEW mv_weekly_hiring_activity AS
SELECT
    DATE_TRUNC('week', first_seen_at)::date::text AS week_start,
    COUNT(*)                                       AS job_count
FROM job_pipeline_ledger
GROUP BY DATE_TRUNC('week', first_seen_at)
WITH DATA;

-- Unique index required for REFRESH CONCURRENTLY
CREATE UNIQUE INDEX idx_mv_weekly_hiring_week ON mv_weekly_hiring_activity(week_start);

-- Pipeline stats: pre-compute all dashboard pipeline counts — refreshed every 3 min by scheduler
-- Uses LEFT JOIN instead of NOT IN for pipeline_pending (correct NULL handling + index use)
CREATE MATERIALIZED VIEW mv_pipeline_db_stats AS
SELECT
    1                           AS id,
    c.total_companies,
    c.active_companies,
    q.queued,
    q.crawling,
    j.new_jobs_24h,
    l.errors_24h,
    p.pipeline_pending
FROM
    (SELECT
        COUNT(*)                       AS total_companies,
        COUNT(*) FILTER (WHERE active) AS active_companies
     FROM companies) c,
    (SELECT
        COUNT(*) FILTER (WHERE status = 'queued')     AS queued,
        COUNT(*) FILTER (WHERE status = 'processing') AS crawling
     FROM company_scan_queue) q,
    (SELECT COUNT(*) AS new_jobs_24h
     FROM job_feed
     WHERE added_at >= NOW() - INTERVAL '24 hours'
       AND status != 'dismissed') j,
    (SELECT
        COUNT(*) FILTER (WHERE reason = 'error'
                           AND first_seen_at >= NOW() - INTERVAL '24 hours') AS errors_24h
     FROM job_pipeline_ledger) l,
    (SELECT COUNT(*) AS pipeline_pending
     FROM job_pipeline_ledger lp
     LEFT JOIN job_feed jf ON jf.job_post_id = lp.job_id
     WHERE lp.reason = 'fetch'
       AND jf.job_post_id IS NULL) p
WITH DATA;

-- Unique index on the singleton row — required for REFRESH CONCURRENTLY
CREATE UNIQUE INDEX idx_mv_pipeline_db_stats ON mv_pipeline_db_stats(id);

-- +goose Down
DROP MATERIALIZED VIEW mv_weekly_hiring_activity;
DROP MATERIALIZED VIEW mv_pipeline_db_stats;
