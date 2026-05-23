-- Migration 068: dashboard security — composite index and BRIN verification
-- Required by GetProgressStats after user_id scoping was added.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_jat_user_stage_at
    ON job_application_tracker (user_id, stage_at DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_jobs_added_at_brin
    ON jobs USING BRIN (added_at);
