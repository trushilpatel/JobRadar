-- +goose Up

-- job_feed: status + time composite (main feed query filters by status, orders by added_at)
CREATE INDEX idx_job_feed_status_added ON job_feed(status, added_at DESC);

-- job_pipeline_ledger: reason + time composite (dashboard + pipeline count queries)
CREATE INDEX idx_ledger_reason_time ON job_pipeline_ledger(reason, first_seen_at DESC);

-- job_pipeline_ledger: partial index for active (non-removed) rows by company (runs on every crawl)
CREATE INDEX idx_ledger_active ON job_pipeline_ledger(company_id) WHERE removed_at IS NULL;

-- job_application_tracker: composite for DISTINCT ON + ORDER BY in paged application listing
CREATE INDEX idx_app_job_time ON job_application_tracker(job_result_id, stage_at DESC);

-- job_feed: GIN index on tech_stack JSONB for unnest-based insights queries
CREATE INDEX idx_job_feed_tech_stack ON job_feed USING GIN (tech_stack);

-- +goose Down
DROP INDEX idx_job_feed_status_added;
DROP INDEX idx_ledger_reason_time;
DROP INDEX idx_ledger_active;
DROP INDEX idx_app_job_time;
DROP INDEX idx_job_feed_tech_stack;
