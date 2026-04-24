# Filter Worker Simplification — Design

Replace the filter worker's parallel LLM fan-out with serial per-chunk processing, a configurable inter-call sleep, per-chunk offset commits, ledger-visible retry state, and a concrete observability layer.

## Motivation

`apps/filter/worker.go` runs up to `FILTER_LLM_PARALLELISM` concurrent `ScoreBatch` calls per Kafka fetch, coordinated with a semaphore + `sync.WaitGroup`. The concurrency layer is redundant — LLM rate limiting already lives inside the scorer (slot/cooldown). Two other limitations worth fixing in the same pass:

1. The whole fetch is the commit unit → a crash mid-fetch replays every chunk already processed.
2. Retry state is invisible in the db — a job bouncing 1m→10m→1h shows `reason='fetch'` in `job_pipeline_ledger`.

The retry topic already acts as the safety net for `ScoreBatch` failures: failed chunks are republished to `raw.retry.Nm` topics, which re-deliver them to the main topic later. The main topic's offset can therefore advance past failed chunks safely.

## Scope boundaries

**Explicitly off-limits — do not touch the LLM layer.** `service/llm/` (slot/cooldown/rate-limit machinery) remains the sole owner of LLM pacing. Nothing in this refactor moves pacing into or near the LLM layer, now or later. The worker-level sleep is a pragmatic floor between LLM calls, independent of LLM rate-limit internals.

## Goals

- Serial LLM calls inside a fetch, paced by a fixed interval
- Preserve at-least-once delivery
- Shrink restart replay window from "entire fetch" to "one chunk"
- Retry state observable in `job_pipeline_ledger` (reason + count + next attempt)
- Concrete observability: tracing, metrics, structured logs
- No new Kafka consumer abstraction; one small method added to the existing wrapper

## Non-goals

- Changing batch size semantics (still `FILTER_BATCH_SIZE` jobs per `ScoreBatch` call)
- Changing retry ladder topology (tiers: 1m / 10m / 1h, same delays)
- Changing DLQ routing
- Changing `job_listings.filtered` publish semantics
- Changing the broker-level accumulation behavior (keeps `FILTER_FLUSH_TIMEOUT_SECONDS` → `FetchMaxWait`)
- Changing anything in `service/llm/`

## Design

### Worker flow

```
Run(ctx):
  consumer.PollBatch(ctx, processFetch)

processFetch(ctx, records):
  fetchID := new UUID
  ctx = log.With(ctx, fetch_id=fetchID)
  items := parseRecords(records)                 // []chunkItem — see type split below
  chunks := splitBySize(items, batchSize)

  for i, chunk := range chunks:
    if i > 0:
      clock.Sleep(ctx, llmCallInterval)          // ctx-aware, clock-injected
    processChunk(ctx, chunk, chunkIndex=i, chunkCount=len(chunks))
    if err := consumer.CommitRecords(ctx, chunk.records...); err != nil:
      log.Error("commit failed", err, ...)
      metrics.CommitFailures.Inc()
      // continue — next fetch will redeliver uncommitted records
  return nil
```

`processChunk` always completes its routing (either publishes relevant / updates ledger, or sends failures to retry/DLQ). Commit happens after each chunk regardless of whether `ScoreBatch` succeeded. Rare transient publish failures inside `routeFailedChunk` remain as today — logged, counter-incremented, not retried. The retry queue is the designed safety net for scoring failures; silent drop during routing is an accepted rare edge with an alertable metric.

### Type split — isolate Kafka leakage

Today `jobWithCtx` carries `ctx`, `job`, `retryCount`. For per-chunk commit we need the original `*kgo.Record`. Rather than polluting `jobWithCtx` with a Kafka pointer, introduce:

```go
// jobWithCtx stays domain-only: ctx + job + retryCount.
// chunkItem pairs a jobWithCtx with its source Kafka record for offset tracking.
type chunkItem struct {
    job    jobWithCtx
    record *kgo.Record
}
```

`splitBySize` now operates over `[]chunkItem`. A chunk exposes both `jobs()` (domain slice used for `ScoreBatch`/routing) and `records()` (the `*kgo.Record` slice used for `CommitRecords`).

### Per-chunk commit semantics

Chunks are processed in strict fetch order (`splitBySize` preserves the offset ordering from `PollFetches`). Kafka offset commits are per-partition max-offset semantics: committing chunk N implicitly commits chunks 1..N-1 on the same partition.

**Ordering is load-bearing.** A comment on the loop documents this. If a future change reintroduces parallel chunk processing, this commit strategy becomes unsafe (a fast later chunk could commit past an unfinished earlier one, silently skipping records).

### Interaction with `PollBatch` auto-commit

`Consumer.PollBatch` calls `CommitUncommittedOffsets` after the handler returns nil. With per-chunk `CommitRecords`, each chunk's records are committed as we go; the final `CommitUncommittedOffsets` is a no-op (nothing left uncommitted). On the empty-chunks path (`len(items) == 0`, e.g. all records malformed), `processFetch` returns nil and `PollBatch`'s trailing commit advances past the fetch normally.

### `routeFailedChunk` outcome shape

Today returns `[]string` (DLQ IDs only). New return:

```go
type routeOutcome struct {
    DLQed   []string           // successfully published to raw DLQ (ledger → reason='error')
    Retried []RetryLedgerEntry // successfully published to a retry tier (ledger → reason='retry')
    // Publish failures are counter-incremented and logged; not included in either slice.
}

type RetryLedgerEntry struct {
    JobID         string
    RetryCount    int       // the new retry_count after increment
    NextAttemptAt time.Time // UTC; computed once, used for both Kafka header and ledger row
}
```

### `NextAttemptAt` single source of truth

`routeFailedChunk` computes `nextAt := clock.Now().Add(tier.Delay)` **once per job** and passes the same value to both `BuildRetryHeaders(...)` (Kafka header) and `RetryLedgerEntry.NextAttemptAt` (ledger). The header and the ledger row are guaranteed to agree at write time. The retry consumer reads the header and remains authoritative for actual scheduling.

### `processChunk` outcome → ledger

After `ScoreBatch`:

- Relevant → publish to `job_listings.filtered` (unchanged). Publish failures logged (unchanged).
- Irrelevant → ledger `reason='llm_irrelevant'` via `UpdateSeenReasonBatch` (unchanged; does not touch `retry_count`/`next_attempt_at`).
- `ScoreBatch` error → `routeFailedChunk` returns `routeOutcome`. Ledger updates:
  - `outcome.DLQed`   → `reason='error'` via `UpdateSeenReasonBatch` (unchanged; does not touch retry columns).
  - `outcome.Retried` → `reason='retry'` + `retry_count` + `next_attempt_at` via new `UpdateRetryBatch` call.

`processChunk` does not return an error signal — commit happens after it returns.

**Partial-routing inconsistency window (documented):** if 48 of 50 jobs publish to retry and 2 fail, the ledger gets 48 `retry` rows, the 2 failures are logged + metric-incremented, and the chunk commits. The 2 failed jobs are lost from Kafka and their ledger state is stale (last known `reason='fetch'`). This matches current behavior. The new `filter_retry_publish_failures_total` counter makes the drop alertable.

### Ledger schema

New migration `033_add_retry_ledger_fields.sql`. Uses `NOT VALID` + `VALIDATE CONSTRAINT` to avoid the `ACCESS EXCLUSIVE` lock on constraint replacement:

```sql
-- +goose Up
ALTER TABLE job_pipeline_ledger
    ADD COLUMN retry_count     INT         NULL,
    ADD COLUMN next_attempt_at TIMESTAMPTZ NULL;

ALTER TABLE job_pipeline_ledger
    DROP CONSTRAINT job_pipeline_ledger_reason_check;

ALTER TABLE job_pipeline_ledger
    ADD CONSTRAINT job_pipeline_ledger_reason_check
    CHECK (reason IN ('fetch','llm_irrelevant','user_dismissed','error','retry'))
    NOT VALID;

ALTER TABLE job_pipeline_ledger
    VALIDATE CONSTRAINT job_pipeline_ledger_reason_check;

-- Partial index for "what's currently retrying?" queries.
CREATE INDEX idx_ledger_retry_due
    ON job_pipeline_ledger(next_attempt_at)
    WHERE reason = 'retry';

-- +goose Down
DROP INDEX IF EXISTS idx_ledger_retry_due;

-- Collapse any remaining retry rows to 'error' before re-tightening the CHECK,
-- so Down is safe regardless of in-flight retries.
UPDATE job_pipeline_ledger SET reason = 'error' WHERE reason = 'retry';

ALTER TABLE job_pipeline_ledger
    DROP CONSTRAINT job_pipeline_ledger_reason_check;

ALTER TABLE job_pipeline_ledger
    ADD CONSTRAINT job_pipeline_ledger_reason_check
    CHECK (reason IN ('fetch','llm_irrelevant','user_dismissed','error'));

ALTER TABLE job_pipeline_ledger
    DROP COLUMN next_attempt_at,
    DROP COLUMN retry_count;
```

Columns are populated only for rows where `reason='retry'`. They stay NULL elsewhere.

### `UpdateRetryBatch` SQL strategy

Per-row `retry_count` / `next_attempt_at` requires a single-round-trip multi-value update. Use `UPDATE ... FROM (VALUES ...)`:

```sql
UPDATE job_pipeline_ledger AS l
SET reason          = 'retry',
    retry_count     = v.retry_count,
    next_attempt_at = v.next_attempt_at
FROM (VALUES
    ($1, $2::int, $3::timestamptz),
    ($4, $5::int, $6::timestamptz),
    ...
) AS v(job_id, retry_count, next_attempt_at)
WHERE l.job_id = v.job_id;
```

One round trip, all entries updated with their own values. No CASE expressions, no per-row UPDATE in a transaction.

**Idempotency contract:** if the same `job_id` is routed twice in overlapping fetches (possible under rebalance), the last write wins. `retry_count` is monotonically advanced by the caller (`routeFailedChunk` reads current count from the header and increments before writing), so a re-execution of a prior routing writes the same value — no backward ratchet in normal flow.

### Ledger transitions

```
retry → llm_irrelevant  (ScoreBatch succeeds, job irrelevant → UpdateSeenReasonBatch)
retry → retry           (ScoreBatch fails again, retry_count++, next_attempt_at bumped → UpdateRetryBatch)
retry → error           (retry budget exhausted → DLQ → UpdateSeenReasonBatch)
```

Relevant jobs get no ledger update after successful scoring (unchanged). A relevant job that *was* previously retried will keep `reason='retry'` with its final `retry_count` frozen. Analyzer owns downstream state via `job_feed`. Dashboards that want "currently retrying" must filter `WHERE reason='retry'` — never use `retry_count > 0` as a proxy.

### Retry tier consumer interplay (existing, unchanged)

The filter binary runs three retry-tier consumers alongside the main worker (see `apps/filter/main.go` — `runRetryConsumer` for each of `raw.retry.1m`, `raw.retry.10m`, `raw.retry.1h`). Each consumer holds records, sleeps until `next_attempt_at`, and republishes to `job_listings.raw` preserving the `retry_count` header. This machinery is not modified.

Observability gap acknowledged: between "retry consumer republishes" and "main worker re-scores," the ledger row still shows the prior `next_attempt_at` which is now in the past. The job is still retrying, just between waypoints. Closing this gap is out of scope here.

### Shutdown

The inter-chunk sleep uses `select { case <-time.After(...): case <-ctx.Done(): return ctx.Err() }` (via the `clock` interface). On SIGTERM:

- Mid-sleep: loop exits with `ctx.Err()`; committed chunks stay committed; uncommitted tail replays.
- Mid-`ScoreBatch`: scorer returns ctx error → treated as a `ScoreBatch` failure → `routeFailedChunk` attempts to publish (may also fail with ctx cancellation, incrementing the publish-failure counter) → chunk committed.
- Mid-publish: publish error logged + counter, chunk still committed.

### Throughput ceiling

Per-partition throughput under this design:

```
throughput_per_partition = batch_size / (avg_score_batch_latency + llm_call_interval)
```

With `batch_size=50`, `llm_call_interval=5s`, and a typical `ScoreBatch` latency of ~2s, that's ~7 jobs/s per partition. Cluster throughput is `N × 7` for N partitions on `job_listings.raw`. If retry volume grows or main-topic traffic exceeds this ceiling, retry tier consumers can be extracted into their own binary as a follow-up — not part of this refactor.

## New Consumer method

```go
// common/kafka/consumer.go
func (c *Consumer) CommitRecords(ctx context.Context, records ...*kgo.Record) error {
    return c.client.CommitRecords(ctx, records...)
}
```

One method. No other consumer changes.

## Config

| Env var                            | Default | Purpose                                                   |
| ---------------------------------- | ------- | --------------------------------------------------------- |
| `FILTER_BATCH_SIZE`                | `50`    | Jobs per single `ScoreBatch` call (unchanged)             |
| `FILTER_FLUSH_TIMEOUT_SECONDS`     | `30`    | Broker `FetchMaxWait` — trickle accumulation (unchanged)  |
| `FILTER_LLM_CALL_INTERVAL_SECONDS` | `5`     | Sleep between successive `ScoreBatch` calls in one fetch (new) |
| `FILTER_LLM_PARALLELISM`           | —       | Removed                                                   |

`config.Config.Filter`: drop `LLMParallelism`, add `LLMCallIntervalSeconds int`.

## Ledger repository additions

`service/job/services/JobSeenLedgerRepository.go` (single interface `FilterLedger` — not split):

```go
type RetryLedgerEntry struct {
    JobID         string
    RetryCount    int
    NextAttemptAt time.Time
}

UpdateRetryBatch(ctx context.Context, entries []RetryLedgerEntry) error
```

Added to `seenLedgerStore` and the `FilterLedger` interface in `apps/filter/interfaces.go`. Implementation uses the `UPDATE ... FROM (VALUES ...)` pattern above via GORM raw SQL.

## Constants

`service/job/common/JobServiceConstants.go`:

```go
SeenReasonRetry = "retry"
```

## Observability

This section is load-bearing, not optional.

### OpenTelemetry spans

Hierarchy (parent → child):

```
filter.worker.processFetch      attrs: fetch.id, fetch.records, fetch.chunks, kafka.partition
├── filter.worker.processChunk  attrs: chunk.index, chunk.size, chunk.retry_count_max
│   ├── filter.worker.scoreBatch attrs: batch.size, outcome={relevant|mixed|irrelevant|error}
│   ├── filter.worker.routeFailed attrs: dlqed, retried, publish_failed  [error path only]
│   ├── job.store.UpdateRetryBatch | UpdateSeenReasonBatch (existing per-store spans)
│   └── filter.worker.commit    attrs: chunk.index, kafka.partition, kafka.offset
```

Use `common/tracing.Start` / `tracing.End`. Every span uses the chunk's ctx, which in turn comes from the fetch ctx with the fetch ID baked in. The known per-record trace-context drop (already present in `processFetch`) is not addressed here — it's called out so it doesn't silently return.

### Prometheus metrics

Add to `common/metrics`:

| Metric                                           | Type      | Labels                           | Measures                                       |
| ------------------------------------------------ | --------- | -------------------------------- | ---------------------------------------------- |
| `filter_chunk_duration_seconds`                  | Histogram | `outcome=relevant\|mixed\|irrelevant\|error` | Per-chunk score+route+publish duration |
| `filter_retry_publishes_total`                   | Counter   | `tier=1m\|10m\|1h`                | Successful retry-tier publishes               |
| `filter_retry_publish_failures_total`            | Counter   | `tier=1m\|10m\|1h`                | Silent-drop alert surface                      |
| `filter_dlq_publishes_total`                     | Counter   | `cause=exhausted\|validation`    | Successful DLQ publishes                      |
| `filter_commit_failures_total`                   | Counter   | (none)                           | `CommitRecords` errors (replay on next poll) |
| `filter_chunks_per_fetch`                        | Histogram | (none, buckets 1/2/4/8/16/32/64) | Fan-out size per fetch                         |

Repurpose existing `FilterLLMDuration`: label it per-chunk (not per-fetch). Keep existing `FilterJobsTotal{relevant|irrelevant|error}`.

### Structured log fields

Add to `common/logger/fields.go`:

```go
FieldFetchID       = "fetch_id"
FieldChunkIndex    = "chunk_index"
FieldChunkCount    = "chunk_count"
FieldRetryCount    = "retry_count"
FieldRetryTier     = "retry_tier"
FieldNextAttemptAt = "next_attempt_at"
FieldPartition     = "partition"
FieldOffset        = "offset"
```

`processFetch` mints a UUID per fetch and attaches it via `log.With(Str(FieldFetchID, fetchID))` so every descendant log line carries it (Loki query surface).

### Log statements — where and what

| Location                                  | Level | Fields                                                      |
| ----------------------------------------- | ----- | ----------------------------------------------------------- |
| `processFetch` entry                      | Info  | records, chunks, partition, first_offset, last_offset       |
| Inter-chunk sleep                         | Debug | sleep, chunk_index                                          |
| `CommitRecords` success                   | Debug | chunk_index, chunk_count, committed_offset, partition       |
| `CommitRecords` failure                   | Error | chunk_index, partition, err — loop continues                |
| `routeFailedChunk` retry publish failure  | Warn  | job_id, retry_count, retry_tier                             |
| `routeFailedChunk` summary                | Info  | retried_count, dlqed_count, publish_failed_count            |
| `UpdateRetryBatch` failure                | Error | chunk_index, entries_count, err — does not fail the loop    |
| `chunk scored` (existing, extend)         | Info  | fetch_id, chunk_index, chunk_count, batch, relevant, irrelevant |

## Files

| File                                                            | Change                                                                                        |
| --------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| `backend/migrations/033_add_retry_ledger_fields.sql`            | Additive migration: add `retry_count`, `next_attempt_at`; extend reason CHECK with `NOT VALID` + `VALIDATE`; partial index `idx_ledger_retry_due`. Safe Down that collapses retry rows to error first. |
| `backend/common/kafka/consumer.go`                              | Add `CommitRecords(ctx, records ...*kgo.Record) error`                                        |
| `backend/common/config/config.go` (and env loading)             | Drop `LLMParallelism`; add `LLMCallIntervalSeconds` (default `5`)                             |
| `backend/common/logger/fields.go`                               | Add new field constants (see Observability)                                                   |
| `backend/common/metrics/` (metrics registry)                    | Add new counters + histograms listed above                                                    |
| `backend/service/job/common/JobServiceConstants.go`             | Add `SeenReasonRetry = "retry"`                                                               |
| `backend/service/job/services/JobSeenLedgerRepository.go`       | Add `RetryLedgerEntry` + `UpdateRetryBatch` on interface + impl using `UPDATE ... FROM (VALUES ...)` |
| `backend/apps/filter/interfaces.go`                             | Extend single `FilterLedger` interface with `UpdateRetryBatch`                                |
| `backend/apps/filter/worker.go`                                 | Remove parallelism, WaitGroup, semaphore. Add `llmCallInterval` + `clock` interface field. Introduce `chunkItem` type (isolates Kafka from `jobWithCtx`). `processFetch` serial loop with per-chunk commit, fetch-ID correlation, clock-gated sleep. `routeFailedChunk` returns `routeOutcome{DLQed, Retried}` with `next_attempt_at` computed once. `processChunk` writes retry ledger. All spans, metrics, logs wired per Observability section. |
| `backend/apps/filter/main.go`                                   | Drop parallelism, pass `time.Duration(cfg.Filter.LLMCallIntervalSeconds) * time.Second`; wire `realClock{}` into the worker |
| `backend/apps/filter/worker_test.go`                            | Update constructor. Add fake clock. New tests (see Tests section).                            |
| `backend/apps/filter/CLAUDE.md`                                 | Rewrite batch flow: serial chunks, per-chunk commit, retry-reason ledger updates, observability anchors |
| `backend/common/config/CLAUDE.md`                               | Remove `FILTER_LLM_PARALLELISM`; add `FILTER_LLM_CALL_INTERVAL_SECONDS`; update `FILTER_FLUSH_TIMEOUT_SECONDS` description |
| `backend/CLAUDE.md` (root)                                      | Extend `job_pipeline_ledger.reason` list with `retry`; document new columns briefly           |

## Testing

### Clock injection

```go
type clock interface {
    Now() time.Time
    Sleep(ctx context.Context, d time.Duration) error  // ctx-aware; returns ctx.Err() on cancel
}

type realClock struct{}
func (realClock) Now() time.Time { return time.Now() }
func (realClock) Sleep(ctx context.Context, d time.Duration) error {
    select {
    case <-time.After(d):
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

Production wires `realClock{}`. Tests wire a fake that records sleep calls and returns immediately. The same `clock.Now()` flows through `routeFailedChunk` for deterministic `NextAttemptAt` assertions.

### Test cases

- `processFetch` with N scoreable chunks: fake clock records `N-1` sleep calls each equal to `llmCallInterval`; `CommitRecords` called N times in partition-offset order.
- `processFetch` with chunk whose `ScoreBatch` fails + retries publish OK: `UpdateRetryBatch` receives correct `retry_count` + `next_attempt_at`; `filter_retry_publishes_total{tier="1m"}` incremented by chunk size; chunk committed; next chunk runs.
- `processFetch` with chunk at exhausted retry budget: DLQ publish + `UpdateSeenReasonBatch(error)`; `filter_dlq_publishes_total{cause="exhausted"}` incremented.
- `processFetch` with `ErrLLMValidation`: all jobs DLQ'd regardless of retry count; `filter_dlq_publishes_total{cause="validation"}` incremented.
- `processFetch` with chunk mixing `retry_count=0` and `retry_count=3` under `LLMAllExhaustedError`: tier-eligible jobs routed to retry, exhausted ones DLQ'd in the same chunk.
- Ctx cancellation mid-sleep: fake clock returns `ctx.Err()`; next chunk NOT started; already-committed chunks stay committed.
- Ctx cancellation between `ScoreBatch` return and `CommitRecords` call: records NOT committed; next fetch redelivers.
- `CommitRecords` returns error: `filter_commit_failures_total` incremented; next chunk still runs; no crash.
- `UpdateRetryBatch` DB failure: Error logged; commit still happens; ledger stale but Kafka authoritative.
- Empty `items` slice (all records malformed): returns nil without panicking; `PollBatch` trailing commit advances past fetch.
- Partial-partition chunk: fetch contains records from two partitions, chunk spans both; `CommitRecords` commits max offset per partition; verified via mock consumer capturing partition→offset map.
- `UpdateRetryBatch` store test (against pgtest/testcontainers, NOT sqlmock): writes distinct `retry_count` + `next_attempt_at` per row in one call; idempotent on same job_id; does not corrupt rows for unrelated job_ids.

## Rollout

Single PR:

1. Migration (additive; `NOT VALID` avoids long lock). Safe to deploy against running system.
2. Logger field + metrics registry additions.
3. Repository + interface extension. Old callers compile unchanged.
4. Worker refactor, constant addition, config swap, clock wiring.
5. Wiring + tests + docs.

Deploy order: migration first (goose runs before binaries), then binaries. No downtime. `FILTER_LLM_PARALLELISM` env vars in deployment configs should be removed but are ignored if left behind.

## Open questions

- **Retry-tier consumer extraction threshold.** At what retry volume or retry-1h lag do we extract retry consumers into their own binary? Not required now, but the trigger condition should be agreed and captured as a follow-up issue before this ships.
- **Per-record trace-context propagation.** The existing `processFetch` drops per-record Kafka trace context when building `fetchCtx`. This refactor does not fix it but inherits it. Worth a separate follow-up.
