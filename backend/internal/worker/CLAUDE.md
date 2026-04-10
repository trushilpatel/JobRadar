# internal/worker/ — Pipeline workers

## Three workers, three stages

| Package | Consumes | Produces | Writes to DB |
|---|---|---|---|
| `job_listing_crawler/` | companies table | `job_listings.raw` | `job_seen_ledger`, `company_scan_log` |
| `job_listing_relevance_filter/` | `job_listings.raw` | `job_listings.filtered` | updates `job_seen_ledger.reason` |
| `job_post_analyzer/` | `job_listings.filtered` | — | `job`, `job_snapshot`, updates `job_seen_ledger.reason` |

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

Failed jobs are marked in `job_seen_ledger` with `reason = 'error'`.
They can be re-processed later by deleting their `job_seen_ledger` row.

## Enricher snapshot rule

The enricher writes to `job_snapshot` **after** a successful `InsertJob`. The snapshot write is non-fatal — if it fails, the job row is already saved and the error is logged. Do not roll back the job insert if the snapshot fails.

```go
// correct — snapshot failure is non-fatal, log as Warn not Error
if err := w.snapshot.InsertSnapshot(...); err != nil {
    jobLog.Warn("write snapshot failed (non-fatal)", logger.Err(err))
    // continue — job is saved
}
```

## job_listing_relevance_filter error handling

On `ScoreBatch` failure the entire batch is marked `reason='error'` in `job_seen_ledger` — **not re-queued**. Re-queuing caused an infinite tight loop because the batch was immediately re-consumed.

Failed jobs can be reprocessed by deleting their `job_seen_ledger` rows.

## job_listing_crawler specifics

- `RunOnce(ctx)` runs one full cycle and returns — no internal ticker
- Scheduling lives in `cmd/company_scan_scheduler/main.go` — checks every 1 min, enqueues companies whose `last_scan_at` is older than `SCAN_INTERVAL_MINUTES`
- Manual trigger via `POST /companies/:id/scan` also respects `SCAN_INTERVAL_MINUTES` — returns 429 if scanned too recently
- `EnsureTopics` must be called before the producer is created — see `cmd/CLAUDE.md`
