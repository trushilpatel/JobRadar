-- +goose Up
CREATE TABLE company_scan_queue (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id  TEXT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    status      TEXT NOT NULL DEFAULT 'queued',
    source      TEXT NOT NULL DEFAULT 'scheduler',
    queued_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at  TIMESTAMPTZ
);

-- Enforces at most one active entry per company at the DB level.
-- ON CONFLICT DO NOTHING in Enqueue relies on this index.
CREATE UNIQUE INDEX uq_company_scan_queue_active
    ON company_scan_queue(company_id)
    WHERE status IN ('queued', 'processing');

-- +goose Down
DROP TABLE company_scan_queue;
