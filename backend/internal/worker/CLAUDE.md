# internal/worker/ — Pipeline workers

## Three workers, three stages

| Package | Consumes | Produces | Writes to DB |
|---|---|---|---|
| `job_listing_crawler/` | `company_scan_queue` (DB poll) | `job_listings.raw` | `job_pipeline_ledger`, `company_scan_log` |
| `job_listing_relevance_filter/` | `job_listings.raw` | `job_listings.filtered` | updates `job_pipeline_ledger.reason` |
| `job_post_analyzer/` | `job_listings.filtered` | — | `job_feed`, `job_analytics`, updates `job_pipeline_ledger.reason` |

## Error handling rule

**Errors are per-job, not per-batch.** One bad job must never stop the worker.

```go
// correct — use a child logger with job_id baked in, mark error, continue
jobLog := w.log.With(logger.Str(logger.FieldJobID, job.ID))
if err := doSomething(job); err != nil {
    jobLog.Error("step failed", err)
    w.markError(job.ID)
    continue
}

// wrong — returning error stops the entire consumer
return err
```

Failed jobs are marked in `job_pipeline_ledger` with `reason = 'error'`.
They can be re-processed later by deleting their `job_pipeline_ledger` row.

## Job eligibility — `dropJob`

Before writing to the DB, `job_post_analyzer` calls `dropJob(enriched)`. If it returns a non-empty reason, the job is marked `error` in `job_pipeline_ledger` and skipped — never written to `job_feed`.

Conditions (all checked in Go, not the LLM):
- `Sponsorship == false` — posting explicitly denies visa sponsorship
- `RequiresCitizen == true` — posting requires US Citizen or Green Card
- `SecurityClearance == true` — posting requires any security clearance
- `ExperienceYears > DISCARD_JOB_WITH_EXP_GT` — experience requirement exceeds config limit (0 = disabled)

`dropJob` is a method on `Worker` so it can read `maxExpYears` from config. Never add business logic anywhere else for these rules.

## Enricher snapshot rule

The enricher writes to `job_analytics` **after** a successful `InsertJob`. The snapshot write is non-fatal — if it fails, the job row is already saved and the error is logged. Do not roll back the job insert if the snapshot fails.

```go
// correct — snapshot failure is non-fatal, log as Warn not Error
if err := w.snapshot.InsertSnapshot(...); err != nil {
    jobLog.Warn("write snapshot failed (non-fatal)", logger.Err(err))
    // continue — job is saved
}
```

## job_listing_relevance_filter error handling

On `ScoreBatch` failure the entire batch is marked `reason='error'` in `job_pipeline_ledger` — **not re-queued**. Re-queuing caused an infinite tight loop because the batch was immediately re-consumed.

Failed jobs can be reprocessed by deleting their `job_pipeline_ledger` rows.

## job_post_analyzer — FullContent fast path

If `RawJob.FullContent != nil`, the analyzer skips both the HTTP fetch and the `ContentFetcher` call. It calls `fullContentToResponse(job)` to convert the pre-fetched data directly to `FetchJobContentResponse`, then proceeds to LLM enrichment as normal.

```
job.FullContent != nil  →  fullContentToResponse()  →  Enrich()  →  DB
job.FullContent == nil  →  fetchContent()            →  Enrich()  →  DB
```

The LLM enrichment step always runs — `FullContent` only skips the fetch, not the enrichment. This is intentional: enrichment extracts sponsorship flags, tech stack, salary, etc. regardless of how content was obtained.

## job_listing_crawler specifics

- `RunForCompany(ctx, companyID)` runs a scan for one company — called by the crawler's DB poll loop
- The crawler polls `company_scan_queue` every 10s; `ClaimNext` uses `FOR UPDATE SKIP LOCKED` so concurrent instances don't double-process
- Scheduling lives in `cmd/company_scan_scheduler/main.go` — checks every 1 min, inserts due companies into `company_scan_queue`
- Manual trigger via `POST /companies/:id/scan` also respects `SCAN_INTERVAL_MINUTES` — returns 429 if scanned too recently
- `EnsureTopics` must be called before the producer is created — see `cmd/CLAUDE.md`
