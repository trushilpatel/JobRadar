-- +goose Up

-- Drop stale indexes from migration 044 multi-user refactor.
-- Both reference columns (status, saved) that were dropped from jobs in 044.
DROP INDEX IF EXISTS idx_job_feed_status_added;
DROP INDEX IF EXISTS idx_job_feed_saved;

-- B-tree for keyset cursor on the global feed.
-- BRIN (idx_job_feed_added_brin) cannot do ordered point lookups required by:
-- WHERE (added_at, id) < (?, ?) ORDER BY added_at DESC, id DESC
CREATE INDEX IF NOT EXISTS idx_jobs_added_at_id
    ON jobs (added_at DESC, id DESC);

-- Company filter + GetDistinctCompanyNames.
-- idx_job_company from migration 004 was on the pre-rename table; never recreated.
CREATE INDEX IF NOT EXISTS idx_jobs_company_id
    ON jobs (company_id);

-- Status-tab query path: drives from user_job_state filtered by (user_id, status).
CREATE INDEX IF NOT EXISTS idx_user_job_state_user_status
    ON user_job_state (user_id, status);

-- +goose Down
DROP INDEX IF EXISTS idx_user_job_state_user_status;
DROP INDEX IF EXISTS idx_jobs_company_id;
DROP INDEX IF EXISTS idx_jobs_added_at_id;
-- Note: stale indexes are not restored on Down — they were invalid before removal.
