-- +goose Up
-- Expand the user_job_state.status CHECK constraint to include
-- 'interviewing', 'offer', and 'rejected'. The constraint in migration 044
-- only allowed ('saved', 'applied', 'dismissed'); 'interviewing' and 'offer'
-- were in the frontend but missing from the DB, making those status writes fail.
ALTER TABLE user_job_state
    DROP CONSTRAINT IF EXISTS user_job_state_status_check;

ALTER TABLE user_job_state
    ADD CONSTRAINT user_job_state_status_check
    CHECK (status IN ('saved', 'applied', 'dismissed', 'interviewing', 'offer', 'rejected'));

-- +goose Down
ALTER TABLE user_job_state
    DROP CONSTRAINT IF EXISTS user_job_state_status_check;

ALTER TABLE user_job_state
    ADD CONSTRAINT user_job_state_status_check
    CHECK (status IN ('saved', 'applied', 'dismissed'));
