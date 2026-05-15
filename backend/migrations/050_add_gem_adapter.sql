-- +goose Up
-- Add gem adapter for gem.com Job Board API.
-- The Job Board API is a single GET that returns every open posting with full content
-- (see backend/apps/crawler/ats/gem/gem.go). 3s inter-company delay matches the other
-- pure-HTTP JSON adapters; 1440min scan interval mirrors the default.

ALTER TABLE companies DROP CONSTRAINT IF EXISTS companies_adapter_type_check;
ALTER TABLE companies ADD CONSTRAINT companies_adapter_type_check
    CHECK(adapter_type IN ('greenhouse', 'lever', 'ashby', 'google', 'netflix', 'uber', 'microsoft', 'rippling', 'apple', 'meta', 'gem'));

INSERT INTO adapter_configs (adapter_type, scan_interval_minutes, inter_company_delay_s) VALUES
    ('gem', 1440, 3)
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM adapter_configs WHERE adapter_type = 'gem';
ALTER TABLE companies DROP CONSTRAINT IF EXISTS companies_adapter_type_check;
ALTER TABLE companies ADD CONSTRAINT companies_adapter_type_check
    CHECK(adapter_type IN ('greenhouse', 'lever', 'ashby', 'google', 'netflix', 'uber', 'microsoft', 'rippling', 'apple', 'meta'));
