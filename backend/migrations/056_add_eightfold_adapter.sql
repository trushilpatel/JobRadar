-- +goose Up
-- Register the Eightfold AI Talent Intelligence adapter.
--
-- Eightfold powers careers boards at company-specific hosts (e.g.
-- explore.jobs.netflix.net, jobs.bms.com, etc.) backed by a shared public
-- positions API. The adapter requires per-company configuration
-- (host + domain + API key) supplied via company_scan_settings.adapter_config
-- and is wired through types.ConfigurableAdapter.
--
-- Inter-company delay is set to 5s — Eightfold's CDN (CloudFront) tolerates
-- rapid pagination internally (150ms inter-page is fine), but spacing the
-- per-tenant claims keeps a single crawler instance well below per-IP
-- thresholds when many tenants are seeded.
--
-- See backend/apps/crawler/ats/eightfold/ for the adapter implementation.
INSERT INTO adapter_configs (adapter_type, scan_interval_minutes, inter_company_delay_s) VALUES
    ('eightfold', 1440, 5)
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM adapter_configs WHERE adapter_type = 'eightfold';
