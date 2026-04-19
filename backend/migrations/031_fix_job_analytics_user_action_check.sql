-- +goose Up
ALTER TABLE job_analytics DROP CONSTRAINT IF EXISTS job_analytics_user_action_check;
ALTER TABLE job_analytics ADD CONSTRAINT job_analytics_user_action_check
    CHECK (user_action IN ('saved', 'applied', 'dismissed', 'interviewing', 'offer'));

-- +goose Down
ALTER TABLE job_analytics DROP CONSTRAINT IF EXISTS job_analytics_user_action_check;
ALTER TABLE job_analytics ADD CONSTRAINT job_analytics_user_action_check
    CHECK (user_action IN ('saved', 'applied', 'dismissed'));
