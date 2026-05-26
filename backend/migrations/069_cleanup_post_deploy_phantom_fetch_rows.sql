-- +goose Up
-- Second-pass cleanup of phantom job_pipeline_ledger rows with reason='fetch'
-- that have no matching jobs row.
--
-- Migration 066 cleared the original phantoms produced by the pre-fix filter
-- worker (Ack-before-ledger ordering). That migration ran at 08:34 UTC on
-- 2026-05-23, but the new filter binary (commit 8c7581e) was not rolled out
-- until 15:35 UTC the same day. During the ~7h deploy gap the old buggy
-- binary kept running, leaking 228 fresh phantom rows (first_seen_at between
-- 10:44 and 12:41 UTC) into the freshly cleaned ledger and re-inflating
-- pipeline_pending in mv_pipeline_db_stats.
--
-- This migration re-runs the same UPDATE so the second batch is reclassified
-- as reason='error' (consistent with 066's resolution path) and refreshes
-- the MV so the "Awaiting Filter" dashboard count reflects reality.
--
-- After this migration, no further phantom rows should appear: the running
-- filter binary contains the 8c7581e fix (ledger write before Ack).

UPDATE job_pipeline_ledger
SET reason = 'error'
WHERE reason = 'fetch'
  AND NOT EXISTS (
      SELECT 1 FROM jobs WHERE jobs.job_post_id = job_pipeline_ledger.job_id
  );

REFRESH MATERIALIZED VIEW CONCURRENTLY mv_pipeline_db_stats;

-- +goose Down
-- Intentional no-op: the NATS messages that produced these rows are long
-- gone; restoring reason='fetch' would only re-inflate pipeline_pending
-- without any corresponding messages to reprocess. Same reasoning as 066.
