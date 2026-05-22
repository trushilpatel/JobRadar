-- +goose Up
-- Add FK from company_scan_settings.adapter_type → adapter_configs(adapter_type).
-- company_scan_queue already has this constraint (migration 035); company_scan_settings
-- was created in 044 without it, leaving adapter_type unconstrained at the DB level.
-- Mirrors the FK pattern on company_scan_queue so both tables enforce valid adapters
-- through the adapter_configs primary key rather than a hand-maintained CHECK list.

ALTER TABLE company_scan_settings
    ADD CONSTRAINT fk_css_adapter_type
    FOREIGN KEY (adapter_type) REFERENCES adapter_configs(adapter_type);

-- +goose Down
ALTER TABLE company_scan_settings
    DROP CONSTRAINT IF EXISTS fk_css_adapter_type;
