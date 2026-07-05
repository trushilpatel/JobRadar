-- +goose Up
-- Dedupe existing (user_id, label) collisions before the unique index below
-- can be created. For any user+label pair that already repeats, the oldest
-- row keeps its label; every later duplicate is suffixed with a fragment of
-- its own id (globally unique, so the rewrite can never collide with itself
-- or an existing label) rather than silently dropped.
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY user_id, label ORDER BY created_at ASC, id ASC) AS rn
    FROM resumes
)
UPDATE resumes
SET label = resumes.label || ' (dup-' || substr(resumes.id::text, 1, 8) || ')'
FROM ranked
WHERE resumes.id = ranked.id AND ranked.rn > 1;

CREATE UNIQUE INDEX idx_resumes_user_id_label ON resumes(user_id, label);

-- +goose Down
DROP INDEX IF EXISTS idx_resumes_user_id_label;
