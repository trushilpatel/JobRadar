-- +goose Up
-- Backfill job_analytics for all jobs that are missing an analytics snapshot.
--
-- Root cause: prior to fix 116cf77, the analyzer sent empty bytes ('') to the
-- job_analytics.tech_stack (jsonb) column for jobs with no tech stack.
-- Postgres rejects empty bytes for jsonb (SQLSTATE 22P02). The snapshot write
-- is non-fatal so the error was logged as WARN and the row was silently dropped.
-- ~5 000 jobs accumulated with no analytics row between 2026-05-01 and 2026-06-27.
--
-- This migration inserts the missing rows using data already present in the
-- jobs table. role_category is not stored in jobs so it is left NULL.
-- snapshotted_at uses the job's added_at date to preserve the original timeline.

INSERT INTO job_analytics (
    id, company_id, snapshotted_at,
    country, state, remote,
    sponsorship, requires_citizen, security_clearance,
    employ_type, experience_years, tech_stack
)
SELECT
    j.id,
    j.company_id,
    j.added_at::date                         AS snapshotted_at,
    j.country,
    j.state,
    j.remote,
    j.sponsorship,
    j.requires_citizen,
    j.security_clearance,
    j.employ_type,
    j.experience_years,
    COALESCE(j.tech_stack, '[]'::jsonb)
FROM jobs j
LEFT JOIN job_analytics a ON a.id = j.id
WHERE a.id IS NULL
ON CONFLICT (id) DO NOTHING;

-- +goose Down
-- Not reversible: we cannot distinguish rows inserted by this migration
-- from rows that were already present. Leave job_analytics intact on rollback.
