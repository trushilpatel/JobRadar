-- +goose Up
-- Deduplicate jobs by job_post_id and enforce uniqueness going forward.
--
-- TIMING: run BEFORE the new binary deploy. Deduplication and the unique-index
-- install run in a single transaction so concurrent inserts cannot land a fresh
-- duplicate between the DELETE and the CREATE UNIQUE INDEX. The new InsertJob
-- (ON CONFLICT (job_post_id) DO NOTHING) requires this unique index to exist.
--
-- ONE-WAY: the Down section drops the unique index and recreates the original
-- non-unique index, but it CANNOT restore the rows deleted here.
--
-- Dedup policy: keep the OLDEST row per job_post_id (MIN(added_at), tiebroken by
-- MIN(id)). The oldest row has had the longest to accumulate per-user state and
-- snapshots; deleting it would cascade through user_job_state and
-- job_application_tracker (both ON DELETE CASCADE) and lose user data.

-- +goose StatementBegin
DO $$
BEGIN
    -- Delete every duplicate row except the oldest per job_post_id.
    DELETE FROM jobs j
    USING (
        SELECT id
        FROM (
            SELECT
                id,
                ROW_NUMBER() OVER (
                    PARTITION BY job_post_id
                    ORDER BY added_at ASC, id ASC
                ) AS rn
            FROM jobs
        ) ranked
        WHERE ranked.rn > 1
    ) dupes
    WHERE j.id = dupes.id;
END $$;
-- +goose StatementEnd

-- Replace the non-unique index with a unique one. job_post_id is now an
-- application-level invariant; the unique index enforces it at the DB layer.
DROP INDEX IF EXISTS idx_job_feed_job_post_id;
CREATE UNIQUE INDEX uq_jobs_job_post_id ON jobs(job_post_id);

-- +goose Down
-- Cannot restore the deleted duplicate rows — this is a one-way data migration.
-- We only revert the index shape.
DROP INDEX IF EXISTS uq_jobs_job_post_id;
CREATE INDEX idx_job_feed_job_post_id ON jobs(job_post_id);
