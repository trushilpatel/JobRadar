-- +goose Up
CREATE INDEX idx_tech_tags_tagged_at_tech ON job_tech_tags (tagged_at, tech);

-- +goose Down
DROP INDEX IF EXISTS idx_tech_tags_tagged_at_tech;
