-- +goose Up
ALTER TABLE job_feed ALTER COLUMN responsibilities TYPE JSONB USING NULL;

-- +goose Down
ALTER TABLE job_feed ALTER COLUMN responsibilities TYPE TEXT USING NULL;
