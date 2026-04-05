-- +goose Up
CREATE TABLE job_discarded (
    id            BIGSERIAL PRIMARY KEY,
    job_result_id TEXT NOT NULL REFERENCES job(id),
    reason        TEXT NOT NULL
                      CHECK(reason IN (
                          'user_dismissed',
                          'user_not_relevant'
                      )),
    discarded_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    note          TEXT
);

CREATE INDEX idx_job_discarded ON job_discarded(job_result_id);

-- +goose Down
DROP INDEX idx_job_discarded;
DROP TABLE job_discarded;
