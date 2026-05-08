-- +goose Up

-- Migration 037 renamed status 'processing' → 'running' in company_scan_queue
-- but did not update the partial unique index, which still references the now-
-- impossible 'processing' value. The CHECK constraint prevents any 'processing'
-- row from being inserted, so the index effectively only covers 'queued' rows.
-- This means a company with a 'running' row can be re-enqueued as 'queued'
-- by the manual Enqueue path (ON CONFLICT DO NOTHING never fires for 'running').
-- Replace the index to cover the correct statuses.
DROP INDEX IF EXISTS uq_company_scan_queue_active;
CREATE UNIQUE INDEX uq_company_scan_queue_active
    ON company_scan_queue(company_id)
    WHERE status IN ('queued', 'running');

-- +goose Down

DROP INDEX IF EXISTS uq_company_scan_queue_active;
CREATE UNIQUE INDEX uq_company_scan_queue_active
    ON company_scan_queue(company_id)
    WHERE status IN ('queued', 'processing');
