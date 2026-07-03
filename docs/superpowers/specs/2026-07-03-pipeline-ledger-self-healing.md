# Pipeline Ledger Self-Healing

**Date:** 2026-07-03

## What was built

Two blind spots in the scheduler's ledger reclaimer were fixed, making the filter and enricher pipeline stages self-healing against permanently stuck rows.

### Root causes fixed

1. **`ReclaimStuckFetchRows` ignored removed jobs** — the SQL had `AND removed_at IS NULL`, so rows where the crawler had marked a job as removed from the careers page were never cleaned up. These rows blocked re-processing indefinitely via `HasSeenBatch`.

2. **No reclaimer for `reason='filtered'` rows** — jobs that passed the filter but whose enricher NATS message expired or was lost accumulated in the ledger forever with no cleanup path.

### Changes

- `backend/service/job/services/JobSeenLedgerRepository.go`: removed `AND removed_at IS NULL` from `ReclaimStuckFetchRows`; added `ReclaimStuckFilteredRows` (same DELETE pattern for `reason='filtered'`). Both methods accept `olderThan time.Time` (Go-computed cutoff) instead of a duration, making the SQL SQLite-compatible for unit tests.
- `backend/service/job/services/JobService.go`: updated `SeenLedgerWriter` interface with new signatures and `ReclaimStuckFilteredRows`.
- `backend/service/scan/services/ScanSchedulerService.go`: added `ReclaimStuckFilteredRows` method with 1h threshold, 500-row limit, metric increment, and structured log.
- `backend/service/scan/services/ScanService.go`: added method to `SchedulerService` interface and `BaseService` pass-through.
- `backend/service/scan/services/ScanRepositoryService.go`: added no-op implementation.
- `backend/apps/scheduler/interfaces.go` and `scheduler.go`: wired `ReclaimStuckFilteredRows` into `runCycle` with isolated error handling (never gates `EnqueueDue`).
- `backend/common/metrics/metrics.go`: added `SchedulerStuckFilteredReclaimedTotal` counter.

### Test coverage

- `backend/service/job/tests/JobSeenLedgerReclaim_test.go`: success-path and error-path tests for the updated `ReclaimStuckFetchRows` (including `removed_at IS NOT NULL` rows).
- `backend/service/job/tests/JobSeenLedgerFilteredReclaim_test.go`: new file; success-path and error-path tests for `ReclaimStuckFilteredRows`.
- `backend/service/scan/tests/sweeper_service_test.go`: service-layer tests for `ReclaimStuckFilteredRows` on `schedulerService`.
- `backend/apps/scheduler/scheduler_test.go`: tick-loop tests for the new `runCycle` call.

### Coverage gate result

`service/job/services`: 95.1% — passes ≥95% threshold.
