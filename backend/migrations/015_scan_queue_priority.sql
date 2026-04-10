-- +goose Up
ALTER TABLE company_scan_queue ADD COLUMN priority INT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE company_scan_queue DROP COLUMN priority;
