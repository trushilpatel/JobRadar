# Stuck-fetch ledger reclaimer (self-healing pipeline recovery)

## Problem

A job enters the filter when the crawler writes a `reason='fetch'` row to
`job_pipeline_ledger` (before publishing to NATS). On infrastructure disruption
mid-flight — a NATS restart during a large burst, a pod crash, an OOM — a message
can die before the filter writes a terminal verdict (`filtered` / `llm_irrelevant`
/ `non_usa_location` / `error` / `retry`), stranding the row at `reason='fetch'`.

The dedup check `HasSeenBatch` (`backend/service/job/services/JobSeenLedgerRepository.go`)
matches purely on `job_id` with **no `reason` or `removed_at` filter**, so any
existing row — including a stranded `fetch` — marks the job "seen forever." The
job is then silently lost: future crawls dedup it out, and the only recovery was a
manual `DELETE` (previously documented in `backend/apps/filter/CLAUDE.md`).

Observed in production: 441 rows from a single company stranded during a
1,209-company re-crawl burst when NATS restarted once.

## Fix

A self-healing reclaimer that **DELETEs** stranded `fetch` rows on a schedule,
mirroring the existing `ResetStuck` / `EscalateStuckRetryJobs` sweep pattern.
DELETE (not escalate-to-`error`) is required: because dedup ignores `reason`,
escalating would keep the job permanently deduped. Removing the row clears the
block so the next scheduled crawl re-discovers a still-live job and re-drives it
through the filter.

- Store: `SeenLedgerWriter.ReclaimStuckFetchRows(ctx, threshold, limit)` — DELETE of
  `reason='fetch' AND removed_at IS NULL AND first_seen_at < NOW()-threshold`,
  bounded by `LIMIT` via an `IN (SELECT … LIMIT ?)` subquery. Returns
  `RowsAffected`; as a count-based sweep it intentionally does **not** return
  `ErrLedgerRowMissing` on 0 rows (documented divergence from the targeted-write
  rule). `backend/service/job/services/JobSeenLedgerRepository.go`.
- Service: `schedulerService.ReclaimStuckFetchRows(ctx)` — nil-guard, tracing span,
  module consts `stuckFetchThreshold = 1h` / `stuckFetchLimit = 100`, metric + INFO
  log on n>0. `backend/service/scan/services/ScanSchedulerService.go`.
- Loop: invoked once per `runCycle`, immediately after `ResetStuck`, continue-on-error
  (metric label `reclaim_stuck_fetch`). `backend/apps/scheduler/scheduler.go`.
- Contracts: method added to `SeenLedgerWriter`, scan `SchedulerService`/`Service`
  (+ `BaseService` pass-through), scan REST no-op terminal, and the app-level
  `SchedulerService` interface.
- Metric: `SchedulerStuckFetchReclaimedTotal` (`backend/common/metrics/metrics.go`).

Threshold is a 1h safety floor — ≫ normal fetch→verdict latency (seconds–minutes)
and well beyond NATS `AckWait × MaxDeliver` redelivery plus retry backoff, so
legitimately in-flight rows are never deleted. `retry` is a separate reason, so the
`fetch`-only predicate already excludes the retry ladder.

## Tests

`backend/service/job/tests/JobSeenLedgerReclaim_test.go` (store: NoRows / SkipsNonFetch
/ BuildsQuery, at parity with the sibling `EscalateStuckRetryJobs` suite),
`backend/service/scan/tests/sweeper_service_test.go` (service: delegates / returns
count / error-propagates / metric / nil no-op), `backend/apps/scheduler/scheduler_test.go`
(loop: called each cycle / error continues / logs on reclaim).

Gates: golangci-lint clean, race-clean, per-package coverage ≥95%
(job/services 95.1%, scan/services 95.7%, scheduler 96.3%, metrics 100%).

## Follow-ups

- The exact-row predicate behavior (threshold boundary, `removed_at` guard, LIMIT,
  post-delete `HasSeenBatch=false`) is verified by inspection but not by an executing
  assertion — SQLite (unit tier) cannot run `NOW() - INTERVAL`. This matches the
  accepted sibling convention; a `//go:build integration` Postgres test would close it
  once a DB-integration harness exists (none currently in the backend unit tree).
- The upstream trigger (NATS restarting under a one-shot full-fleet re-crawl burst on
  the resource-constrained node) is a separate infra concern; the reclaimer makes the
  pipeline resilient to it regardless of cause.
