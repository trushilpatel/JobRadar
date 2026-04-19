# apps/filter/ — Relevance filter worker

Consumes `job_listings.raw` (Kafka), scores jobs in batches via LLM, routes relevant jobs to `job_listings.filtered`, marks irrelevant/error jobs in `job_pipeline_ledger`.

## Key files

| File            | Role                                                              |
| --------------- | ----------------------------------------------------------------- |
| `main.go`       | Wiring — Kafka consumer/producer, LLM scorer, ledger store        |
| `worker.go`     | Batch accumulation, parallel `ScoreBatch` calls, result routing   |
| `interfaces.go` | `FilterLedger` — narrow ledger interface the worker depends on    |

## Batch flow

1. Accumulate jobs up to `FILTER_BATCH_SIZE` or `FILTER_FLUSH_TIMEOUT_SECONDS`
2. Split into `LLM_PARALLELISM` sub-batches, score in parallel
3. Relevant → publish to `job_listings.filtered`
4. Irrelevant → mark `reason='llm_irrelevant'` in ledger
5. ScoreBatch error → mark entire sub-batch `reason='error'` — **never re-queue**

## Error handling rule

On `ScoreBatch` failure the whole sub-batch is marked `error` in `job_pipeline_ledger`. Re-queuing caused an infinite tight loop in the past. Reprocess by deleting the ledger rows manually.

## LLM dependency

Depends on `service/llm/services.JobsRelevanceFilter` — never on `*geminiScorer` directly.
