# Filter / pipeline silent-failure fix — implementation spec

Date: 2026-05-23
Branch: `fix/filter-pipeline-silent-failures`
Plan: `docs/superpowers/plans/2026-05-23-filter-pipeline-silent-failures.md`

## What shipped

Closed three classes of silent pipeline failure plus the duplicate-job-row
problem. The ledger is now the authority for transitions; every UPDATE surfaces
a typed sentinel when zero rows match, and publish/Ack ordering is made
consistent so NATS redelivery (or the next scan) is always the recovery path.

## Domain sentinels + observability

- `backend/common/errors/errors.go` — added `ErrLedgerRowMissing`
  (`LEDGER_ROW_MISSING`, 409), `ErrQueueRowMissing` (`QUEUE_ROW_MISSING`, 404),
  `ErrJobAlreadyExists` (`JOB_DUPLICATE`, 200 internal-only). Added a
  Code-based `ServiceError.Is` so `errors.Is` matches sentinels produced via
  `WithMessage`. Tests in `errors_test.go`.
- `backend/common/metrics/metrics.go` — new counters: `CrawlerPublishFailures`,
  `CrawlerScanAbortedReassigned`, `FilterPublishFailures`, `FilterLedgerMissing`,
  `EnricherLedgerMissing`, `EnricherJobsDuplicate`. Registered in `init()`.
- `backend/common/logger/fields.go` — added `FieldReason`.

## Repository layer

- `backend/service/job/services/JobSeenLedgerRepository.go` — `UpdateSeenReason`,
  `UpdateSeenReasonBatch`, `UpdateRetryBatch` return `ErrLedgerRowMissing` when
  `RowsAffected` falls short of the expected count.
- `backend/service/job/services/JobRepositoryService.go` — `InsertJob` uses
  `ON CONFLICT (job_post_id) DO NOTHING` and returns `ErrJobAlreadyExists` on a
  no-op insert.
- `backend/service/scan/services/ScanRepositoryService.go` — `Complete`,
  `Remove`, `UpdateHeartbeat` return `ErrQueueRowMissing` on zero rows;
  `ScanQueueStore.ResetStuck` now returns `(int, error)` (rows reclaimed). The
  admin `RemoveFromQueue` propagates `ErrQueueRowMissing` (renders HTTP 404).
- `backend/service/scan/services/ScanCrawlerQueueImplService.go` — the crawler
  `CrawlerQueueService.ResetStuck` signature matches `(int, error)`.

## Pipeline service tolerance

- `backend/service/job/services/JobPipelineImplService.go` — `SaveEnrichedJob`
  tolerates `ErrJobAlreadyExists` (skip snapshot + tech-tags, still mark ledger
  `saved`, increment `EnricherJobsDuplicate`) and `ErrLedgerRowMissing`
  (log WARN, increment `EnricherLedgerMissing`, return nil).

## Workers

- `backend/apps/crawler/engine.go` — `publishNewJobs` now `MarkSeenBatch`
  (reason `fetch`) for all new IDs BEFORE publishing, closing the
  crawler→filter race. Pre-publish ledger failure skips publish; per-job
  publish failure increments `CrawlerPublishFailures` and leaves the row at
  `fetch` for next-scan recovery. Non-USA gate preserved.
- `backend/apps/crawler/workers/base.go` — heartbeat aborts the scan (cancel
  ctx + `CrawlerScanAbortedReassigned` + skip `Complete`) on
  `ErrQueueRowMissing` from `UpdateHeartbeat`.
- `backend/apps/crawler/main.go` — adapts the `ResetStuck (int, error)` return.
- `backend/apps/filter/worker.go` — `publishDetail` returns an error;
  `processChunk` partitions relevant/pass-through jobs into published-OK vs
  publish-failed (`publishPartition`), marks only the published bucket
  `filtered`, and Naks publish-failed jobs (`FilterPublishFailures`, no ledger
  write). `markSeenOrNak` Naks the chunk on `ErrLedgerRowMissing`
  (`FilterLedgerMissing`) instead of Acking. `routeFailedChunk` Nak-failure now
  Terms and still records the retry ledger entry.
- `backend/apps/analyzer/worker.go` — `markError`/`markIrrelevant`/`markNonUSA`
  tolerate `ErrLedgerRowMissing` (WARN + `EnricherLedgerMissing`, non-fatal);
  `markLedgerRetryWithRetry` skips the inline retry when the row is permanently
  missing.

## Tests

New / updated unit + integration-style tests alongside each source file
(mirrors the plan's test plan #1–#25). The crawler→filter race regression
(#23) lives in `backend/apps/filter/worker_extra_test.go` and drives the real
sqlite-backed `SeenLedgerStore`: a message whose ledger row is missing Naks and
creates no phantom row; after the INSERT lands, the replay transitions to
`filtered`. `backend/common/jetstream/testutil.go` gained a `NakErr` knob for
the Nak-failure path.

## Migrations

- `071_jobs_dedupe_and_unique_job_post_id.sql` — dedupe keep-oldest per
  `job_post_id`, drop `idx_job_feed_job_post_id`, create `uq_jobs_job_post_id`.
  Run BEFORE deploy. One-way (cannot restore deleted rows).
- `072` — SKIPPED. Migration 037 already enforces
  `company_scan_queue_status_check CHECK (status IN ('queued','running'))`, so
  the defensive status-check migration is redundant.
- `073_cleanup_post_deploy_phantom_fetch_rows_v2.sql` — re-runs the 066/069
  phantom-`fetch` cleanup + MV refresh. RUN AFTER NEW BINARY DEPLOY (header).

## Deploy runbook

1. Run migration 071 (before deploy).
2. Deploy the new image; wait for crawler + filter pod rollout.
3. Run migration 073 (after rollout).
