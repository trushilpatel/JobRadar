-- +goose Up
-- Widen the job_pipeline_ledger.reason CHECK to include 'non_usa_location',
-- written by the filter worker when a job is rejected before LLM scoring
-- because its location is unambiguously non-USA.
ALTER TABLE job_pipeline_ledger
    DROP CONSTRAINT IF EXISTS job_pipeline_ledger_reason_check;

ALTER TABLE job_pipeline_ledger
    ADD CONSTRAINT job_pipeline_ledger_reason_check
    CHECK (reason IN ('fetch','llm_irrelevant','user_dismissed','error','retry','non_usa_location'))
    NOT VALID;

ALTER TABLE job_pipeline_ledger
    VALIDATE CONSTRAINT job_pipeline_ledger_reason_check;

-- +goose Down
-- Collapse any non_usa_location rows back to 'llm_irrelevant' so the tighter
-- CHECK can be reinstated without losing data. The semantic distinction is
-- preserved well enough for rollback (both indicate a relevance rejection).
UPDATE job_pipeline_ledger
SET reason = 'llm_irrelevant'
WHERE reason = 'non_usa_location';

ALTER TABLE job_pipeline_ledger
    DROP CONSTRAINT job_pipeline_ledger_reason_check;

ALTER TABLE job_pipeline_ledger
    ADD CONSTRAINT job_pipeline_ledger_reason_check
    CHECK (reason IN ('fetch','llm_irrelevant','user_dismissed','error','retry'));
