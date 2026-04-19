-- +goose Up

-- Add CHECK constraint on company_scan_queue.status — only 'queued' and 'processing' are valid;
-- rows are deleted on completion, not transitioned to 'done'/'failed'.
ALTER TABLE company_scan_queue
    ADD CONSTRAINT company_scan_queue_status_check
    CHECK (status IN ('queued', 'processing'));

-- Replace the low-cardinality boolean btree index on job_feed.saved with a partial index.
-- The btree splits into only 2 buckets (true/false) and is often ignored by the planner.
-- The partial index covers only saved=true rows (small fraction) and includes added_at
-- so WHERE saved = true ORDER BY added_at DESC becomes an index-only scan.
DROP INDEX idx_job_saved;
CREATE INDEX idx_job_feed_saved ON job_feed(added_at DESC) WHERE saved = true;

-- +goose Down
DROP INDEX idx_job_feed_saved;
CREATE INDEX idx_job_saved ON job_feed(saved);
ALTER TABLE company_scan_queue DROP CONSTRAINT company_scan_queue_status_check;
