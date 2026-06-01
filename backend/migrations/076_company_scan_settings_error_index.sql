-- +goose Up
-- Partial index supporting the admin company-list scan_status=error filter,
-- which selects company_scan_settings rows WHERE consecutive_errors > 0.
-- A partial index keeps the index tiny (only erroring companies) while making
-- the filter an index scan instead of a full table scan.
--
-- No CONCURRENTLY: goose runs each migration in a transaction block where
-- CONCURRENTLY is forbidden. IF NOT EXISTS guards against re-run.

CREATE INDEX IF NOT EXISTS idx_css_consecutive_errors
    ON company_scan_settings (company_id)
    WHERE consecutive_errors > 0;

-- +goose Down
DROP INDEX IF EXISTS idx_css_consecutive_errors;
