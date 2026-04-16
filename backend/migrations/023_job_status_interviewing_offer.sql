-- +goose Up
ALTER TABLE job_feed
  DROP CONSTRAINT IF EXISTS job_status_check,
  ADD CONSTRAINT job_status_check
    CHECK (status IN ('new', 'saved', 'applied', 'interviewing', 'offer', 'dismissed'));

-- +goose Down
ALTER TABLE job_feed
  DROP CONSTRAINT IF EXISTS job_status_check,
  ADD CONSTRAINT job_status_check
    CHECK (status IN ('new', 'saved', 'applied', 'dismissed'));
