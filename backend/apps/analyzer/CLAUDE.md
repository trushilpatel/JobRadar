# apps/analyzer/ — Job post analyzer (enricher)

Consumes `job_listings.filtered` (Kafka), fetches full job page content with per-adapter rate limiting, runs LLM enrichment in batches, writes final rows to `job_feed` and `job_analytics`.

## Key files

| File           | Role                                                                              |
| -------------- | --------------------------------------------------------------------------------- |
| `main.go`      | Wiring — Kafka consumer, LLM enricher, pipeline service, rate limiter             |
| `worker.go`    | Top-level orchestration — `Run`, `processFetch`, retry/DLQ routing                |
| `fetcher.go`   | Stage 1 — parallel HTTP fetches with per-adapter rate limit; FullContent passthrough |
| `enricher.go`  | Stage 2 — LLM `EnrichBatch` per group, drop check, save                           |
| `ratelimit.go` | Per-adapter token bucket (lazy-created, default RPS + burst + jitter)             |
| `techtag/`     | Normalizes and deduplicates raw `tech_stack` strings from LLM output              |

## Two-stage flow per Kafka fetch

```
PollBatch (FetchMaxWait = ENRICHER_FLUSH_TIMEOUT_SECONDS)
   ↓
Stage 1 — parallel HTTP fetches (cap = ENRICHER_FETCH_CONCURRENCY)
   - FullContent set?      → fullContentToResponse()  (no HTTP)
   - FullContent unset?    → adapter rate-limit Wait → ContentFetcher / HTTP fallback
   - per-job failure       → handleFailure → retry/DLQ
   ↓
Stage 2 — group successful (job, content) into chunks of ENRICHER_LLM_BATCH_SIZE
   - EnrichBatch(group)    → []EnrichResult, attributed by job_id
   - missing job_id        → routed as transient failure for that job
   - per-job: drop check   → save or markIrrelevant / markError
   - whole batch fails     → routeBatchFailure → each job through retry ladder
   ↓
WaitGroup ensures all stages finish before Kafka offset commit
```

## Configuration

| Env var                         | Default | Notes                                           |
| ------------------------------- | ------- | ----------------------------------------------- |
| `ENRICHER_FETCH_CONCURRENCY`    | `15`    | Parallel HTTP fetches per Kafka fetch (IO-bound) |
| `ENRICHER_LLM_BATCH_SIZE`       | `5`     | Jobs per single `EnrichBatch` call               |
| `ENRICHER_FLUSH_TIMEOUT_SECONDS`| `120`   | Kafka `FetchMaxWait` — accumulate trickle traffic at the broker |
| `ENRICHER_DEFAULT_RPS`          | `2`     | Per-adapter token bucket rate (requests/sec)     |
| `ENRICHER_DEFAULT_BURST`        | `3`     | Per-adapter token bucket burst capacity          |
| `ENRICHER_JITTER_MS`            | `200`   | Random ±jitter after each rate-limit grant (ms)  |
| `DISCARD_JOB_WITH_EXP_GT`       | `0`     | Drop jobs requiring more years; 0 = disabled     |

## dropJob — eligibility rules (Go, not LLM)

| Condition              | Drop if                                  |
| ---------------------- | ---------------------------------------- |
| No sponsorship         | `Sponsorship == false`                   |
| Citizenship required   | `RequiresCitizen == true`                |
| Security clearance     | `SecurityClearance == true`              |
| Over experience limit  | `ExperienceYears > DISCARD_JOB_WITH_EXP_GT` (0 = disabled) |

## Rate limiter (`ratelimit.go`)

One `*rate.Limiter` per adapter, lazy-created on first use with the configured RPS/burst. Concurrent fetches for the same ATS serialize through that adapter's bucket — fetches for different ATSes are unaffected. Adds a small random jitter after each grant so concurrent goroutines don't fire on identical bucket boundaries.

## Logging strategy

| Stage              | Level | Key fields                                                        |
| ------------------ | ----- | ----------------------------------------------------------------- |
| Worker started     | INFO  | `fetch_concurrency`, `llm_batch_size`                             |
| Per-job fetching   | DEBUG | `job_id`, `adapter`, `retry_count`                                |
| Rate limit waited  | DEBUG | `adapter`, `waited` — only logged when wait > 100ms               |
| Per-job fetched    | DEBUG | `job_id`, `description_bytes`                                     |
| Fetch failed       | ERROR | `job_id`, `adapter`, `retry_count`, `err`                         |
| LLM batch sent     | INFO  | `batch_size`, `job_ids`                                           |
| LLM batch returned | INFO  | `batch_size`, `result_count`, `duration`                          |
| Job saved          | DEBUG | `job_id`, `role_match`, `tech_stack_count`                        |
| Job dropped        | INFO  | `job_id`, `drop_reason`                                           |
| Per-fetch summary  | INFO  | `records`, `fetched`, `batches`, `fetch_duration`, `enrich_duration` |

## Rules

- Errors are **per-job** — one bad job never stops the worker
- Whole-batch LLM failure routes each job through its own retry tier (per-job retryCount preserved)
- LLM result missing for a job in the batch → that job is treated as a transient failure
- Snapshot write (`job_analytics`) is non-fatal — if it fails, job row is already saved
- Depends on `service/llm/services.JobPostEnricher` (`EnrichBatch`) — never on `*geminiScorer` directly
- Rate limiter is per-adapter, not per-host — Greenhouse + Lever + Ashby each get their own bucket
