-- +goose Up
-- Normalize company.url to display-style URLs.
-- The backend now uses company.id (slug) directly; company.url is for display/link-out only.
UPDATE companies SET url = 'https://jobs.lever.co/' || id WHERE adapter_type = 'lever';
UPDATE companies SET url = 'https://jobs.ashbyhq.com/' || id WHERE adapter_type = 'ashby';

-- +goose Down
UPDATE companies SET url = 'https://api.lever.co/v0/postings/' || id WHERE adapter_type = 'lever';
UPDATE companies SET url = 'https://api.ashbyhq.com/posting-api/job-board/' || id WHERE adapter_type = 'ashby';
