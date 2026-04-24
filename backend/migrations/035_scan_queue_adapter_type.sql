-- +goose Up
ALTER TABLE company_scan_queue ADD COLUMN adapter_type TEXT NOT NULL DEFAULT '';

UPDATE company_scan_queue q
SET adapter_type = c.adapter_type
FROM companies c
WHERE q.company_id = c.id;

ALTER TABLE company_scan_queue
    ADD CONSTRAINT fk_scan_queue_adapter_type
    FOREIGN KEY (adapter_type) REFERENCES adapter_configs(adapter_type);

-- +goose Down
ALTER TABLE company_scan_queue DROP CONSTRAINT IF EXISTS fk_scan_queue_adapter_type;
ALTER TABLE company_scan_queue DROP COLUMN IF EXISTS adapter_type;
