-- +goose Up
-- Widen the job_pipeline_ledger.reason CHECK to include 'saved', written by
-- the analyzer worker when a job is successfully persisted to the jobs table.
-- This distinguishes "awaiting enrichment" (reason='filtered') from
-- "enrichment complete" (reason='saved') in the ledger.

ALTER TABLE job_pipeline_ledger
    DROP CONSTRAINT IF EXISTS job_pipeline_ledger_reason_check;

ALTER TABLE job_pipeline_ledger
    ADD CONSTRAINT job_pipeline_ledger_reason_check
    CHECK (reason IN ('fetch','filtered','llm_irrelevant','user_dismissed','error','retry','non_usa_location','saved'))
    NOT VALID;

ALTER TABLE job_pipeline_ledger
    VALIDATE CONSTRAINT job_pipeline_ledger_reason_check;

-- +goose Down
-- Collapse saved rows back to fetch so the tighter CHECK can be reinstated.
UPDATE job_pipeline_ledger
SET reason = 'fetch'
WHERE reason = 'saved';

ALTER TABLE job_pipeline_ledger
    DROP CONSTRAINT IF EXISTS job_pipeline_ledger_reason_check;

ALTER TABLE job_pipeline_ledger
    ADD CONSTRAINT job_pipeline_ledger_reason_check
    CHECK (reason IN ('fetch','filtered','llm_irrelevant','user_dismissed','error','retry','non_usa_location'))
    NOT VALID;

ALTER TABLE job_pipeline_ledger
    VALIDATE CONSTRAINT job_pipeline_ledger_reason_check;
