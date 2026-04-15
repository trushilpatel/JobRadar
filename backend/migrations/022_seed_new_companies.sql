-- +goose Up

-- Seed company rows for adapters added in migration 021.
-- last_scan_at is set to NOW() so the scheduler does not immediately pick them up;
-- trigger the first scan manually via POST /companies/:id/scan.
INSERT INTO companies (id, name, url, adapter_type, active, last_scan_at)
VALUES
    ('microsoft', 'Microsoft', 'https://careers.microsoft.com',          'microsoft', true, NOW()),
    ('meta',      'Meta',      'https://www.metacareers.com/jobs/',       'meta',      true, NOW()),
    ('apple',     'Apple',     'https://www.apple.com/careers/us/',       'apple',     true, NOW()),
    ('rippling',  'Rippling',  'https://www.rippling.com/careers/open-roles', 'rippling', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- +goose Down
DELETE FROM companies WHERE id IN ('microsoft', 'meta', 'apple', 'rippling');
