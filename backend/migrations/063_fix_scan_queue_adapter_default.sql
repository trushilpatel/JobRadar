-- +goose Up
-- Fix company_scan_queue.adapter_type: remove the empty-string DEFAULT added in
-- migration 035 and add a CHECK to reject empty strings. The DEFAULT '' was a
-- backfill sentinel; after the UPDATE in 035 all rows carry a real adapter type.
-- Dropping the default means new inserts must supply adapter_type explicitly,
-- which is correct — the application always sets it from company_scan_settings.

ALTER TABLE company_scan_queue
    ALTER COLUMN adapter_type DROP DEFAULT;

ALTER TABLE company_scan_queue
    ADD CONSTRAINT chk_company_scan_queue_adapter_nonempty
    CHECK (adapter_type <> '') NOT VALID;

ALTER TABLE company_scan_queue
    VALIDATE CONSTRAINT chk_company_scan_queue_adapter_nonempty;

-- +goose Down
ALTER TABLE company_scan_queue
    DROP CONSTRAINT IF EXISTS chk_company_scan_queue_adapter_nonempty;

ALTER TABLE company_scan_queue
    ALTER COLUMN adapter_type SET DEFAULT '';
