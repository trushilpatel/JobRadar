-- +goose Up
ALTER TABLE companies DROP CONSTRAINT IF EXISTS companies_adapter_type_check;
ALTER TABLE companies ADD CONSTRAINT companies_adapter_type_check
    CHECK(adapter_type IN ('greenhouse', 'lever', 'ashby', 'google', 'netflix', 'uber'));

-- +goose Down
ALTER TABLE companies DROP CONSTRAINT IF EXISTS companies_adapter_type_check;
ALTER TABLE companies ADD CONSTRAINT companies_adapter_type_check
    CHECK(adapter_type IN ('greenhouse', 'lever', 'ashby', 'google', 'netflix'));
