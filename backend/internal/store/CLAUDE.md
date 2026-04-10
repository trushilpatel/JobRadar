# internal/store/ — DB query layer

One file per table. Each file owns exactly one table — no cross-table queries.

| File | Interface(s) | Table |
|---|---|---|
| `company.go` | `CompanyReader` | `companies` |
| `scan_queue.go` | `ScanQueueStore` | `company_scan_queue` |
| `seen_ledger.go` | `SeenLedgerReader`, `SeenLedgerWriter`, `SeenLedgerStore` | `job_pipeline_ledger` |
| `company_scan_log.go` | `CompanyScanLogWriter`, `CompanyScanLogReader`, `CompanyScanLogStore` | `company_scan_log` |
| `job.go` | `JobReader`, `JobWriter`, `JobStore` | `job_feed` |
| `job_snapshot.go` | `SnapshotWriter` | `job_analytics` |
| `application.go` | `ApplicationStore` | `job_application_tracker` |
| `discard.go` | `DiscardStore` | `job_dismissal_log` |
| `insights.go` | `InsightsReader` | `job_feed`, `job_analytics`, `job_pipeline_ledger` (read-only aggregations) |
| `errors.go` | `IsNotFound(err)` helper | — |

## Interface split pattern

Split Reader/Writer when a worker only needs one direction:

```go
// scan engine only needs to read companies — give it CompanyReader, not a full store
type ScanEngine struct {
    companies store.CompanyReader   // can't accidentally write
    ledger    store.SeenLedgerStore // needs both read + write
}
```

## IsNotFound

Use `store.IsNotFound(err)` to distinguish "record not found" from real DB errors:

```go
job, err := h.jobs.GetJobByID(id)
if store.IsNotFound(err) {
    respondError(w, http.StatusNotFound, "job not found")
    return
}
```

## InsightsReader cross-table queries

`InsightsReader` is the only store interface permitted to query across tables. It covers:
- `job_feed` + `companies` — tech stack trends, company activity
- `job_analytics` + `companies` — funnel, location, velocity, sponsorship, role categories
- `job_pipeline_ledger` + `job_feed` — `GetPipelineCounts()`: pending (fetched but not yet enriched) and error counts

`GetPipelineCounts()` returns:
- `pending` = `job_pipeline_ledger` rows with `reason='fetch'` that have no matching row in `job_feed` (by `job_post_id`)
- `errors` = `job_pipeline_ledger` rows with `reason='error'`

These are surfaced in `GET /dashboard/stats` as `pipeline_pending` and `pipeline_errors`.

## job_analytics vs job_feed

`job_analytics` is the analytics source of truth. `job_feed` is the operational feed.

- Enricher writes to both on every enriched job
- `job_analytics.user_action` is backfilled by the API when user saves/applies/dismisses
- `InsightsReader` queries `job_analytics` for funnel, location, velocity, sponsorship, role-category
- Never delete `job_analytics` rows — they represent immutable pipeline history

## SnapshotWriter usage

Only the enricher and the job handler (for backfilling `user_action`) should hold `SnapshotWriter`:

```go
// enricher: write at pipeline time
snapshot.InsertSnapshot(store.SnapshotInput{...})

// job handler: backfill on user action (non-fatal, use _ =)
_ = h.snapshot.UpdateSnapshotAction(id, "saved")
```
