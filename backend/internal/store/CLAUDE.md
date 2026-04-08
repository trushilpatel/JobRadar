# internal/store/ — DB query layer

One file per table. Each file owns exactly one table — no cross-table queries.

| File | Interface(s) | Table |
|---|---|---|
| `company.go` | `CompanyReader` | `companies` |
| `seen_ledger.go` | `SeenLedgerReader`, `SeenLedgerWriter`, `SeenLedgerStore` | `job_seen_ledger` |
| `company_scan_log.go` | `CompanyScanLogWriter`, `CompanyScanLogReader`, `CompanyScanLogStore` | `company_scan_log` |
| `job.go` | `JobReader`, `JobWriter`, `JobStore` | `job` |
| `job_snapshot.go` | `SnapshotWriter` | `job_snapshot` |
| `application.go` | `ApplicationStore` | `job_application` |
| `discard.go` | `DiscardStore` | `job_discarded` |
| `insights.go` | `InsightsReader` | `job`, `job_snapshot`, `job_seen_ledger` (read-only aggregations) |
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
- `job` + `companies` — tech stack trends, company activity
- `job_snapshot` + `companies` — funnel, location, velocity, sponsorship, role categories
- `job_seen_ledger` + `job` — `GetPipelineCounts()`: pending (fetched but not yet enriched) and error counts

`GetPipelineCounts()` returns:
- `pending` = `job_seen_ledger` rows with `reason='fetch'` that have no matching row in `job` (by `job_post_id`)
- `errors` = `job_seen_ledger` rows with `reason='error'`

These are surfaced in `GET /dashboard/stats` as `pipeline_pending` and `pipeline_errors`.

## job_snapshot vs job

`job_snapshot` is the analytics source of truth. `job` is the operational feed.

- Enricher writes to both on every enriched job
- `job_snapshot.user_action` is backfilled by the API when user saves/applies/dismisses
- `InsightsReader` queries `job_snapshot` for funnel, location, velocity, sponsorship, role-category
- Never delete `job_snapshot` rows — they represent immutable pipeline history

## SnapshotWriter usage

Only the enricher and the job handler (for backfilling `user_action`) should hold `SnapshotWriter`:

```go
// enricher: write at pipeline time
snapshot.InsertSnapshot(store.SnapshotInput{...})

// job handler: backfill on user action (non-fatal, use _ =)
_ = h.snapshot.UpdateSnapshotAction(id, "saved")
```
