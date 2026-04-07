-- +goose Up

-- 1. Allow 'dismissed' as a valid job status
ALTER TABLE job DROP CONSTRAINT IF EXISTS job_status_check;
ALTER TABLE job ADD CONSTRAINT job_status_check
    CHECK (status IN ('new', 'saved', 'applied', 'dismissed'));

-- 2. Structured location on job (LLM extracts these at enrichment time)
ALTER TABLE job
    ADD COLUMN country TEXT,
    ADD COLUMN state   TEXT;

CREATE INDEX idx_job_country ON job(country);

-- 3. Track when a job disappears from a careers page (role filled signal)
ALTER TABLE job_seen_ledger ADD COLUMN removed_at TIMESTAMPTZ;

-- 4. Cascade deletes on job_discarded so job rows can be cleaned up freely
ALTER TABLE job_discarded
    DROP CONSTRAINT job_discarded_job_result_id_fkey;

ALTER TABLE job_discarded
    ADD CONSTRAINT job_discarded_job_result_id_fkey
    FOREIGN KEY (job_result_id) REFERENCES job(id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE job_discarded DROP CONSTRAINT job_discarded_job_result_id_fkey;
ALTER TABLE job_discarded ADD CONSTRAINT job_discarded_job_result_id_fkey
    FOREIGN KEY (job_result_id) REFERENCES job(id);

ALTER TABLE job_seen_ledger DROP COLUMN removed_at;

DROP INDEX IF EXISTS idx_job_country;
ALTER TABLE job DROP COLUMN IF EXISTS country;
ALTER TABLE job DROP COLUMN IF EXISTS state;

ALTER TABLE job DROP CONSTRAINT IF EXISTS job_status_check;
ALTER TABLE job ADD CONSTRAINT job_status_check
    CHECK (status IN ('new', 'saved', 'applied'));
