-- +goose Up
-- Composite index for GetProgressStats and GetInterviews after user_id scoping.
-- NOTE: No CONCURRENTLY — goose runs inside a transaction block where CONCURRENTLY
-- is forbidden. IF NOT EXISTS guards against re-run.

CREATE INDEX IF NOT EXISTS idx_jat_user_stage_at
    ON job_application_tracker (user_id, stage_at DESC);

CREATE INDEX IF NOT EXISTS idx_jobs_added_at_brin
    ON jobs USING BRIN (added_at);

-- +goose Down
DROP INDEX IF EXISTS idx_jobs_added_at_brin;
DROP INDEX IF EXISTS idx_jat_user_stage_at;
