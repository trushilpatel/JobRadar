-- +goose Up
-- Cleanup: fix phantom ledger rows with reason='fetch' that have no matching jobs row.
--
-- Root cause: the filter worker was Acking NATS messages before calling
-- UpdateSeenReasonBatch, so a crash between the Ack and the ledger write left
-- rows stuck at reason='fetch' even though the jobs had already been routed
-- through the filter. These phantom rows inflate the pipeline_pending count in
-- mv_pipeline_db_stats.
--
-- This migration transitions those phantom rows to reason='error' so they are
-- clearly distinguishable from genuinely pending jobs and the dashboard stat
-- reflects reality. reason='error' is already used for jobs the filter could
-- not process, so ops workflows for inspecting stuck rows apply here too.

UPDATE job_pipeline_ledger
SET reason = 'error'
WHERE reason = 'fetch'
  AND NOT EXISTS (
      SELECT 1 FROM jobs WHERE jobs.job_post_id = job_pipeline_ledger.job_id
  );

-- Refresh the materialized view so pipeline_pending and pipeline_filtered
-- reflect the corrected ledger data immediately.
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_pipeline_db_stats;

-- +goose Down
-- Intentional no-op: the NATS messages that produced these rows are gone;
-- restoring reason='fetch' would only re-inflate the phantom pipeline_pending
-- count without any corresponding NATS messages to reprocess. Operators who
-- need to investigate specific rows should query job_pipeline_ledger directly
-- using the audit trail in job_pipeline_ledger.first_seen_at / updated_at.
