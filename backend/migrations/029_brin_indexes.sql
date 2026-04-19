-- +goose Up

-- job_pipeline_ledger: rows inserted in time order — BRIN eliminates old page blocks on time-range scans
CREATE INDEX idx_ledger_first_seen_brin ON job_pipeline_ledger USING BRIN (first_seen_at);

-- job_feed: same pattern — time-range filters skip irrelevant blocks
CREATE INDEX idx_job_feed_added_brin ON job_feed USING BRIN (added_at);

-- job_analytics: snapshotted_at is a DATE written once per job — BRIN covers all insights time-range queries
CREATE INDEX idx_analytics_snapshotted_brin ON job_analytics USING BRIN (snapshotted_at);

-- job_tech_tags: tagged_at is set on insert — BRIN covers quarterly adoption and tech trend queries
CREATE INDEX idx_tech_tags_tagged_brin ON job_tech_tags USING BRIN (tagged_at);

-- +goose Down
DROP INDEX idx_ledger_first_seen_brin;
DROP INDEX idx_job_feed_added_brin;
DROP INDEX idx_analytics_snapshotted_brin;
DROP INDEX idx_tech_tags_tagged_brin;
