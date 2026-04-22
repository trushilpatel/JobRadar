# apps/filter/ — Relevance filter worker

Consumes `job_listings.raw` (Kafka), scores jobs in batches via LLM, routes relevant jobs to `job_listings.filtered`, marks irrelevant/error jobs in `job_pipeline_ledger`.

## Key files

| File            | Role                                                              |
| --------------- | ----------------------------------------------------------------- |
| `main.go`       | Wiring — Kafka consumer/producer, LLM scorer, ledger store        |
| `worker.go`     | Batch accumulation, parallel `ScoreBatch` calls, result routing   |
| `interfaces.go` | `FilterLedger` — narrow ledger interface the worker depends on    |

## Batch flow

1. Accumulate jobs until `FILTER_BATCH_SIZE × FILTER_LLM_PARALLELISM` are collected, or `FILTER_FLUSH_TIMEOUT_SECONDS` elapses
2. Split into chunks of exactly `FILTER_BATCH_SIZE` jobs each
3. Score all chunks in parallel (up to `FILTER_LLM_PARALLELISM` concurrent LLM calls) — each chunk is one `ScoreBatch` call
4. Relevant → publish to `job_listings.filtered`
5. Irrelevant → mark `reason='llm_irrelevant'` in ledger
6. ScoreBatch error → route chunk through retry ladder / DLQ — **never re-queue directly**

## Error handling rule

On `ScoreBatch` failure the whole sub-batch is marked `error` in `job_pipeline_ledger`. Re-queuing caused an infinite tight loop in the past. Reprocess by deleting the ledger rows manually.

## LLM dependency

Depends on `service/llm/services.JobsRelevanceFilter` — never on `*geminiScorer` directly.
