# apps/filter/ — Relevance filter worker

Consumes `jobs.raw.*` (NATS JetStream), scores jobs in batches via LLM, routes relevant jobs to `jobs.filtered.*`, marks irrelevant / error / retry jobs in `job_pipeline_ledger`.

Design spec: `docs/superpowers/specs/2026-05-23-filter-pipeline-silent-failures.md`.

## Key files

| File            | Role                                                                                                              |
| --------------- | ----------------------------------------------------------------------------------------------------------------- |
| `main.go`       | Wiring — NATS consumer/publisher, LLM scorer, ledger store, clock                                                 |
| `worker.go`     | Serial per-chunk processing: `chunkItem`, `clock` interface, `routeOutcome`, `processFetch`, observability wiring |
| `interfaces.go` | `FilterLedger` — narrow ledger interface the worker depends on (`UpdateSeenReasonBatch`, `UpdateRetryBatch`)      |

## Batch flow

1. `Consumer.FetchBatch` pulls messages from the `JOBS_RAW` stream (accumulated up to `FILTER_FLUSH_TIMEOUT_SECONDS`)
2. The fetch is split into chunks of exactly `FILTER_BATCH_SIZE` jobs (order-preserving)
3. Chunks are processed **serially**. Between consecutive `ScoreBatch` calls the worker sleeps `FILTER_LLM_CALL_INTERVAL_SECONDS` (default `5s`), ctx-aware via the injected `clock`
4. A chunk's messages are acked via `Msg.Ack()` only after the ledger write for that bucket succeeds. If `UpdateSeenReasonBatch`/`UpdateRetryBatch` returns `ErrLedgerRowMissing` (the crawler INSERT hasn't committed yet — see the race note below), the affected messages are **Nak'd** instead of Acked, so NATS redelivers them once the row exists. `filter_ledger_missing_total` increments.
5. Relevant → `publishDetail` publishes to `jobs.filtered.<adapter>` (`JOBS_FILTERED` stream) and **returns an error**. Only jobs that published OK get `UpdateSeenReasonBatch('filtered')` + Ack; a publish failure Naks the message with **no ledger write** (`filter_publish_failures_total` increments) so it redelivers — the ledger never says `filtered` for a job that isn't in `JOBS_FILTERED`.
6. Irrelevant → mark `reason='llm_irrelevant'` in ledger via `UpdateSeenReasonBatch`
7. `ScoreBatch` error → `routeFailedChunk` routes to retry tier or DLQ (`jobs.dlq`). Retried jobs get `UpdateRetryBatch` (sets `reason='retry'`, `retry_count`, `next_attempt_at`); DLQ'd jobs get `UpdateSeenReasonBatch('error')`. If a Nak itself fails, the job is still recorded as `retry` and the message Term'd (never silently dropped).

## Crawler → filter race (why missing rows happen)

The crawler writes the `reason='fetch'` ledger row **before** publishing to NATS, so by the time the filter consumes a message the row normally exists. If a redelivery or a slow commit means the row isn't there yet, the filter's ledger UPDATE returns `ErrLedgerRowMissing` and the filter Naks rather than Acking — this is what prevents the old phantom-`fetch` bug (where a silent-nil UPDATE + Ack left a row stuck at `fetch` forever). Never "fix" a missing-row by Acking anyway.

## Error handling rules

- `ScoreBatch` failures → `routeFailedChunk` issues `msg.Nak()` so NATS BackOff handles redelivery on the configured back-off schedule. No publish to a retry topic — NATS manages redelivery timing.
- `UpdateRetryBatch` records `next_attempt_at` in the ledger for dashboard visibility only; its failure is non-fatal to message disposition.
- Exhausted jobs (retry count >= max) → `msg.Term()` + publish to `jobs.dlq` via `publishToDLQ`. If the DLQ publish itself fails, the message is still Term'd (prevents endless redelivery of an exhausted job).
- Reprocess stuck `error` rows by deleting the ledger rows manually. Stranded `reason='fetch'` rows (job entered the filter but never reached a terminal verdict — e.g. a NATS restart mid-flight) are now reclaimed automatically: the scheduler's `ReclaimStuckFetchRows` sweep DELETEs `fetch` rows older than 1h each cycle so the next crawl re-drives them. No manual cleanup needed for `fetch`.

## Ledger retry state

- `reason='retry'` is a **transient in-flight** state written only by the filter.
- Columns `retry_count`, `next_attempt_at` are populated **only** while `reason='retry'`; NULL elsewhere.
- `UpdateRetryBatch` (on `FilterLedger`) is the single path that writes retry state — per-row values in one round trip.
- Transitions: `retry → llm_irrelevant` (scored irrelevant), `retry → retry` (fails again, count++ / next_attempt_at bumped), `retry → error` (retry budget exhausted → DLQ).
- A relevant job that was previously retried keeps its final `retry_count` frozen — `jobs` is downstream authority. Dashboards filtering "currently retrying" must use `reason='retry'`, **never** `retry_count > 0`.

## Observability

- **Spans:** `filter.worker.processFetch` → `filter.worker.processChunk` → (`filter.worker.scoreBatch` | `filter.worker.routeFailed`). Started/ended via `platform/tracing`.
- **Metrics** (in `common/metrics`): `filter_chunk_duration_seconds`, `filter_retry_publishes_total`, `filter_retry_publish_failures_total`, `filter_dlq_publishes_total`, `filter_commit_failures_total`, `filter_chunks_per_fetch`, `filter_publish_failures_total{subject}` (publish to `jobs.filtered.*` failed → Nak), `filter_ledger_missing_total{reason}` (ledger UPDATE hit `ErrLedgerRowMissing` → Nak for redelivery). Existing `FilterLLMDuration` and `FilterJobsTotal{relevant|irrelevant|error}` retained (former now labeled per-chunk).
- **Logs:** `processFetch` mints a UUID `fetch_id` attached via `log.With(...)` so every descendant line carries it. Additional field constants: `chunk_index`, `chunk_count`, `retry_count`, `retry_tier`, `next_attempt_at`, `partition`, `offset`.

## LLM dependency

Depends on `service/llm/services.JobsRelevanceFilter` — never on `*geminiScorer` directly. Worker-level sleep is independent of the LLM layer's slot/cooldown pacing.

## Adding a new LLM outcome

1. Add a new `routeOutcome` constant in `worker.go`
2. Handle it in the `processChunk` switch statement
3. Write the corresponding ledger update via `FilterLedger`
4. Add a Prometheus metric in `common/metrics/` if it's a new event type
5. Add a test case in `worker_test.go` covering the new branch
