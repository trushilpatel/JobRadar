-- +goose Up
CREATE TABLE job_scan_log (
    id            BIGSERIAL PRIMARY KEY,
    company_id    TEXT NOT NULL REFERENCES companies(id),
    scanned_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    jobs_fetched  INTEGER NOT NULL DEFAULT 0,
    jobs_new      INTEGER NOT NULL DEFAULT 0,
    error         TEXT,
    duration_ms   INTEGER
);

CREATE INDEX idx_scan_company ON job_scan_log(company_id);
CREATE INDEX idx_scan_time    ON job_scan_log(scanned_at DESC);

-- +goose Down
DROP INDEX idx_scan_time;
DROP INDEX idx_scan_company;
DROP TABLE job_scan_log;
