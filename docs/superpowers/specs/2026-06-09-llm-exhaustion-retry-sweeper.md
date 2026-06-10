# LLM Exhaustion Retry Sweeper — Spec

**Date:** 2026-06-09

## What shipped

A production-grade retry path for LLM quota exhaustion in the filter and analyzer workers, plus a scheduler-driven sweeper that re-publishes deferred jobs when their backoff expires.

## Key files

### Infrastructure

- `backend/migrations/080_add_ledger_payload_adapter.sql` — adds `payload JSONB NULL` and `adapter VARCHAR NULL` to `job_pipeline_ledger`
- `backend/common/jetstream/streams.go` — updated `ConsumerFilterWorkers` and `ConsumerAnalyzerWorkers` to `MaxDeliver:3, BackOff:[30s, 2m]`
- `backend/common/metrics/metrics.go` — 5 new counters: `FilterLLMExhaustionTotal`, `EnricherLLMExhaustionTotal`, `SweeperRetriesPublishedTotal`, `SweeperRetriesEscalatedTotal`, `SweeperRetriesFailedTotal`
- `backend/common/logger/fields.go` — added `FieldExhaustedCount`, `FieldSweeperEnabled`

### Data layer

- `backend/service/job/models/JobSeenLedger.go` — added `RetryCount *int`, `NextAttemptAt *time.Time`, `Payload *json.RawMessage`, `Adapter *string`
- `backend/service/job/services/JobSeenLedgerRepository.go` — 4 new store methods: `UpdateRetryBatchWithPayload`, `FetchDueRetryJobs`, `ResetRetryToFetch`, `EscalateRetryToError`; `FetchDueRetryJobs` uses `CURRENT_TIMESTAMP` (not `NOW()`) for SQLite test compatibility
- `backend/service/job/services/JobService.go` — `SeenLedgerWriter` interface extended with the 4 new methods

### Filter worker

- `backend/apps/filter/interfaces.go` — `FilterLedger` extended with `UpdateRetryBatchWithPayload`
- `backend/apps/filter/worker.go` — `LLMAllExhaustedError` path: Term message, write payload+adapter to ledger; `filterBackOff=[30s,2m]`, `filterMaxRetries=2`

### Analyzer worker

- `backend/apps/analyzer/interfaces.go` — `EnricherLedger` extended with `UpdateRetryBatchWithPayload`
- `backend/apps/analyzer/worker.go` — `LLMAllExhaustedError` path on `enrich` stage: Term, write payload; `analyzerBackOff=[30s,2m]`, `analyzerMaxRetries=2`; fetch-stage exhaustion falls through to normal Nak path

### Scheduler sweeper

- `backend/service/scan/services/ScanSchedulerService.go` — `RetryDueJobs` implementation on `schedulerService`; `SweeperPublisher` interface; `NewSchedulerServiceWithSweeper` constructor; ordering invariant: `ResetRetryToFetch` before `Publish`
- `backend/service/scan/services/ScanService.go` — `RetryDueJobs` added to `SchedulerService`, `Service` interfaces, and `BaseService` delegation
- `backend/service/scan/services/ScanRepositoryService.go` — no-op `RetryDueJobs` for REST decorator chain
- `backend/apps/scheduler/scheduler.go` — `RetryDueJobs` call in `runCycle` before `EnqueueDue`
- `backend/apps/scheduler/interfaces.go` — `RetryDueJobs` added to local `SchedulerService` interface
- `backend/apps/scheduler/main.go` — NATS connect + sweeper wiring; graceful degradation if NATS unavailable
- `backend/common/config/config.go` — `SWEEPER_MAX_RETRIES` env var (default 10)

### Tests

- `backend/service/job/tests/JobSeenLedgerPayload_test.go` — 9 unit tests for new store methods
- `backend/service/scan/tests/sweeper_service_test.go` — 18 tests for `RetryDueJobs`: no-op paths, empty batch, fetch error, under-budget publish, escalation, per-job isolation, ordering invariant, payload passthrough, metrics

## Design decisions

**Exhaustion vs infra error paths are separate.** LLM exhaustion uses Term+ledger payload so the sweeper can re-publish after the backoff. Infra errors use Nak so NATS BackOff handles redelivery timing. These are distinguished by `stderrors.As(err, &exhausted)`.

**Ordering invariant:** DB ledger write completes successfully before any NATS Term or Publish call. On publish failure the sweeper re-writes the retry row with a 1-minute delay.

**Sweeper degrades gracefully.** If NATS is unavailable at scheduler startup, the sweeper is disabled (no-op `RetryDueJobs`); scan scheduling continues unaffected.

**`CURRENT_TIMESTAMP` over `NOW()`** in `FetchDueRetryJobs` for SQLite test compatibility while remaining semantically correct on PostgreSQL.
