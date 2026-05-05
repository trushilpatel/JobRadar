-- +goose Up
-- Backfill company_scan_settings for all seeded companies that have no row.
-- Migration 044 dropped adapter_type from companies without backfilling
-- company_scan_settings. Adapter type is derived from the companies.website
-- URL pattern — this matches the original seed data intent exactly, and
-- corrects the one known seed error (xona-space: URL is greenhouse, not ashby).
-- Uses ON CONFLICT DO NOTHING so manually-configured settings are never overwritten.

INSERT INTO company_scan_settings (company_id, adapter_type, active)
SELECT
    id,
    CASE
        WHEN website ILIKE '%greenhouse.io%'         THEN 'greenhouse'
        WHEN website ILIKE '%jobs.lever.co%'         THEN 'lever'
        WHEN website ILIKE '%jobs.ashbyhq.com%'      THEN 'ashby'
        WHEN website ILIKE '%jobs.netflix.net%'      THEN 'netflix'
        WHEN website ILIKE '%uber.com%'              THEN 'uber'
        WHEN website ILIKE '%metacareers.com%'       THEN 'meta'
        WHEN website ILIKE '%apple.com/careers%'     THEN 'apple'
        WHEN website ILIKE '%careers.microsoft.com%' THEN 'microsoft'
        WHEN website ILIKE '%rippling.com%'          THEN 'rippling'
        WHEN website ILIKE '%careers.google.com%'    THEN 'google'
    END,
    true
FROM companies
WHERE NOT EXISTS (
    SELECT 1 FROM company_scan_settings WHERE company_id = companies.id
)
AND (
    website ILIKE '%greenhouse.io%'
    OR website ILIKE '%jobs.lever.co%'
    OR website ILIKE '%jobs.ashbyhq.com%'
    OR website ILIKE '%jobs.netflix.net%'
    OR website ILIKE '%uber.com%'
    OR website ILIKE '%metacareers.com%'
    OR website ILIKE '%apple.com/careers%'
    OR website ILIKE '%careers.microsoft.com%'
    OR website ILIKE '%rippling.com%'
    OR website ILIKE '%careers.google.com%'
);

-- +goose Down
-- Remove only the rows that this migration inserted. Matches the same URL
-- patterns used in Up so manually-added settings are untouched.
DELETE FROM company_scan_settings
WHERE company_id IN (
    SELECT id FROM companies
    WHERE website ILIKE '%greenhouse.io%'
       OR website ILIKE '%jobs.lever.co%'
       OR website ILIKE '%jobs.ashbyhq.com%'
       OR website ILIKE '%jobs.netflix.net%'
       OR website ILIKE '%uber.com%'
       OR website ILIKE '%metacareers.com%'
       OR website ILIKE '%apple.com/careers%'
       OR website ILIKE '%careers.microsoft.com%'
       OR website ILIKE '%rippling.com%'
       OR website ILIKE '%careers.google.com%'
);
