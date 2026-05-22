-- +goose Up
-- Add CHECK constraint on company_scan_queue.source to enumerate valid values.
-- The column was created in 014 with DEFAULT 'scheduler' but no constraint.
-- Valid sources: 'scheduler' (automatic cadence) and 'manual' (API-triggered).
-- Matches the validation in scan/services/ScanValidationService.go.

ALTER TABLE company_scan_queue
    ADD CONSTRAINT chk_company_scan_queue_source
    CHECK (source IN ('scheduler', 'manual')) NOT VALID;

ALTER TABLE company_scan_queue
    VALIDATE CONSTRAINT chk_company_scan_queue_source;

-- +goose Down
ALTER TABLE company_scan_queue
    DROP CONSTRAINT IF EXISTS chk_company_scan_queue_source;
