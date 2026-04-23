# Enricher (Analyzer) Simplification — Design

Apply the filter refactor pattern to the enricher (`apps/analyzer`). Replace Stage 2's unbounded goroutine fan-out with a serial per-group loop, pace LLM calls with a configurable interval, commit Kafka offsets per group, and write retry state to `job_pipeline_ledger` for observability.

## Motivation

`apps/analyzer/worker.go` today runs Stage 2 via `enrichAllGroups`, which spawns one goroutine per group and waits on a `sync.WaitGroup`. There's **no cap on concurrent `EnrichBatch` calls** — a fetch of 100 records with `ENRICHER_LLM_BATCH_SIZE=5` fans out 20 simultaneous LLM calls. The LLM slot/cooldown layer rate-limits at its own level, but the worker-level fan-out duplicates that responsibility and makes failure reasoning harder.

Three concrete limitations the refactor addresses:

1. **Unbounded Stage 2 parallelism** — redundant given the LLM layer already rate-limits.
2. **Whole-fetch commit unit** — a crash mid-fetch replays every record, even groups already saved.
3. **Stage 2 (LLM) retry state invisible in db** — a job bouncing through analyzer Stage-2 retry tiers shows `reason='fetch'` (or whatever it had coming out of the filter) in `job_pipeline_ledger`. The `retry_count`/`next_attempt_at` columns (added in migration 033 for the filter) are reused.

**Scope narrowing:** Only Stage 2 (`EnrichBatch`) failures write `reason='retry'` to the ledger. Stage 1 (HTTP fetch) failures still route to the retry tier topic (Kafka headers carry retry_count + next_attempt_at as today), but do not touch the ledger. This keeps `reason='retry'` semantically consistent across filter and analyzer: "LLM-layer failure, retrying." HTTP transport retries stay in Kafka only — dashboards querying "stuck in LLM retry" are not polluted by fetch failures.

## Scope boundaries

**Explicitly off-limits — do not touch the LLM layer.** `service/llm/` (slot/cooldown/rate-limit machinery) remains the sole owner of LLM pacing. Nothing in this refactor moves pacing into or near the LLM layer, now or later.

**Also off-limits:**

- Stage 1 HTTP fetch concurrency model (still parallel, still capped by `ENRICHER_FETCH_CONCURRENCY`, still per-adapter rate-limited).
- Analyzer retry tier topology (`raw.retry.1m|10m|1h` equivalents — the `AnalyzerRetryTiers()` helper in `common/kafka/retry.go`, and the `runRetryConsumer` in `apps/analyzer/main.go`).
- DLQ routing and `sendToDLQ` / `markError` / `markIrrelevant` behavior.
- `pipeline.SaveEnrichedJob` — `job_feed` AND `job_analytics` writes remain untouched.
- Drop rules (`dropJob`).

## Goals

- Serial LLM calls inside a fetch, paced by a fixed interval.
- Preserve at-least-once delivery.
- Shrink restart replay window from "entire fetch" to "one group".
- Retry state observable in `job_pipeline_ledger` (reason + count + next attempt) for both Stage 1 and Stage 2 failures that route to retry.
- Concrete observability: spans, metrics, structured logs with fetch correlation.
- No new Kafka consumer abstraction (`CommitRecords` already exists from the filter refactor).
- No schema migration (migration 033 already added the columns).

## Non-goals

- Changing `ENRICHER_LLM_BATCH_SIZE` default (stays 5).
- Changing `ENRICHER_FLUSH_TIMEOUT_SECONDS` default (stays 120).
- Changing the fetch-all-then-enrich staging (Stage 1 still runs to completion before Stage 2 starts).
- Altering the `PipelineService` interface beyond adding one new method.
- Touching `service/llm/`.

## Design

### Worker flow

```
Run(ctx):
  consumer.PollBatch(ctx, processFetch)

processFetch(ctx, records):
  fetchID := new UUID
  ctx = log.With(ctx, fetch_id=fetchID)

  // Stage 1 — unchanged: parallel HTTP fetches up to fetchConc,
  // per-adapter rate-limited. Per-record failures route to retry tier
  // or DLQ via handleFailure (now writing retry ledger state too).
  fetched := fetchAll(ctx, records)

  if len(fetched) == 0:
    return nil   // PollBatch's trailing commit advances past the fetch.

  groups := chunkFetched(fetched, llmBatchSize)   // preserves offset order
  observe metrics.EnricherGroupsPerFetch(len(groups))

  // Stage 2 — now serial with pacing + per-group commits.
  for i, group := range groups:
    if i > 0:
      clock.Sleep(ctx, llmCallInterval)    // ctx-aware, clock-injected
    processGroup(ctx, group, fetchID, i, len(groups))
    if err := consumer.CommitRecords(ctx, group.records...); err != nil:
      log.Error("commit failed", err, ...)
      metrics.EnricherCommitFailuresTotal.Inc()
      // continue — next PollBatch redelivers uncommitted records

  return nil
```

`processGroup` is the renamed + extended `enrichGroup` — same internals (build inputs, `EnrichBatch`, distribute results, route failures, per-job save/drop/mark), now plus span + metric hooks + ledger retry updates driven by `handleFailure`.

### Stage 1 / Stage 2 commit interaction

Stage 1 runs to completion for all records in the fetch *before* Stage 2 starts. Jobs that fail Stage 1 are routed to retry/DLQ inside `fetchAll` via `handleFailure` (same call path as today, now with retry-ledger writes). Their offsets remain uncommitted at this point.

Stage 2 commits per-group. Kafka's per-partition max-offset commit semantics mean: when Stage 2 group N commits, all records with lower offsets on the same partition are implicitly committed — including Stage 1 failures that fell between this group's records and the prior group's commit.

Consequence: Stage 1 failures only "materialize" as committed once their surrounding Stage 2 group commits. This matches the filter pattern: the retry topic owns the failed job's lifecycle, main topic offset advances past it.

**Ordering is load-bearing.** `chunkFetched` must preserve offset order for Stage 2 groups. Today `fetchAll` returns `[]fetchedJob` "in arbitrary order" (per its docstring) because fetches complete at different times. The refactor must **sort by (partition, offset) before grouping** so group commits don't skip records. Add a sort step between `fetchAll` and `chunkFetched`.

### `fetchedJob` — carry the Kafka record

Today `fetchedJob` holds `value`, `retryCount`, `ctx` but not the `*kgo.Record` itself. For per-group commits the record pointer is required. Two options:

- **Add `record *kgo.Record` to `fetchedJob`.** Simplest. The worker package already lives close to Kafka types.
- **Introduce a separate `groupItem` type** (like the filter's `chunkItem`) pairing a `fetchedJob` with its record.

**Chosen:** add `record *kgo.Record` to `fetchedJob`. The struct is already worker-package-internal; no domain leakage concern like the filter's `jobWithCtx`. The `value []byte` field can then be derived from `record.Value` and potentially dropped (subject to current callers).

### Retry-ledger writes

`PipelineService` is NOT extended. Instead, introduce a narrow worker-facing ledger interface following the filter precedent:

```go
// backend/apps/analyzer/interfaces.go (NEW FILE)
type EnricherLedger interface {
    // For Stage 2 retry-tier routing only.
    UpdateRetryBatch(ctx context.Context, entries []jobsvc.RetryLedgerEntry) error
    // Delegated through PipelineService today via MarkLedgerError / MarkLedgerIrrelevant.
    // Expose the two concrete ledger operations the worker actually performs,
    // keeping ISP over the broad PipelineService.
}
```

The worker receives an `EnricherLedger` alongside `PipelineService`. The concrete implementation is `seenLedgerStore` (already exposes `UpdateRetryBatch`). No new SQL, no new store method, no change to `PipelineService`.

**Call site:** inside `worker.handleFailure`, immediately after a successful retry-tier `PublishWithHeaders`, **only on the Stage 2 path** (see the `stage` argument threaded below):

```go
nextAttemptAt := w.clock.Now().Add(tier.Delay)
headers := kafka.BuildRetryHeaders(retryCount+1, nextAttemptAt)

if err := w.publisher.PublishWithHeaders(ctx, tier.Topic, value, headers); err != nil {
    metrics.EnricherRetryPublishFailures.WithLabelValues(tier.Name(), stage).Inc()
    log.Error("publish retry failed", err, logger.Str(logger.FieldRetryTier, tier.Name()))
    return
}
metrics.EnricherRetryPublishesTotal.WithLabelValues(tier.Name(), stage).Inc()

if stage == "enrich" {
    entry := jobsvc.RetryLedgerEntry{JobID: job.ID, RetryCount: retryCount+1, NextAttemptAt: nextAttemptAt}
    if err := w.markLedgerRetryWithRetry(ctx, entry); err != nil {
        metrics.EnricherRetryLedgerFailuresTotal.Inc()
        log.Error("mark ledger retry failed after retry", err, ...)   // non-fatal
    }
}
log.Info("scheduled retry", ...)
```

`markLedgerRetryWithRetry` attempts `UpdateRetryBatch([entry])` up to twice (original + one inline retry after ~100ms) before emitting the failure counter. Kafka-tier publish is authoritative; a stale ledger row is a brief observability gap, not a correctness issue.

**`NextAttemptAt` single source of truth:** computed once via `w.clock.Now().Add(tier.Delay)` and passed to both `BuildRetryHeaders` and the ledger entry.

Validation errors and exhausted-retry DLQ paths continue to call `markError` via `PipelineService` (`reason='error'`) — unchanged. Filter's rule carries over: non-`retry` reason writes do not touch `retry_count`/`next_attempt_at`.

### Clock injection

Same pattern as filter:

```go
type clock interface {
    Now() time.Time
    Sleep(ctx context.Context, d time.Duration) error
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

`Worker` gets a `clock clock` field (lowercase, package-local), `main.go` wires `realClock{}`, tests inject a `fakeClock` that records sleeps and returns immediately. The same clock is used by `handleFailure` for `nextAttemptAt`, making retry ledger writes deterministic in tests.

### Shutdown

Inter-group sleep uses the ctx-aware clock. On SIGTERM:

- **Mid-sleep:** `clock.Sleep` returns `ctx.Err()`; loop exits; committed groups stay committed; uncommitted tail replays.
- **Mid-`EnrichBatch` (critical correctness):** scorer returns ctx error → `routeBatchFailure` runs, per-job `handleFailure` short-circuits on `ctx.Err()` → nothing published. If the loop then committed this group, records would be lost (never routed, offsets advanced). To prevent this, `processGroup` returns a boolean `routed bool` indicating whether all jobs reached a terminal routing (saved, dropped, retry-published, DLQ'd). **If `routed == false`, the loop returns `nil` WITHOUT committing this group.** Kafka redelivers on next poll; previously-committed groups (lower offsets) stay committed. This is the ctx-cancel-mid-fetch safety rule, matching filter's ctx-between-ScoreBatch-and-commit behavior.
- **Mid-publish inside `handleFailure`:** publish error logged + counter; `processGroup` records the job as "not routed" → `routed = false` → loop skips commit → Kafka redelivers.

**Loop shape:**

```go
for i, group := range groups {
    if i > 0 {
        if err := w.clock.Sleep(ctx, w.llmCallInterval); err != nil {
            return nil   // ctx cancelled; uncommitted tail replays
        }
    }
    routed := w.processGroup(ctx, group, fetchID, i, len(groups))
    if !routed {
        return nil   // skip commit; replay whole tail including this group
    }
    if err := w.consumer.CommitRecords(ctx, group.records()...); err != nil {
        metrics.EnricherCommitFailuresTotal.Inc()
        log.Error("commit failed", err, ...)
        // continue — next PollBatch will redeliver uncommitted records
    }
}
```

### Throughput ceiling

Per-partition steady-state throughput:

```
throughput_per_partition = batch_size / (avg_enrich_batch_latency + llm_call_interval)
```

With `batch_size=5`, `llm_call_interval=5s`, and typical `EnrichBatch` latency ~3–8s: ~0.4–0.6 jobs/sec/partition. Cluster throughput is `N × that`.

Heavy backlog after restart: a fetch of 100 records produces 20 groups, total ~100–260s of wall time. Per-group commit means only the last few are vulnerable to another restart. If retry volume grows beyond steady-state throughput, retry-tier consumer extraction is the documented follow-up (same as filter).

## Config

| Env var                              | Default | Purpose                                                   |
| ------------------------------------ | ------- | --------------------------------------------------------- |
| `ENRICHER_LLM_BATCH_SIZE`            | `5`     | Jobs per `EnrichBatch` call (unchanged)                   |
| `ENRICHER_FLUSH_TIMEOUT_SECONDS`     | `120`   | Broker `FetchMaxWait` (unchanged)                         |
| `ENRICHER_FETCH_CONCURRENCY`         | `15`    | Stage 1 parallel HTTP cap (unchanged)                     |
| `ENRICHER_DEFAULT_RPS`               | `2`     | Per-adapter rate limit (unchanged)                        |
| `ENRICHER_DEFAULT_BURST`             | `3`     | Per-adapter burst (unchanged)                             |
| `ENRICHER_JITTER_MS`                 | `200`   | Adapter jitter (unchanged)                                |
| `DISCARD_JOB_WITH_EXP_GT`            | `0`     | Drop threshold (unchanged)                                |
| `ENRICHER_LLM_CALL_INTERVAL_SECONDS` | `5`     | Sleep between Stage 2 `EnrichBatch` calls (**new**)       |

`config.Config.Enricher`: add `LLMCallIntervalSeconds int`. Env is loaded via the existing `getEnvInt` helper.

## Observability

### OpenTelemetry spans

Hierarchy:

```
analyzer.worker.processFetch      attrs: fetch.id, fetch.records, fetch.fetched, fetch.groups, kafka.partition
├── analyzer.worker.fetchAll      attrs: records, fetched, failed                       [NEW span]
│   └── analyzer.worker.fetchOne  attrs: job_id, adapter, wait_ms, outcome              [NEW — per-record child of fetchAll]
├── analyzer.worker.processGroup  attrs: group.index, group.size, group.retry_count_max [renamed from enrichGroup]
│   ├── analyzer.worker.enrichBatch attrs: batch.size, outcome={ok|error|missing_results} [NEW]
│   ├── analyzer.worker.routeBatchFailure attrs: retried, dlqed, publish_failed         [error path only]
│   └── job.store.UpdateRetryBatch existing per-store span                               [existing]
└── analyzer.worker.commit        attrs: group.index, kafka.partition, kafka.offset     [NEW]
```

Use `common/tracing.Start` / `tracing.End`. Fetch ctx with fetch ID baked in flows to all descendants.

### Prometheus metrics

Add to `common/metrics`:

| Metric                                     | Type      | Labels                                                         | Measures                                |
| ------------------------------------------ | --------- | -------------------------------------------------------------- | --------------------------------------- |
| `enricher_group_duration_seconds`          | Histogram | `outcome=saved\|dropped\|error\|mixed`                         | Per-group enrich+route+save duration     |
| `enricher_retry_publishes_total`           | Counter   | `tier=1m\|10m\|1h`, `stage=fetch\|enrich`                      | Successful retry-tier publishes         |
| `enricher_retry_publish_failures_total`    | Counter   | `tier=1m\|10m\|1h`, `stage=fetch\|enrich`                      | Silent-drop alert surface               |
| `enricher_retry_ledger_failures_total`     | Counter   | (none)                                                         | `MarkLedgerRetry` give-up after inline retry |
| `enricher_dlq_publishes_total`             | Counter   | `cause=exhausted\|validation\|fetch_failed\|llm_result_missing`| Successful DLQ publishes                |
| `enricher_commit_failures_total`           | Counter   | (none)                                                         | `CommitRecords` errors                  |
| `enricher_groups_per_fetch`                | Histogram | (none, buckets `[1,2,4,8,16,32,64]`)                           | Fan-out size per fetch                   |

Keep existing `EnricherLLMDuration` (observed per `EnrichBatch`) and `EnricherJobsTotal{saved|dropped|error|dlq}` as-is.

### Group outcome label — deterministic precedence

`enricher_group_duration_seconds{outcome}` requires a single label per group. Apply in order; first match wins:

1. Any job in the group was routed via `handleFailure` (retry or DLQ) → **`error`**
2. Any job saved + any job dropped → **`mixed`**
3. All jobs saved → **`saved`**
4. All jobs dropped → **`dropped`**

### `tierHumanName` lifted to shared Kafka package

Currently only in `apps/filter/worker.go`. Lift to `common/kafka/retry.go` as a method on `RetryTier`:

```go
func (t RetryTier) Name() string { ... }  // returns "1m" | "10m" | "1h" | "" for unknowns
```

Update filter's `worker.go` to call `tier.Name()` and remove the local helper. Analyzer uses the same method for metric labels and log fields.

### Structured log fields

Reuse the logger field constants added in the filter refactor (`FieldFetchID`, `FieldChunkIndex`, `FieldChunkCount`, `FieldRetryCount`, `FieldRetryTier`, `FieldNextAttemptAt`, `FieldPartition`, `FieldOffset`). No new constants.

`processFetch` mints a UUID and attaches via `log.With(Str(FieldFetchID, fetchID))` so every descendant log line carries it.

### Log statements

| Location                                    | Level | Fields                                                           |
| ------------------------------------------- | ----- | ---------------------------------------------------------------- |
| `processFetch` entry                        | Info  | records, first_offset, last_offset, partition, fetch_id          |
| Inter-group sleep                           | Debug | sleep, chunk_index                                               |
| `CommitRecords` success                     | Debug | chunk_index, chunk_count, committed_offset, partition             |
| `CommitRecords` failure                     | Error | chunk_index, partition, err — loop continues                     |
| `handleFailure` retry publish failure       | Warn  | job_id, retry_count, retry_tier                                  |
| `handleFailure` retry published             | Info  | job_id, retry_count, retry_tier, next_attempt_at (existing, extend with retry_tier/next_attempt_at fields) |
| `MarkLedgerRetry` failure                   | Error | job_id, retry_count, err — does not fail the worker              |
| `processFetch` summary (existing, extend)   | Info  | fetch_id, records, fetched, batches, fetch_duration, enrich_duration |

## Files

| File                                                         | Change                                                                                                                 |
| ------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------- |
| `backend/common/config/config.go` (+ env loading)            | Add `Enricher.LLMCallIntervalSeconds int` (env `ENRICHER_LLM_CALL_INTERVAL_SECONDS`, default `5`)                      |
| `backend/common/metrics/metrics.go`                          | Add the seven new `enricher_*` metrics listed above with the new `stage` label on retry counters                       |
| `backend/common/kafka/retry.go`                              | Add method `(t RetryTier) Name() string` returning `"1m"`/`"10m"`/`"1h"` for matching delays; `""` otherwise           |
| `backend/apps/filter/worker.go`                              | Replace local `tierHumanName(topic)` with `tier.Name()`; remove the helper. Keep everything else untouched.           |
| `backend/apps/analyzer/interfaces.go` (**new file**)         | New `EnricherLedger` interface exposing `UpdateRetryBatch` (the narrow worker-facing ledger contract)                   |
| `backend/apps/analyzer/worker.go`                            | Remove `enrichAllGroups`. Add `clock`, `llmCallInterval`, `ledger EnricherLedger` fields. Extend `fetchedJob` with `record *kgo.Record`; drop the `value []byte` field (use `record.Value`). Rewrite `processFetch` to serial loop with per-group commit + ctx-aware sleep + `routed bool` skip rule. Sort `[]fetchedJob` by `(partition, offset)` via `sort.SliceStable` before grouping (comment explaining why). Extract a helper `sortFetched(s []fetchedJob)`. Rename `enrichGroup` → `processGroup` (returns `routed bool`). Introduce `WorkerOptions` struct; refactor `NewWorker(opts WorkerOptions) *Worker`. |
| `backend/apps/analyzer/enricher.go`                          | `processGroup` gains span `analyzer.worker.enrichBatch` around the `EnrichBatch` call; emit `enricher_group_duration_seconds{outcome}` at end using the precedence rule. `distributeResults` returns `allRouted bool` so `processGroup` can signal ctx-cancel-mid-batch via the new shutdown rule. Use scoped `fetchLog` logger for `fetch_id`/`chunk_index` on every line (no more `w.log` in hot paths). |
| `backend/apps/analyzer/fetcher.go`                           | `fetchedJob` field change propagates. Add span `analyzer.worker.fetchAll` wrapping the whole stage and per-record `analyzer.worker.fetchOne` with `adapter`, `wait_ms`, `outcome` attrs. Use scoped `fetchLog` for all log lines (no `w.log` in hot paths). Stage 1 parallelism and rate-limiting untouched. |
| `backend/apps/analyzer/worker.go` — `handleFailure`          | Gain a `stage string` parameter (`"fetch"` or `"enrich"`). Compute `nextAttemptAt` once from `clock.Now().Add(tier.Delay)`. Retry publish success: `EnricherRetryPublishesTotal{tier.Name(), stage}.Inc()`; if `stage == "enrich"`, call `markLedgerRetryWithRetry` on the new `EnricherLedger` (one inline retry ~100ms, then give up + `EnricherRetryLedgerFailuresTotal.Inc()`). Retry publish failure: `EnricherRetryPublishFailures{tier.Name(), stage}.Inc()`. Extend "scheduled retry" log with `retry_tier`, `next_attempt_at`, `stage`. |
| `backend/apps/analyzer/worker.go` — `sendToDLQ`              | Increment `enricher_dlq_publishes_total{cause}` with `cause = validation | exhausted | fetch_failed | llm_result_missing` derived from the error and call path. Existing `markError` + `EnricherJobsTotal{"dlq"}` unchanged. |
| `backend/apps/analyzer/main.go`                              | Switch to `NewWorker(opts WorkerOptions)` with all deps. Wire `realClock{}`, `time.Duration(cfg.Enricher.LLMCallIntervalSeconds) * time.Second`, inject the ledger (from the same `seenLedgerStore` used by the pipeline). |
| `backend/apps/analyzer/worker_test.go`                       | Add `fakeClock` + `fakeLedger` (records `UpdateRetryBatch` calls) + compile-time assertion `var _ jobsvc.PipelineService = (*fakePipelineService)(nil)`. Update constructor calls to `WorkerOptions`. Add the new test cases below. |
| `backend/apps/analyzer/CLAUDE.md`                            | Rewrite Two-stage flow: Stage 1 unchanged + parallel HTTP; Stage 2 serial with `ENRICHER_LLM_CALL_INTERVAL_SECONDS` pacing and per-group commits. Add retry-ledger note (Stage 2 only writes `reason='retry'`). Add observability section. Pointer to design spec. |
| `backend/common/config/CLAUDE.md`                            | Enricher worker table: add `ENRICHER_LLM_CALL_INTERVAL_SECONDS` row.                                                  |

No migration. No new logger field constants. `PipelineService` interface unchanged.

## Tests

All use `fakeClock` for deterministic pacing + `nextAttemptAt` assertions. Add compile-time check `var _ jobsvc.PipelineService = (*fakePipelineService)(nil)` at the bottom of the test file.

- `processFetch` with N scoreable groups: `fakeClock.sleeps` has N-1 entries each equal to `llmCallInterval`; `CommitRecords` called N times in partition-offset order.
- Single-group fetch: no sleep (`fakeClock.sleeps` empty); one `CommitRecords` call.
- `processFetch` with fetch of 0 records after Stage 1 failures: returns nil; no sleeps; no `CommitRecords` calls (PollBatch's trailing commit handles it).
- Stage 1 failure for one job: `handleFailure` routes to retry tier; `EnricherRetryPublishesTotal{tier="1m", stage="fetch"}` incremented; **no** `UpdateRetryBatch` call (scope: only Stage 2 writes retry ledger).
- Stage 1 failure at exhausted budget: DLQ publish + `markError`; `enricher_dlq_publishes_total{cause="fetch_failed"}` incremented.
- Stage 2 `EnrichBatch` failure whole-group: each job routed via `handleFailure` using its own `retryCount`; `UpdateRetryBatch` called with one entry per retried job containing correct `retry_count` + `next_attempt_at`; `EnricherRetryPublishesTotal{tier, stage="enrich"}` incremented for each.
- Stage 2 group with mixed retry_count (group contains jobs with retry_count=0 and retry_count=3): tier-eligible routed to retry, exhausted DLQ'd; `UpdateRetryBatch` only contains the tier-eligible subset.
- Missing LLM result for a job in the batch: treated as transient Stage 2 failure → retry ledger write (same as other Stage 2 failures). At exhausted budget → DLQ with `cause="llm_result_missing"`.
- Validation error (`ErrLLMValidation`): all jobs straight to DLQ with `cause="validation"`; no retry ledger writes.
- Ctx cancel mid-sleep: loop returns without starting next group; already-committed groups stay committed.
- Ctx cancel mid-`EnrichBatch` (**critical**): `processGroup` returns `routed=false`; loop skips commit; fetch redelivers. No records lost.
- Ctx cancel between `EnrichBatch` return and `CommitRecords` call: commit not attempted; fetch redelivers.
- `CommitRecords` returns error: `enricher_commit_failures_total` incremented; next group still runs.
- `UpdateRetryBatch` transient failure then success on inline retry: one retry attempt made, zero `EnricherRetryLedgerFailuresTotal` increments.
- `UpdateRetryBatch` fails twice (inline retry exhausted): `EnricherRetryLedgerFailuresTotal` incremented; worker continues; Kafka already published (authoritative).
- Partial-partition group (records from two partitions in one group): `CommitRecords` commits max offset per partition; verified via fake consumer capturing partition→offset map. Group spanning partitions is explicitly allowed.
- Offset ordering: unit-test `sortFetched` with deliberately shuffled input spanning two partitions; assert output is stable-sorted by `(partition, offset)`. Plus end-to-end `processFetch` test where fake Stage-1 injects jobs in non-offset order and the final commits observed are in per-partition ascending offset order.
- Group outcome label precedence: parameterized table test with all four labels (`error`, `mixed`, `saved`, `dropped`) exercising the rule.
- `llmCallInterval=0`: `realClock.Sleep(ctx, 0)` returns immediately; fetch still processes groups serially with no wall-clock wait.

## Rollout

Single PR. No migration. No data changes.

1. Add pipeline interface method + impl.
2. Add config + metric registrations.
3. Worker refactor + tests + docs.
4. `main.go` wiring update.

Deploy: binary only. Old binary writing via the prior parallel path is compatible with new consumer-group offset state (per-group commits are strict subsets of fetch-level commits).

## Open questions

- **Retry-tier consumer extraction threshold.** Inherits filter's open question — if retry volume grows beyond steady-state throughput of the main worker, extract retry consumers. Same trigger condition applies to analyzer.
- **Stage 1 commit granularity.** Current design commits only via Stage 2 groups. If Stage 2 never runs (all records fail Stage 1), the trailing `CommitUncommittedOffsets` in `PollBatch` covers it. If Stage 1 is expensive and Stage 2 rarely runs, per-Stage-1-record commit could reduce replay; not proposed now because it complicates two-stage commit ordering.
