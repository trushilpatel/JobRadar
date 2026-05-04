-- +goose Up
-- Phase 2 — multi-user data model.
-- HARD-STOP CUTOVER: destructive. Drops per-user state from job_feed, splits operational
-- fields off `companies` into `company_scan_settings`, wipes job_application_tracker, and
-- drops `job_dismissal_log` (replaced by user_job_state.dismissal_reason / dismissal_note).
-- Existing companies have NO scan_settings rows after this migration — admins must re-onboard
-- via /admin/companies/{id}/scan-settings (Phase 3 UI). Workers skip companies with no settings.

-- +goose StatementBegin

-- ---------------------------------------------------------------------------
-- 1. Drop materialized views that reference soon-to-be-dropped columns.
--    Recreated in Phase 2 worker bucket / dashboard refactor — not here.
-- ---------------------------------------------------------------------------
DROP MATERIALIZED VIEW IF EXISTS mv_pipeline_db_stats;
DROP MATERIALIZED VIEW IF EXISTS mv_weekly_hiring_activity;

-- ---------------------------------------------------------------------------
-- 2. Rename job_feed -> jobs and drop per-user columns.
--    NOTE: phase2-contracts.md lists the per-user columns as
--    (status, saved_at, applied_at, dismissed_at, notes). The current schema has
--    `status` + `saved` (bool) + `notes` instead of timestamp variants. We drop
--    `saved` here as it is the equivalent per-user state column — see report.
-- ---------------------------------------------------------------------------
ALTER TABLE job_feed RENAME TO jobs;

ALTER TABLE jobs
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS saved,
    DROP COLUMN IF EXISTS notes;

-- ---------------------------------------------------------------------------
-- 3. user_job_state — per-user job interaction state.
--    Absence of row = "new". Status enum does NOT include "new".
-- ---------------------------------------------------------------------------
CREATE TABLE user_job_state (
    user_id          UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    job_id           TEXT         NOT NULL REFERENCES jobs(id)  ON DELETE CASCADE,
    status           TEXT         NOT NULL
                                  CHECK (status IN ('saved', 'applied', 'dismissed')),
    notes            TEXT,
    dismissal_reason TEXT,
    dismissal_note   TEXT,
    saved_at         TIMESTAMPTZ,
    applied_at       TIMESTAMPTZ,
    dismissed_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, job_id)
);

-- Reverse lookup (find users who saved/applied/dismissed a given job)
CREATE INDEX idx_user_job_state_job_id ON user_job_state(job_id);

-- Per-user feed queries: saved list ordered by saved_at DESC
CREATE INDEX idx_user_job_state_saved
    ON user_job_state(user_id, saved_at DESC)
    WHERE status = 'saved';

-- Per-user feed queries: applied list ordered by applied_at DESC
CREATE INDEX idx_user_job_state_applied
    ON user_job_state(user_id, applied_at DESC)
    WHERE status = 'applied';

-- ---------------------------------------------------------------------------
-- 4. company_scan_settings — admin-only operational scan config.
--    No backfill: existing companies start without rows here.
-- ---------------------------------------------------------------------------
CREATE TABLE company_scan_settings (
    company_id              TEXT         PRIMARY KEY REFERENCES companies(id) ON DELETE CASCADE,
    adapter_type            TEXT         NOT NULL,
    adapter_config          JSONB,
    scan_interval_override  INT
                                         CHECK (scan_interval_override IS NULL
                                                OR scan_interval_override > 0),
    active                  BOOLEAN      NOT NULL DEFAULT TRUE,
    last_scan_at            TIMESTAMPTZ,
    last_success_at         TIMESTAMPTZ,
    last_error_at           TIMESTAMPTZ,
    last_error_message      TEXT,
    consecutive_errors      INT          NOT NULL DEFAULT 0,
    created_at              TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- ---------------------------------------------------------------------------
-- 5. companies — drop operational columns, prepare public-only shape.
--    Keep: id, name, slug, website, careers_url, logo_url, created_at, updated_at.
--    Existing schema has only: id, name, url, adapter_type, active, created_at,
--    last_scan_at, last_scan_status, scan_interval_minutes_override.
--    Therefore we additionally rename `url` -> `website` and add slug, careers_url,
--    logo_url, updated_at to match the contract.
-- ---------------------------------------------------------------------------
ALTER TABLE companies
    DROP COLUMN IF EXISTS adapter_type,
    DROP COLUMN IF EXISTS active,
    DROP COLUMN IF EXISTS last_scan_at,
    DROP COLUMN IF EXISTS last_scan_status,
    DROP COLUMN IF EXISTS scan_interval_minutes_override;

ALTER TABLE companies RENAME COLUMN url TO website;

ALTER TABLE companies
    ADD COLUMN IF NOT EXISTS slug         TEXT,
    ADD COLUMN IF NOT EXISTS careers_url  TEXT,
    ADD COLUMN IF NOT EXISTS logo_url     TEXT,
    ADD COLUMN IF NOT EXISTS updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Backfill slug from id (slug-form ids today) and enforce uniqueness for Phase 2 lookups.
UPDATE companies SET slug = id WHERE slug IS NULL;
ALTER TABLE companies ALTER COLUMN slug SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_companies_slug ON companies(slug);

-- ---------------------------------------------------------------------------
-- 6. job_application_tracker — wipe and add user_id.
--    Per design: per-user data is dropped on cutover; users re-create applications.
-- ---------------------------------------------------------------------------
DELETE FROM job_application_tracker;

ALTER TABLE job_application_tracker
    ADD COLUMN user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE;

CREATE INDEX idx_job_application_tracker_user
    ON job_application_tracker(user_id);
CREATE INDEX idx_job_application_tracker_user_job
    ON job_application_tracker(user_id, job_result_id);

-- ---------------------------------------------------------------------------
-- 7. job_dismissal_log — dropped. Reason + note now live on user_job_state.
-- ---------------------------------------------------------------------------
DROP TABLE IF EXISTS job_dismissal_log;

-- +goose StatementEnd


-- +goose Down
-- Reverse Phase 2 to the pre-cutover (Phase 1) shape.
-- NOTE: per-user data wiped from job_application_tracker in Up cannot be restored.
-- NOTE: data in user_job_state and company_scan_settings is dropped on rollback.
-- NOTE: materialized views are recreated to their migration-042 shape.

-- +goose StatementBegin

-- ---------------------------------------------------------------------------
-- 7r. Recreate job_dismissal_log (matches migrations 006 + 010 cascade).
-- ---------------------------------------------------------------------------
CREATE TABLE job_dismissal_log (
    id            BIGSERIAL PRIMARY KEY,
    job_result_id TEXT NOT NULL
                       REFERENCES job_feed(id) ON DELETE CASCADE,
    reason        TEXT NOT NULL
                       CHECK (reason IN ('user_dismissed', 'user_not_relevant')),
    discarded_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    note          TEXT
);
CREATE INDEX idx_job_discarded ON job_dismissal_log(job_result_id);

-- ---------------------------------------------------------------------------
-- 6r. job_application_tracker — drop user_id and its indexes.
-- ---------------------------------------------------------------------------
DROP INDEX IF EXISTS idx_job_application_tracker_user_job;
DROP INDEX IF EXISTS idx_job_application_tracker_user;
ALTER TABLE job_application_tracker DROP COLUMN IF EXISTS user_id;

-- ---------------------------------------------------------------------------
-- 5r. companies — restore operational columns and revert public-only changes.
-- ---------------------------------------------------------------------------
DROP INDEX IF EXISTS uq_companies_slug;
ALTER TABLE companies
    DROP COLUMN IF EXISTS slug,
    DROP COLUMN IF EXISTS careers_url,
    DROP COLUMN IF EXISTS logo_url,
    DROP COLUMN IF EXISTS updated_at;

ALTER TABLE companies RENAME COLUMN website TO url;

ALTER TABLE companies
    ADD COLUMN adapter_type TEXT,
    ADD COLUMN active BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN last_scan_at TIMESTAMPTZ,
    ADD COLUMN last_scan_status TEXT,
    ADD COLUMN scan_interval_minutes_override INT;

ALTER TABLE companies
    ADD CONSTRAINT companies_adapter_type_check
        CHECK (adapter_type IN (
            'greenhouse', 'lever', 'ashby',
            'workday', 'workable', 'chromedp',
            'google'
        ));

ALTER TABLE companies
    ADD CONSTRAINT companies_last_scan_status_check
        CHECK (last_scan_status IN ('ok', 'error', 'pending'));

ALTER TABLE companies
    ADD CONSTRAINT companies_scan_interval_check
        CHECK (scan_interval_minutes_override IS NULL
               OR scan_interval_minutes_override > 0);

-- ---------------------------------------------------------------------------
-- 4r. Drop company_scan_settings.
-- ---------------------------------------------------------------------------
DROP TABLE IF EXISTS company_scan_settings;

-- ---------------------------------------------------------------------------
-- 3r. Drop user_job_state.
-- ---------------------------------------------------------------------------
DROP INDEX IF EXISTS idx_user_job_state_applied;
DROP INDEX IF EXISTS idx_user_job_state_saved;
DROP INDEX IF EXISTS idx_user_job_state_job_id;
DROP TABLE IF EXISTS user_job_state;

-- ---------------------------------------------------------------------------
-- 2r. Rename jobs -> job_feed and restore per-user columns.
-- ---------------------------------------------------------------------------
ALTER TABLE jobs RENAME TO job_feed;

ALTER TABLE job_feed
    ADD COLUMN status TEXT NOT NULL DEFAULT 'new'
               CHECK (status IN ('new', 'saved', 'applied',
                                 'interviewing', 'offer', 'dismissed')),
    ADD COLUMN saved  BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN notes  TEXT;

CREATE INDEX IF NOT EXISTS idx_job_feed_status_added ON job_feed(status, added_at DESC);
CREATE INDEX IF NOT EXISTS idx_job_feed_saved ON job_feed(added_at DESC) WHERE saved = true;

-- ---------------------------------------------------------------------------
-- 1r. Recreate materialized views to their migration-042 shape.
-- ---------------------------------------------------------------------------
CREATE MATERIALIZED VIEW mv_weekly_hiring_activity AS
SELECT
    DATE_TRUNC('week', added_at)::date::text AS week_start,
    COUNT(*)                                  AS job_count
FROM job_feed
GROUP BY DATE_TRUNC('week', added_at)
WITH DATA;
CREATE UNIQUE INDEX idx_mv_weekly_hiring_week ON mv_weekly_hiring_activity(week_start);

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

-- +goose StatementEnd
