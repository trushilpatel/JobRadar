-- +goose Up
-- Dismiss existing non-USA on-site jobs for all users.
--
-- The post-enrichment country gate (analyzer enricher.go) prevents future
-- non-USA on-site jobs from reaching the jobs table, but 621 historical
-- on-site entries slipped through before the fix. Remote non-USA jobs are
-- intentionally kept: they may be US-eligible even when posted from a
-- non-US company or location.
--
-- This migration upserts a dismissed state into user_job_state for every
-- non-remote job whose LLM-enriched country is non-empty and not USA, so
-- they disappear from user feeds without violating the FK invariant
-- (jobs rows are kept for referential integrity).
--
-- ON CONFLICT DO NOTHING preserves any status a user explicitly set (saved,
-- applied) — those are never overwritten.

INSERT INTO user_job_state (
    user_id,
    job_id,
    status,
    dismissed_at,
    dismissal_reason,
    dismissal_note,
    created_at,
    updated_at
)
SELECT
    u.id                AS user_id,
    j.id                AS job_id,
    'dismissed'         AS status,
    NOW()               AS dismissed_at,
    'non_usa_location'  AS dismissal_reason,
    NULL                AS dismissal_note,
    NOW()               AS created_at,
    NOW()               AS updated_at
FROM jobs j
CROSS JOIN users u
WHERE j.country IS NOT NULL
  AND j.country != 'USA'
  AND j.remote = false
ON CONFLICT (user_id, job_id) DO NOTHING;

-- +goose Down
DELETE FROM user_job_state
WHERE status = 'dismissed'
  AND dismissal_reason = 'non_usa_location'
  AND job_id IN (
    SELECT j.id FROM jobs j
    WHERE j.country IS NOT NULL
      AND j.country != 'USA'
      AND j.remote = false
  );
