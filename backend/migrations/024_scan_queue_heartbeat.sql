-- +goose Up
ALTER TABLE company_scan_queue ADD COLUMN heartbeat_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE company_scan_queue DROP COLUMN heartbeat_at;
