-- +goose Up
CREATE INDEX idx_job_feed_job_post_id ON job_feed(job_post_id);

-- +goose Down
DROP INDEX idx_job_feed_job_post_id;
