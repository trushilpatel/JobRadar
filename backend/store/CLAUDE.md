# store/ — Shared store utilities

This package contains only shared utilities used across service packages.

## IsNotFound

`store.IsNotFound(err)` wraps `gorm.ErrRecordNotFound` for use in service code:

```go
job, err := s.jobs.GetJobByID(ctx, id)
if store.IsNotFound(err) {
    return nil, domain.ErrNotFound.WithMessage("job not found")
}
```

## Store interfaces have moved

Store interfaces and their GORM implementations now live in each service package:

| Package                | Store file                                                                          |
| ---------------------- | ----------------------------------------------------------------------------------- |
| `service/job/`         | `store.go` — JobStore, SeenLedgerStore, DiscardStore, SnapshotWriter, TechTagWriter |
| `service/company/`     | `store.go` — CompanyStore                                                           |
| `service/application/` | `store.go` — ApplicationStore                                                       |
| `service/dashboard/`   | `store.go` — DashboardStore, PipelineStore                                          |
| `service/insights/`    | `store.go` — InsightsStore                                                          |
| `service/scan/`        | `store.go` — ScanLogWriter                                                          |

Each store file defines the interfaces the service depends on, then implements them with GORM — all in the same package. This keeps each domain fully self-contained.
