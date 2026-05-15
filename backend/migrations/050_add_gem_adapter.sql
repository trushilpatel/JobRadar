-- +goose Up
-- Register the gem.com Job Board adapter.
--
-- After migration 044 (phase 2 multi-user cutover) the companies.adapter_type
-- column was dropped; the allowlist source-of-truth is now `adapter_configs`,
-- referenced via FK by company_scan_queue. Adding a new adapter requires only
-- a row in adapter_configs.
--
-- See backend/apps/crawler/ats/gem/ for the adapter implementation.
INSERT INTO adapter_configs (adapter_type, scan_interval_minutes, inter_company_delay_s) VALUES
    ('gem', 1440, 3)
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM adapter_configs WHERE adapter_type = 'gem';
