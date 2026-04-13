-- +goose Up

-- Remove companies using deprecated adapters before tightening the constraint.
DELETE FROM companies
WHERE adapter_type NOT IN ('greenhouse', 'lever', 'ashby', 'google');

-- Remove workday, workable, chromedp, icims from the adapter_type CHECK constraint.
ALTER TABLE companies
    DROP CONSTRAINT IF EXISTS companies_adapter_type_check;

ALTER TABLE companies
    ADD CONSTRAINT companies_adapter_type_check
    CHECK(adapter_type IN (
        'greenhouse', 'lever', 'ashby',
        'google'
    ));

-- +goose Down

ALTER TABLE companies
    DROP CONSTRAINT IF EXISTS companies_adapter_type_check;

ALTER TABLE companies
    ADD CONSTRAINT companies_adapter_type_check
    CHECK(adapter_type IN (
        'greenhouse', 'lever', 'ashby',
        'workday', 'workable', 'chromedp',
        'google'
    ));
