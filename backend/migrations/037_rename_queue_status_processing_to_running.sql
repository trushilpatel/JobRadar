-- +goose Up

-- Rename queue status 'processing' → 'running' for domain consistency.
ALTER TABLE company_scan_queue DROP CONSTRAINT company_scan_queue_status_check;
UPDATE company_scan_queue SET status = 'running' WHERE status = 'processing';
ALTER TABLE company_scan_queue
    ADD CONSTRAINT company_scan_queue_status_check
    CHECK (status IN ('queued', 'running'));

-- Recreate the pipeline stats materialized view to reflect the renamed status.
DROP MATERIALIZED VIEW mv_pipeline_db_stats;
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
        COUNT(*) FILTER (WHERE status = 'queued')  AS queued,
        COUNT(*) FILTER (WHERE status = 'running') AS crawling
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

CREATE UNIQUE INDEX idx_mv_pipeline_db_stats ON mv_pipeline_db_stats(id);

-- +goose Down

ALTER TABLE company_scan_queue DROP CONSTRAINT company_scan_queue_status_check;
UPDATE company_scan_queue SET status = 'processing' WHERE status = 'running';
ALTER TABLE company_scan_queue
    ADD CONSTRAINT company_scan_queue_status_check
    CHECK (status IN ('queued', 'processing'));

DROP MATERIALIZED VIEW mv_pipeline_db_stats;
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

CREATE UNIQUE INDEX idx_mv_pipeline_db_stats ON mv_pipeline_db_stats(id);
