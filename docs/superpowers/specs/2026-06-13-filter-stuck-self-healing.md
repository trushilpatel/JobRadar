# Filter Pipeline Stuck-Job Self-Healing

**Date:** 2026-06-13

## What was built

Seven root causes that allowed jobs to permanently accumulate as `reason='retry'` in `job_pipeline_ledger` were identified and fixed. The result is a self-healing filter pipeline that handles LLM quota exhaustion, transient NATS outages, and Redis infra errors without manual intervention.

## Root causes fixed

- **RC-1 (RC-header):** `retry_count` was reset to 1 on every sweeper cycle because `msg.DeliveryCount()` restarts from 1 after each sweeper re-publish. Fixed by stamping `X-Retry-Count` header on the NATS message and reading it in the filter worker via `getRetryCount()`.
- **RC-2 (lazy-publisher):** Sweeper was permanently disabled if NATS was down at scheduler startup. Fixed by `lazyPublisher` in `apps/scheduler/`, which connects on first use and evicts the connection on publish error.
- **RC-3 (payload-filter):** Sweeper called `FetchDueRetryJobs` which returned rows with null payload, causing publish panics. New `FetchDueRetryJobsWithPayload` filters for non-null payload and non-empty adapter.
- **RC-4 (ack-wait):** `ConsumerFilterWorkers` AckWait was 2 minutes — too tight for LLM processing. Extended to 10 minutes.
- **RC-5 (rpd-floor):** RPD quota exhaustion set `next_attempt_at` using a 30-second backoff, causing a hot spin for hours. Floor added: `filterRPDExhaustionRetryAfter = 4h`.
- **RC-6 (redis-infra):** Redis `Reserve` infra errors short-circuited the LLM router without trying remaining slots. Now wrapped as `ErrLLMLocalQuotaExceeded` so the router continues to the next slot.
- **RC-7 (escalation):** No circuit-breaker for rows that were never re-published. `EscalateStuckRetryJobs` runs each scheduler cycle and escalates `reason='retry'` rows older than 4 hours to `reason='error'`.

## Files changed

- `apps/filter/worker.go` — `getRetryCount`, RPD exhaustion 4h floor
- `apps/filter/worker_rc_test.go` — new tests for RC-1 and RC-5
- `apps/scheduler/lazy_publisher.go` — RC-2: lazy NATS reconnect
- `apps/scheduler/lazy_publisher_test.go` — 9 tests for lazy publisher
- `apps/scheduler/main.go` — always constructs lazy publisher
- `apps/scheduler/scheduler.go` — calls `EscalateStuckRetryJobs` in `runCycle`
- `apps/scheduler/interfaces.go` — `EscalateStuckRetryJobs` added
- `common/jetstream/publisher.go` — `PublishWithHeaders` on `Publisher` interface
- `common/jetstream/subjects.go` — `HeaderRetryCount` constant
- `common/jetstream/streams.go` — `ConsumerFilterWorkers` AckWait 2m → 10m
- `common/jetstream/testutil.go` — `NewTestMsgWithHeaders` test utility
- `common/metrics/metrics.go` — `SweeperStuckEscalatedTotal`, `SweeperNATSUnavailableTotal`
- `service/job/services/JobSeenLedgerRepository.go` — `FetchDueRetryJobsWithPayload`, `EscalateStuckRetryJobs`
- `service/job/services/JobService.go` — interface updated with new store methods
- `service/llm/services/LLMRateLimitService.go` — Redis infra error → `ErrLLMLocalQuotaExceeded`
- `service/scan/services/ScanSchedulerService.go` — sweeper uses `FetchDueRetryJobsWithPayload`, stamps `X-Retry-Count`, `EscalateStuckRetryJobs` impl
- `service/scan/services/ScanService.go` — `SchedulerService` interface updated

## Key design decisions

- Retry count is carried as a NATS message header (`X-Retry-Count`) across sweeper cycles; the filter reads it in preference to `DeliveryCount()-1` to preserve accumulated count.
- The sweeper leaves rows at `reason='retry'` while in-flight (no `ResetRetryToFetch` call); the filter transitions them on delivery.
- `EscalateStuckRetryJobs` uses `int64(threshold.Seconds()) * INTERVAL '1 second'` — the same PostgreSQL-safe pattern as `ResetStuck`.
