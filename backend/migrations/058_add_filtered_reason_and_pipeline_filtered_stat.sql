-- +goose Up
-- Widen the job_pipeline_ledger.reason CHECK to include 'filtered', written by
-- the filter worker when a job is published to jobs.filtered.* (passed the filter,
-- awaiting enrichment). This separates "awaiting filter" from "awaiting enrichment"
-- so pipeline_pending and pipeline_filtered each track one stage accurately.

ALTER TABLE job_pipeline_ledger
    DROP CONSTRAINT IF EXISTS job_pipeline_ledger_reason_check;

ALTER TABLE job_pipeline_ledger
    ADD CONSTRAINT job_pipeline_ledger_reason_check
    CHECK (reason IN ('fetch','filtered','llm_irrelevant','user_dismissed','error','retry','non_usa_location'))
    NOT VALID;

ALTER TABLE job_pipeline_ledger
    VALIDATE CONSTRAINT job_pipeline_ledger_reason_check;

-- Recreate mv_pipeline_db_stats to add pipeline_filtered:
--   pipeline_pending  = reason='fetch'    not yet in jobs (truly awaiting filter)
--   pipeline_filtered = reason='filtered' not yet in jobs (passed filter, awaiting enricher)
DROP MATERIALIZED VIEW IF EXISTS mv_pipeline_db_stats;

CREATE MATERIALIZED VIEW mv_pipeline_db_stats AS
SELECT
    1                           AS id,
    c.total_companies,
    c.active_companies,
    q.queued,
    q.crawling,
    j.new_jobs_24h,
    l.errors_24h,
    p.pipeline_pending,
    f.pipeline_filtered
FROM
    (SELECT
        COUNT(*)                                                   AS total_companies,
        COALESCE(
            SUM(CASE WHEN css.active THEN 1 ELSE 0 END), 0
        )::BIGINT                                                  AS active_companies
     FROM companies
     LEFT JOIN company_scan_settings css ON css.company_id = companies.id) c,
    (SELECT
        COUNT(*) FILTER (WHERE status = 'queued')  AS queued,
        COUNT(*) FILTER (WHERE status = 'running') AS crawling
     FROM company_scan_queue) q,
    (SELECT COUNT(*) AS new_jobs_24h
     FROM jobs
     WHERE added_at >= NOW() - INTERVAL '24 hours') j,
    (SELECT
        COUNT(*) FILTER (WHERE reason = 'error'
                           AND first_seen_at >= NOW() - INTERVAL '24 hours') AS errors_24h
     FROM job_pipeline_ledger) l,
    (SELECT COUNT(*) AS pipeline_pending
     FROM job_pipeline_ledger lp
     LEFT JOIN jobs jt ON jt.job_post_id = lp.job_id
     WHERE lp.reason = 'fetch'
       AND jt.job_post_id IS NULL) p,
    (SELECT COUNT(*) AS pipeline_filtered
     FROM job_pipeline_ledger lf
     LEFT JOIN jobs jt ON jt.job_post_id = lf.job_id
     WHERE lf.reason = 'filtered'
       AND jt.job_post_id IS NULL) f
WITH DATA;

CREATE UNIQUE INDEX idx_mv_pipeline_db_stats ON mv_pipeline_db_stats(id);

-- +goose Down
-- Collapse filtered rows back to fetch so the tighter CHECK can be reinstated.
UPDATE job_pipeline_ledger
SET reason = 'fetch'
WHERE reason = 'filtered';

ALTER TABLE job_pipeline_ledger
    DROP CONSTRAINT job_pipeline_ledger_reason_check;

ALTER TABLE job_pipeline_ledger
    ADD CONSTRAINT job_pipeline_ledger_reason_check
    CHECK (reason IN ('fetch','llm_irrelevant','user_dismissed','error','retry','non_usa_location'))
    NOT VALID;

ALTER TABLE job_pipeline_ledger
    VALIDATE CONSTRAINT job_pipeline_ledger_reason_check;

DROP MATERIALIZED VIEW IF EXISTS mv_pipeline_db_stats;

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
        COUNT(*)                                                   AS total_companies,
        COALESCE(
            SUM(CASE WHEN css.active THEN 1 ELSE 0 END), 0
        )::BIGINT                                                  AS active_companies
     FROM companies
     LEFT JOIN company_scan_settings css ON css.company_id = companies.id) c,
    (SELECT
        COUNT(*) FILTER (WHERE status = 'queued')  AS queued,
        COUNT(*) FILTER (WHERE status = 'running') AS crawling
     FROM company_scan_queue) q,
    (SELECT COUNT(*) AS new_jobs_24h
     FROM jobs
     WHERE added_at >= NOW() - INTERVAL '24 hours') j,
    (SELECT
        COUNT(*) FILTER (WHERE reason = 'error'
                           AND first_seen_at >= NOW() - INTERVAL '24 hours') AS errors_24h
     FROM job_pipeline_ledger) l,
    (SELECT COUNT(*) AS pipeline_pending
     FROM job_pipeline_ledger lp
     LEFT JOIN jobs jt ON jt.job_post_id = lp.job_id
     WHERE lp.reason = 'fetch'
       AND jt.job_post_id IS NULL) p
WITH DATA;

CREATE UNIQUE INDEX idx_mv_pipeline_db_stats ON mv_pipeline_db_stats(id);
