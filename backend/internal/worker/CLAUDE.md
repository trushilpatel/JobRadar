# internal/worker/ — Pipeline workers

## Three workers, three stages

| Package | Consumes | Produces | Writes to DB |
|---|---|---|---|
| `scan/` | companies table | `jobs.raw` | `job_seen_ledger`, `job_scan_log` |
| `scorer/` | `jobs.raw` | `jobs.raw_detail` | updates `job_seen_ledger.reason` |
| `enricher/` | `jobs.raw_detail` | — | `job`, updates `job_seen_ledger.reason` |

## Error handling rule

**Errors are per-job, not per-batch.** One bad job must never stop the worker.

```go
// correct — log, mark error, continue
if err := doSomething(job); err != nil {
    log.Printf("[worker] %s: failed: %v", job.ID, err)
    w.markError(job.ID)
    continue
}

// wrong — returning error stops the entire consumer
return err
```

Failed jobs are marked in `job_seen_ledger` with `reason = 'error'`.
They can be re-processed later by deleting their `job_seen_ledger` row.

## Scan engine specifics

- `RunOnce(ctx)` runs one full cycle and returns — no internal ticker
- Scheduling (every 30 min) lives in `cmd/scan/main.go`, not the engine
- `EnsureTopics` must be called before the producer is created — see `cmd/CLAUDE.md`
