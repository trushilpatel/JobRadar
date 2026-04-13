-- +goose Up

-- Add 'google' to the adapter_type CHECK constraint on the companies table.
-- Drop the old constraint and create a new one with the additional value.
ALTER TABLE companies
    DROP CONSTRAINT IF EXISTS companies_adapter_type_check;

ALTER TABLE companies
    ADD CONSTRAINT companies_adapter_type_check
    CHECK(adapter_type IN (
        'greenhouse', 'lever', 'ashby',
        'workday', 'workable', 'chromedp',
        'google'
    ));

-- +goose Down

ALTER TABLE companies
    DROP CONSTRAINT IF EXISTS companies_adapter_type_check;

ALTER TABLE companies
    ADD CONSTRAINT companies_adapter_type_check
    CHECK(adapter_type IN (
        'greenhouse', 'lever', 'ashby',
        'workday', 'workable', 'chromedp'
    ));
