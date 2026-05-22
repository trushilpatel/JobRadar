-- +goose Up
-- Part 1: pg_trgm GIN indexes for ILIKE search performance.
-- Enables fast substring / pattern matching on title, location, and company name.
-- NOTE: CREATE INDEX without CONCURRENTLY — goose runs each migration inside a
-- transaction, and CONCURRENTLY is not permitted inside a transaction block.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_jobs_title_trgm
    ON jobs USING GIN (title gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_jobs_location_trgm
    ON jobs USING GIN (location gin_trgm_ops)
    WHERE location IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_companies_name_trgm
    ON companies USING GIN (name gin_trgm_ops);

-- Part 2: Drop duplicate / redundant indexes on the jobs table.
-- idx_job_company: originally created in 004 on table `job`; migration 048 notes
--   it was "never recreated" after the job→jobs rename, but we guard with IF EXISTS.
-- idx_job_added: originally created in 004 on `job(added_at DESC)`; survived the
--   rename to `jobs`; now a leading-prefix subset of idx_jobs_added_at_id (added in
--   048) and never preferred by the planner.

DROP INDEX IF EXISTS idx_job_company;
DROP INDEX IF EXISTS idx_job_added;

-- Part 3: Partial indexes for additional user_job_state status tabs.
-- Mirrors the existing idx_user_job_state_saved / idx_user_job_state_applied pattern
-- from migration 044 for the two remaining high-row-count statuses.

CREATE INDEX IF NOT EXISTS idx_user_job_state_dismissed
    ON user_job_state (user_id, dismissed_at DESC)
    WHERE status = 'dismissed';

CREATE INDEX IF NOT EXISTS idx_user_job_state_interviewing
    ON user_job_state (user_id, updated_at DESC)
    WHERE status = 'interviewing';

-- +goose Down
DROP INDEX IF EXISTS idx_user_job_state_interviewing;
DROP INDEX IF EXISTS idx_user_job_state_dismissed;
-- Redundant indexes are not restored on Down — they were superseded before removal.
DROP INDEX IF EXISTS idx_companies_name_trgm;
DROP INDEX IF EXISTS idx_jobs_location_trgm;
DROP INDEX IF EXISTS idx_jobs_title_trgm;
-- Extension is not dropped — other objects may depend on pg_trgm.
