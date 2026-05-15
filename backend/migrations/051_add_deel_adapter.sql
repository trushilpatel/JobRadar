-- +goose Up
-- Register the jobs.deel.com adapter.
--
-- Allowlist source-of-truth is `adapter_configs` (FK-referenced by
-- company_scan_queue). 5s inter-company delay — slightly more conservative
-- than gem because each scan downloads a ~100-600 KB RSC stream rather than a
-- compact JSON list.
--
-- See backend/apps/crawler/ats/deel/ for the adapter implementation.
INSERT INTO adapter_configs (adapter_type, scan_interval_minutes, inter_company_delay_s) VALUES
    ('deel', 1440, 5)
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM adapter_configs WHERE adapter_type = 'deel';
