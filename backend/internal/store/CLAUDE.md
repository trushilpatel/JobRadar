# internal/store/ — DB query layer

One file per table. Each file owns exactly one table — no cross-table queries.

| File | Interface(s) | Table |
|---|---|---|
| `company.go` | `CompanyReader` | `companies` |
| `seen_ledger.go` | `SeenLedgerReader`, `SeenLedgerWriter`, `SeenLedgerStore` | `job_seen_ledger` |
| `job_scan_log.go` | `JobScanLogWriter` | `job_scan_log` |
| `job.go` | `JobReader`, `JobWriter`, `JobStore` | `job` |
| `application.go` | `ApplicationStore` | `job_application` |
| `discard.go` | `DiscardStore` | `job_discarded` |
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
