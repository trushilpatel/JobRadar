-- +goose Up
-- Best-effort backfill from job_analytics into job_tech_tags.
-- Only jobs that exist in job_feed (FK constraint) and have a non-empty tech_stack are included.
-- Tags are lowercased and trimmed; validation/normalization runs going-forward in the app layer.
INSERT INTO job_tech_tags (job_id, tech, company_id, tagged_at)
SELECT
    ja.id        AS job_id,
    TRIM(tech)   AS tech,
    ja.company_id,
    ja.snapshotted_at AS tagged_at
FROM job_analytics ja,
     jsonb_array_elements_text(ja.tech_stack) AS tech
WHERE ja.tech_stack IS NOT NULL
  AND ja.tech_stack::text NOT IN ('null', '[]', '')
  AND EXISTS (SELECT 1 FROM job_feed jf WHERE jf.id = ja.id)
  AND TRIM(tech) <> ''
ON CONFLICT (job_id, tech) DO NOTHING;

-- +goose Down
-- Backfill is non-destructive; no rollback needed beyond dropping the table (handled by 025 down).
