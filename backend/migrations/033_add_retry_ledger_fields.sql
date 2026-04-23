-- +goose Up
ALTER TABLE job_pipeline_ledger
    ADD COLUMN retry_count     INT         NULL,
    ADD COLUMN next_attempt_at TIMESTAMPTZ NULL;

ALTER TABLE job_pipeline_ledger
    DROP CONSTRAINT job_pipeline_ledger_reason_check;

ALTER TABLE job_pipeline_ledger
    ADD CONSTRAINT job_pipeline_ledger_reason_check
    CHECK (reason IN ('fetch','llm_irrelevant','user_dismissed','error','retry'))
    NOT VALID;

ALTER TABLE job_pipeline_ledger
    VALIDATE CONSTRAINT job_pipeline_ledger_reason_check;

-- Partial index for "what's currently retrying?" queries.
CREATE INDEX idx_ledger_retry_due
    ON job_pipeline_ledger(next_attempt_at)
    WHERE reason = 'retry';

-- +goose Down
DROP INDEX IF EXISTS idx_ledger_retry_due;

-- Collapse any remaining retry rows to 'error' before re-tightening the CHECK,
-- so Down is safe regardless of in-flight retries.
UPDATE job_pipeline_ledger SET reason = 'error' WHERE reason = 'retry';

ALTER TABLE job_pipeline_ledger
    DROP CONSTRAINT job_pipeline_ledger_reason_check;

ALTER TABLE job_pipeline_ledger
    ADD CONSTRAINT job_pipeline_ledger_reason_check
    CHECK (reason IN ('fetch','llm_irrelevant','user_dismissed','error'));

ALTER TABLE job_pipeline_ledger
    DROP COLUMN next_attempt_at,
    DROP COLUMN retry_count;
