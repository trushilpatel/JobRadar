-- +goose Up

-- job_snapshot: one lightweight row per enriched job, written by the enricher.
-- Never deleted. User actions (saved/applied/dismissed) are backfilled here.
-- This decouples historical analytics from operational job rows.
CREATE TABLE job_snapshot (
    id                 TEXT        PRIMARY KEY,  -- same UUID as job.id
    company_id         TEXT        NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    snapshotted_at     DATE        NOT NULL DEFAULT CURRENT_DATE,

    -- Structured location (extracted by LLM at enrichment time)
    country            TEXT,
    state              TEXT,
    remote             BOOLEAN     NOT NULL DEFAULT FALSE,

    -- Visa / clearance
    sponsorship        BOOLEAN     NOT NULL DEFAULT FALSE,
    requires_citizen   BOOLEAN     NOT NULL DEFAULT FALSE,
    security_clearance BOOLEAN     NOT NULL DEFAULT FALSE,

    -- Role shape
    employ_type        TEXT,
    experience_years   INTEGER,
    tech_stack         JSONB,
    role_category      TEXT        CHECK (role_category IN (
                           'backend', 'frontend', 'fullstack',
                           'devops', 'ml', 'data', 'other'
                       )),

    -- User outcome (null until user acts; updated by API handlers)
    user_action        TEXT        CHECK (user_action IN ('saved', 'applied', 'dismissed')),
    actioned_at        TIMESTAMPTZ
);

CREATE INDEX idx_snapshot_company ON job_snapshot(company_id);
CREATE INDEX idx_snapshot_date    ON job_snapshot(snapshotted_at DESC);
CREATE INDEX idx_snapshot_country ON job_snapshot(country);

-- +goose Down
DROP INDEX IF EXISTS idx_snapshot_country;
DROP INDEX IF EXISTS idx_snapshot_date;
DROP INDEX IF EXISTS idx_snapshot_company;
DROP TABLE job_snapshot;
