-- +goose Up
-- Register the Workday Job Board adapter.
--
-- After migration 044 (phase 2 multi-user cutover) the companies.adapter_type
-- column was dropped; the allowlist source-of-truth is now `adapter_configs`,
-- referenced via FK by company_scan_queue. Adding a new adapter requires only
-- a row in adapter_configs.
--
-- Inter-company delay is set to 15s (vs the typical 3-5s for other ATSes)
-- because Workday's myworkdayjobs.com endpoints rate-limit aggressively and
-- routinely return 429 / 503 under sustained per-tenant load. The 15s pacing
-- combined with the 24h scan interval (1440 minutes) keeps the global crawler
-- well below Workday's per-tenant thresholds even with hundreds of seeded
-- tenants.
--
-- See backend/apps/crawler/ats/workday/ for the adapter implementation.
INSERT INTO adapter_configs (adapter_type, scan_interval_minutes, inter_company_delay_s) VALUES
    ('workday', 1440, 15)
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM adapter_configs WHERE adapter_type = 'workday';
