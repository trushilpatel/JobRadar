-- +goose Up
ALTER TABLE job_pipeline_ledger
    ADD COLUMN payload JSONB NULL,
    ADD COLUMN adapter VARCHAR NULL;

-- +goose Down
ALTER TABLE job_pipeline_ledger
    DROP COLUMN payload,
    DROP COLUMN adapter;
