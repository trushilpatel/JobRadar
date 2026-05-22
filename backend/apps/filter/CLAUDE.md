# apps/filter/ — Relevance filter worker

Consumes `jobs.raw.*` (NATS JetStream), scores jobs in batches via LLM, routes relevant jobs to `jobs.filtered.*`, marks irrelevant / error / retry jobs in `job_pipeline_ledger`.

Design spec: `docs/superpowers/specs/2026-04-22-filter-simplification-design.md`.

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
4. Each chunk's messages are acked via `Msg.Ack()` after `processChunk` returns — success or routed-failure
5. Relevant → publish to `jobs.filtered.<adapter>` (NATS JetStream `JOBS_FILTERED` stream)
6. Irrelevant → mark `reason='llm_irrelevant'` in ledger via `UpdateSeenReasonBatch`
7. `ScoreBatch` error → `routeFailedChunk` routes to retry tier or DLQ (`jobs.dlq`). Retried jobs get `UpdateRetryBatch` (sets `reason='retry'`, `retry_count`, `next_attempt_at`); DLQ'd jobs get `UpdateSeenReasonBatch('error')`

## Error handling rules

- `ScoreBatch` failures never re-queue directly to the main topic. Failed chunks are republished to retry tiers; exhausted retries go to DLQ.
- **Silent-drop window:** if a publish inside `routeFailedChunk` fails transiently, the job is logged, `filter_retry_publish_failures_total` increments, and the chunk commits anyway. The retry topic is the designed safety net for `ScoreBatch` failures — do not introduce an in-loop retry.
- Reprocess stuck `error` rows by deleting the ledger rows manually.

## Ledger retry state

- `reason='retry'` is a **transient in-flight** state written only by the filter.
- Columns `retry_count`, `next_attempt_at` are populated **only** while `reason='retry'`; NULL elsewhere.
- `UpdateRetryBatch` (on `FilterLedger`) is the single path that writes retry state — per-row values in one round trip.
- Transitions: `retry → llm_irrelevant` (scored irrelevant), `retry → retry` (fails again, count++ / next_attempt_at bumped), `retry → error` (retry budget exhausted → DLQ).
- A relevant job that was previously retried keeps its final `retry_count` frozen — `jobs` is downstream authority. Dashboards filtering "currently retrying" must use `reason='retry'`, **never** `retry_count > 0`.

## Observability

- **Spans:** `filter.worker.processFetch` → `filter.worker.processChunk` → (`filter.worker.scoreBatch` | `filter.worker.routeFailed`) → `filter.worker.commit`. Started/ended via `platform/tracing`.
- **Metrics** (in `platform/metrics`): `filter_chunk_duration_seconds`, `filter_retry_publishes_total`, `filter_retry_publish_failures_total`, `filter_dlq_publishes_total`, `filter_commit_failures_total`, `filter_chunks_per_fetch`. Existing `FilterLLMDuration` and `FilterJobsTotal{relevant|irrelevant|error}` retained (former now labeled per-chunk).
- **Logs:** `processFetch` mints a UUID `fetch_id` attached via `log.With(...)` so every descendant line carries it. Additional field constants: `chunk_index`, `chunk_count`, `retry_count`, `retry_tier`, `next_attempt_at`, `partition`, `offset`.

## LLM dependency

Depends on `service/llm/services.JobsRelevanceFilter` — never on `*geminiScorer` directly. Worker-level sleep is independent of the LLM layer's slot/cooldown pacing.
