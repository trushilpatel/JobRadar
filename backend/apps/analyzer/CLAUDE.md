# apps/analyzer/ — Job post analyzer (enricher)

Consumes `jobs.filtered.*` (NATS JetStream) as `core.JobSummary`, fetches full job detail via the crawler-v2 two-phase model (with per-adapter rate limiting), runs LLM enrichment in batches, writes final rows to `jobs` and `job_analytics`.

## Two-stage flow per fetch

```
FetchBatch (up to ENRICHER_FLUSH_TIMEOUT_SECONDS)
   ↓
Stage 1 — parallel detail fetches (cap = ENRICHER_FETCH_CONCURRENCY), resolveJobPost:
   - JobContent set? (fat adapters: gem/uber/deel)  → jobContentToPost()  (no HTTP)
   - JobContent nil  (thin adapters)                → adapter rate-limit Wait →
                                                       crawler-v2 adapters.Registry() bundle.Post.FetchJobPost(summary)
   - no PostFetcher / unknown adapter / fetch error → handleFailure → retry/DLQ (never an empty enrich)
   ↓
Stage 2 — serial: group successful (job, content) into chunks of ENRICHER_LLM_BATCH_SIZE
   - sleeps ENRICHER_LLM_CALL_INTERVAL_SECONDS between EnrichBatch calls (ctx-aware)
   - missing job_id in result → transient failure for that job
   - per-job: drop check → save or markIrrelevant / markError
   - whole batch fails  → routeBatchFailure → each job through retry ladder
   - CommitRecords after each successful group (per-partition max-offset semantics)
   - processGroup returns routed=false (ctx cancelled mid-batch) → skip commit, return
```

## dropJob — eligibility rules (Go, not LLM)

| Condition             | Drop if                                                    |
| --------------------- | ---------------------------------------------------------- |
| No sponsorship        | `Sponsorship == false`                                     |
| Citizenship required  | `RequiresCitizen == true`                                  |
| Security clearance    | `SecurityClearance == true`                                |
| Over experience limit | `ExperienceYears > DISCARD_JOB_WITH_EXP_GT` (0 = disabled) |

## Ledger retry state

- Only **Stage 2** (`EnrichBatch`) failures write `reason='retry'` to `job_pipeline_ledger`
- Stage 1 (HTTP fetch) failures route to retry-tier topics but **do NOT** touch the ledger
- Keeps `reason='retry'` semantically consistent: LLM-layer failure only
- Transitions: `retry → llm_irrelevant | retry | error`

## Ledger-row-missing tolerance

`markError`/`markIrrelevant`/`markNonUSA` (and `markLedgerRetryWithRetry`) call ledger UPDATEs that now return `ErrLedgerRowMissing` when the row is absent (e.g. cleaned by a maintenance migration). At the **analyzer** stage this is **non-fatal** — log WARN, increment `enricher_ledger_missing_total{reason}`, and continue (do NOT Nak/retry; the job is already enriched, a missing ledger row is only an observability gap). This differs from the **filter**, which Naks on the same sentinel because there the missing row means a genuine crawler/filter race. `markLedgerRetryWithRetry` skips its second attempt on `ErrLedgerRowMissing` (the row is gone, not transient).

Duplicate inserts: `SaveEnrichedJob` tolerates `ErrJobAlreadyExists` from `InsertJob` (the `jobs.job_post_id` unique index) — it skips the snapshot/tech-tag writes, still marks the ledger `saved`, increments `enricher_jobs_duplicate_total`, and returns nil.

## Configuration

| Env var                          | Default | Notes                                    |
| -------------------------------- | ------- | ---------------------------------------- |
| `ENRICHER_FETCH_CONCURRENCY`     | `15`    | Parallel HTTP fetches (Stage 1)          |
| `ENRICHER_LLM_BATCH_SIZE`        | `5`     | Jobs per EnrichBatch call (Stage 2)      |
| `ENRICHER_FLUSH_TIMEOUT_SECONDS` | `120`   | NATS fetch timeout                       |
| `ENRICHER_DEFAULT_RPS`           | `2`     | Per-adapter token-bucket rate            |
| `ENRICHER_DEFAULT_BURST`         | `3`     | Per-adapter burst capacity               |
| `ENRICHER_JITTER_MS`             | `200`   | Random ±jitter after rate-limit grant    |
| `DISCARD_JOB_WITH_EXP_GT`        | `0`     | Drop jobs over N years exp; 0 = disabled |

## Rules

- Errors are per-job — one bad job never stops the worker
- Stage 1 failures do NOT write retry ledger; Stage 2 does
- Whole-batch LLM failure routes each job through its own retry tier
- Snapshot write (`job_analytics`) is non-fatal — the job row is already saved if it fails
- Depends on `service/llm/services.JobPostEnricher` — never on `*geminiScorer` directly
- Rate limiter is per-adapter — Greenhouse, Lever, Ashby each get their own bucket
