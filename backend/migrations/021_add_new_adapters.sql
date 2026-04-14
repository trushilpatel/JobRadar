-- +goose Up
-- Add microsoft, rippling, apple, and meta adapter types to the CHECK constraint.
ALTER TABLE companies DROP CONSTRAINT IF EXISTS companies_adapter_type_check;
ALTER TABLE companies ADD CONSTRAINT companies_adapter_type_check
    CHECK(adapter_type IN ('greenhouse', 'lever', 'ashby', 'google', 'netflix', 'uber', 'microsoft', 'rippling', 'apple', 'meta'));

-- +goose Down
ALTER TABLE companies DROP CONSTRAINT IF EXISTS companies_adapter_type_check;
ALTER TABLE companies ADD CONSTRAINT companies_adapter_type_check
    CHECK(adapter_type IN ('greenhouse', 'lever', 'ashby', 'google', 'netflix', 'uber'));
