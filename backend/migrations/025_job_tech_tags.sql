-- +goose Up
CREATE TABLE job_tech_tags (
    job_id     TEXT        NOT NULL REFERENCES job_feed(id) ON DELETE CASCADE,
    tech       TEXT        NOT NULL,
    company_id TEXT        NOT NULL REFERENCES companies(id),
    tagged_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (job_id, tech)
);

-- Supports quarterly aggregation queries (GROUP BY tech, date range on tagged_at)
CREATE INDEX idx_job_tech_tags_tech_tagged ON job_tech_tags (tech, tagged_at);

-- Supports per-company lookups if needed later
CREATE INDEX idx_job_tech_tags_company_tagged ON job_tech_tags (company_id, tagged_at);

-- +goose Down
DROP TABLE IF EXISTS job_tech_tags;
