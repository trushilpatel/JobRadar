-- +goose Up
-- Add missing indexes on company_scan_settings.
-- These columns appear in WHERE predicates and ORDER BY clauses on every
-- admin company list request but had no index support.

-- active: used in COUNT(*) WHERE active = true (admin list header) and
--         WHERE s.active = true/false filter in queryCompanies.
CREATE INDEX IF NOT EXISTS idx_css_active
    ON company_scan_settings (active);

-- last_scan_at: used when sort_by=last_scan_at in the admin list.
CREATE INDEX IF NOT EXISTS idx_css_last_scan_at
    ON company_scan_settings (last_scan_at DESC NULLS LAST);

-- adapter_type: used in WHERE s.adapter_type IN (?) filter and
--               ORDER BY s.adapter_type in the admin list.
CREATE INDEX IF NOT EXISTS idx_css_adapter_type
    ON company_scan_settings (adapter_type);

-- +goose Down
DROP INDEX IF EXISTS idx_css_adapter_type;
DROP INDEX IF EXISTS idx_css_last_scan_at;
DROP INDEX IF EXISTS idx_css_active;
