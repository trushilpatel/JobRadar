-- +goose Up
-- RUN AFTER NEW BINARY DEPLOY.
--
-- Third-pass cleanup of phantom job_pipeline_ledger rows with reason='fetch'
-- that have no matching jobs row. Migrations 066 and 069 swept earlier batches;
-- this pass sweeps any phantoms that accumulated during the deploy window of the
-- silent-failure fix (crawler reorder + filter Nak-on-missing-row + ledger
-- sentinels).
--
-- TIMING IS CRITICAL: this migration MUST run only AFTER the new crawler and
-- filter binaries are rolled out. If it runs before the rollout, the old
-- binaries keep leaking fresh phantoms into the freshly cleaned ledger and
-- re-inflate pipeline_pending in mv_pipeline_db_stats. Deploy runbook:
--   1. deploy the new image
--   2. wait for the crawler + filter pod rollout to complete
--   3. THEN run this migration (goose up)
--
-- After this migration, with the new binaries running (crawler inserts the
-- ledger row before publishing; filter Naks instead of creating a phantom), no
-- further reason='fetch' phantoms should form.

UPDATE job_pipeline_ledger
SET reason = 'error'
WHERE reason = 'fetch'
  AND NOT EXISTS (
      SELECT 1 FROM jobs WHERE jobs.job_post_id = job_pipeline_ledger.job_id
  );

REFRESH MATERIALIZED VIEW CONCURRENTLY mv_pipeline_db_stats;

-- +goose Down
-- Intentional no-op: the NATS messages that produced these rows are long gone;
-- restoring reason='fetch' would only re-inflate pipeline_pending without any
-- corresponding messages to reprocess. Same reasoning as 066 and 069.
