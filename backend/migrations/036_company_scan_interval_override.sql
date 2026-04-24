-- +goose Up
ALTER TABLE companies
    ADD COLUMN scan_interval_minutes_override INT NULL
    CHECK(scan_interval_minutes_override IS NULL OR scan_interval_minutes_override > 0);

-- +goose Down
ALTER TABLE companies DROP COLUMN IF EXISTS scan_interval_minutes_override;
