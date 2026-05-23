-- +goose Up
CREATE INDEX IF NOT EXISTS idx_job_analytics_company_snapshotted ON job_analytics (company_id, snapshotted_at);
CREATE INDEX IF NOT EXISTS idx_job_pipeline_ledger_removed_at ON job_pipeline_ledger (removed_at) WHERE removed_at IS NOT NULL;
ALTER TABLE job_analytics ADD CONSTRAINT job_analytics_employ_type_check
    CHECK (employ_type IN ('full_time', 'part_time', 'contract', 'internship'));

-- +goose Down
DROP INDEX IF EXISTS idx_job_analytics_company_snapshotted;
DROP INDEX IF EXISTS idx_job_pipeline_ledger_removed_at;
ALTER TABLE job_analytics DROP CONSTRAINT IF EXISTS job_analytics_employ_type_check;
