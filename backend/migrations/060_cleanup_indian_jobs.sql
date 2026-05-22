-- +goose Up
-- Hard-delete Indian / non-USA job rows that were ingested before the
-- post-fetch location gate (migration 059 + analyzer fix) was deployed.
-- Deletes from user_job_state and job_analytics first (FK children), then
-- jobs (FK parent), then stamps the ledger rows non_usa_location.
--
-- The CTE captures both the UUID primary key (id) and the ATS job-post ID
-- (job_post_id) so the ledger — which is keyed on job_post_id — can be
-- updated in the same statement.

WITH indian_jobs AS (
    SELECT id, job_post_id
    FROM jobs
    WHERE country = 'India'
       OR location ILIKE ANY(ARRAY[
            '%india%',
            '%bangalore%',
            '%bengaluru%',
            '%mumbai%',
            '%delhi%',
            '%hyderabad%',
            '%chennai%',
            '%pune%',
            '%kolkata%',
            '%gurgaon%',
            '%gurugram%',
            '%noida%',
            '%ahmedabad%',
            '%jaipur%',
            '%coimbatore%',
            '%nagpur%',
            '%bhubaneswar%',
            '%indore%'
        ])
),
del_user_job_state AS (
    DELETE FROM user_job_state
    WHERE job_id IN (SELECT id FROM indian_jobs)
),
del_job_analytics AS (
    DELETE FROM job_analytics
    WHERE id IN (SELECT id FROM indian_jobs)
),
del_jobs AS (
    DELETE FROM jobs
    WHERE id IN (SELECT id FROM indian_jobs)
)
UPDATE job_pipeline_ledger
SET reason = 'non_usa_location'
WHERE job_id IN (SELECT job_post_id FROM indian_jobs)
  AND reason != 'non_usa_location';

-- +goose Down
-- no-op: deleted rows cannot be recovered from this migration
