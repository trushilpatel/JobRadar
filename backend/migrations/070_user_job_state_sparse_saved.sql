-- +goose Up
-- Sparse user_job_state model + materialized per-user counters.
--
-- Migration 044 modelled "saved" as a workflow status, which made saving a job
-- remove it from the New tab. This migration converts "saved" into an orthogonal
-- flag on user_job_state.saved_at:
--
--   * status becomes nullable; NULL means "no workflow status".
--   * 'saved' is dropped from the status CHECK constraint — saved_at IS NOT NULL
--     is the source of truth for "this job is saved".
--   * A job can simultaneously be saved (saved_at IS NOT NULL) AND in a workflow
--     stage (status='applied'/'interviewing'/etc.).
--
-- Also materializes per-user counts in user_job_counters for fast tab-badge
-- reads (no GROUP BY scan per request). The application layer updates this
-- table atomically with every mutation.

-- +goose StatementBegin

-- 1. Backfill saved_at for rows whose status='saved' but saved_at is NULL.
--    Falls back to updated_at so the post-migration "Saved" tab order matches
--    the historical save time as closely as the data allows.
UPDATE user_job_state
   SET saved_at = COALESCE(saved_at, updated_at)
 WHERE status = 'saved'
   AND saved_at IS NULL;

-- 2. Convert status='saved' to status=NULL. The new model treats saved as
--    orthogonal to workflow stage; saved_at carries the signal.
UPDATE user_job_state
   SET status = NULL
 WHERE status = 'saved';

-- 3. Drop the legacy partial indexes that filtered on status='saved' / status='applied'.
--    They would survive the conversion but be empty (for saved) or unnecessarily
--    narrow. The replacements below cover both columns directly.
DROP INDEX IF EXISTS idx_user_job_state_saved;
DROP INDEX IF EXISTS idx_user_job_state_applied;

-- 4. Drop NOT NULL and the legacy CHECK that still listed 'saved'.
ALTER TABLE user_job_state
    DROP CONSTRAINT IF EXISTS user_job_state_status_check;

ALTER TABLE user_job_state
    ALTER COLUMN status DROP NOT NULL;

-- 5. Recreate the status CHECK without 'saved'. NULL is allowed.
ALTER TABLE user_job_state
    ADD CONSTRAINT user_job_state_status_check
    CHECK (status IS NULL
        OR status IN ('applied', 'dismissed', 'interviewing', 'offer', 'rejected'));

-- 6. Guard against meaningless rows (every field NULL). At least one of
--    status / saved_at / notes / dismissal_reason / dismissal_note must be set.
ALTER TABLE user_job_state
    ADD CONSTRAINT user_job_state_non_empty_check
    CHECK (
        status IS NOT NULL
        OR saved_at IS NOT NULL
        OR applied_at IS NOT NULL
        OR dismissed_at IS NOT NULL
        OR notes IS NOT NULL
        OR dismissal_reason IS NOT NULL
        OR dismissal_note IS NOT NULL
    );

-- 7. New partial indexes — saved_at orthogonal to status, and a status-only
--    index for workflow tabs (applied / interviewing / etc.).
CREATE INDEX IF NOT EXISTS idx_user_job_state_saved_at
    ON user_job_state (user_id, saved_at DESC)
    WHERE saved_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_user_job_state_status
    ON user_job_state (user_id, status, updated_at DESC)
    WHERE status IS NOT NULL;

-- 8. Materialized per-user counts. Each counter is updated atomically with
--    its corresponding user_job_state mutation by the application layer; the
--    repository's CountsByUser reads from this single row and falls back to a
--    GROUP BY recompute if the row is missing.
CREATE TABLE IF NOT EXISTS user_job_counters (
    user_id            UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    saved_count        INTEGER NOT NULL DEFAULT 0 CHECK (saved_count        >= 0),
    applied_count      INTEGER NOT NULL DEFAULT 0 CHECK (applied_count      >= 0),
    interviewing_count INTEGER NOT NULL DEFAULT 0 CHECK (interviewing_count >= 0),
    offer_count        INTEGER NOT NULL DEFAULT 0 CHECK (offer_count        >= 0),
    rejected_count     INTEGER NOT NULL DEFAULT 0 CHECK (rejected_count     >= 0),
    dismissed_count    INTEGER NOT NULL DEFAULT 0 CHECK (dismissed_count    >= 0),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 9. Seed user_job_counters from current state. ON CONFLICT lets the migration
--    be re-run safely.
INSERT INTO user_job_counters
    (user_id, saved_count, applied_count, interviewing_count,
     offer_count, rejected_count, dismissed_count, updated_at)
SELECT
    user_id,
    COUNT(*) FILTER (WHERE saved_at IS NOT NULL)         AS saved_count,
    COUNT(*) FILTER (WHERE status   = 'applied')         AS applied_count,
    COUNT(*) FILTER (WHERE status   = 'interviewing')    AS interviewing_count,
    COUNT(*) FILTER (WHERE status   = 'offer')           AS offer_count,
    COUNT(*) FILTER (WHERE status   = 'rejected')        AS rejected_count,
    COUNT(*) FILTER (WHERE status   = 'dismissed')       AS dismissed_count,
    NOW()
FROM user_job_state
GROUP BY user_id
ON CONFLICT (user_id) DO UPDATE
    SET saved_count        = EXCLUDED.saved_count,
        applied_count      = EXCLUDED.applied_count,
        interviewing_count = EXCLUDED.interviewing_count,
        offer_count        = EXCLUDED.offer_count,
        rejected_count     = EXCLUDED.rejected_count,
        dismissed_count    = EXCLUDED.dismissed_count,
        updated_at         = NOW();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- 1. Drop the materialized counters table.
DROP TABLE IF EXISTS user_job_counters;

-- 2. Drop the new indexes.
DROP INDEX IF EXISTS idx_user_job_state_status;
DROP INDEX IF EXISTS idx_user_job_state_saved_at;

-- 3. Drop the non-empty guard.
ALTER TABLE user_job_state
    DROP CONSTRAINT IF EXISTS user_job_state_non_empty_check;

-- 4. Re-saved any row whose saved_at is set but status is NULL — the legacy
--    model required status='saved' for those rows. Workflow-status rows
--    keep their existing status; the workflow status wins over saved.
UPDATE user_job_state
   SET status = 'saved'
 WHERE status IS NULL
   AND saved_at IS NOT NULL;

-- 5. Drop rows that are now still entirely empty (NULL status and no other
--    meaningful field). The legacy schema required NOT NULL status.
DELETE FROM user_job_state
 WHERE status IS NULL;

-- 6. Restore NOT NULL and the legacy CHECK.
ALTER TABLE user_job_state
    DROP CONSTRAINT IF EXISTS user_job_state_status_check;

ALTER TABLE user_job_state
    ALTER COLUMN status SET NOT NULL;

ALTER TABLE user_job_state
    ADD CONSTRAINT user_job_state_status_check
    CHECK (status IN ('saved', 'applied', 'dismissed', 'interviewing', 'offer', 'rejected'));

-- 7. Recreate the legacy partial indexes.
CREATE INDEX IF NOT EXISTS idx_user_job_state_saved
    ON user_job_state(user_id, saved_at DESC)
    WHERE status = 'saved';

CREATE INDEX IF NOT EXISTS idx_user_job_state_applied
    ON user_job_state(user_id, applied_at DESC)
    WHERE status = 'applied';

-- +goose StatementEnd
