-- +goose Up
CREATE TABLE job (
    id              TEXT PRIMARY KEY,
    job_post_id     TEXT NOT NULL,
    company_id      TEXT NOT NULL REFERENCES companies(id),

    title           TEXT NOT NULL,
    location        TEXT,
    remote          BOOLEAN NOT NULL DEFAULT FALSE,
    url             TEXT NOT NULL,
    department      TEXT,
    employ_type     TEXT
                        CHECK(employ_type IN (
                            'full_time', 'part_time', 'contract', 'internship'
                        )),

    years_exp_min   INTEGER,
    years_exp_max   INTEGER,
    tech_stack      JSONB,
    salary_range    JSONB,
    llm_summary     TEXT,
    other_criteria  JSONB,

    status          TEXT NOT NULL DEFAULT 'new'
                        CHECK(status IN ('new', 'saved', 'applied')),
    saved           BOOLEAN NOT NULL DEFAULT FALSE,
    notes           TEXT,

    added_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_job_company ON job(company_id);
CREATE INDEX idx_job_status  ON job(status);
CREATE INDEX idx_job_saved   ON job(saved);
CREATE INDEX idx_job_added   ON job(added_at DESC);

-- +goose Down
DROP INDEX idx_job_added;
DROP INDEX idx_job_saved;
DROP INDEX idx_job_status;
DROP INDEX idx_job_company;
DROP TABLE job;
