-- +goose Up
CREATE INDEX IF NOT EXISTS idx_job_analytics_role_category ON job_analytics(role_category);
CREATE INDEX IF NOT EXISTS idx_job_analytics_state ON job_analytics(state) WHERE state IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_job_analytics_role_category;
DROP INDEX IF EXISTS idx_job_analytics_state;
