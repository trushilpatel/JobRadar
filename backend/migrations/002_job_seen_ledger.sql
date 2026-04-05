-- +goose Up
CREATE TABLE job_seen_ledger (
    job_id        TEXT NOT NULL PRIMARY KEY,
    company_id    TEXT NOT NULL REFERENCES companies(id),
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reason        TEXT NOT NULL
                      CHECK(reason IN (
                          'fetch',
                          'llm_irrelevant',
                          'user_dismissed',
                          'error'
                      ))
);

CREATE INDEX idx_seen_company ON job_seen_ledger(company_id);

-- +goose Down
DROP INDEX idx_seen_company;
DROP TABLE job_seen_ledger;
