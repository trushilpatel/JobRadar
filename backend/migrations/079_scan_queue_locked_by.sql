-- +goose Up
-- NULL = claimed by legacy direct-DB crawler (unchanged behavior).
-- Non-null = claimed by orchestrator on behalf of a crawler node.
ALTER TABLE company_scan_queue
    ADD COLUMN locked_by UUID REFERENCES crawler_nodes(id) ON DELETE SET NULL;

CREATE INDEX idx_csq_locked_by ON company_scan_queue (locked_by)
    WHERE locked_by IS NOT NULL;

-- +goose Down
DROP INDEX idx_csq_locked_by;
ALTER TABLE company_scan_queue DROP COLUMN locked_by;
