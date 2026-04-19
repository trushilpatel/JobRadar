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

## Repository files

Store interfaces and their GORM implementations live in each service package, one file per DB table:

| Service                | Repository files                                                                                                                        |
| ---------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| `service/job/`         | `JobFeedRepository.go`, `JobSeenLedgerRepository.go`, `JobSnapshotRepository.go`, `JobDiscardRepository.go`, `JobTechTagRepository.go` |
| `service/company/`     | `CompanyRepository.go`                                                                                                                  |
| `service/application/` | `ApplicationRepository.go`                                                                                                              |
| `service/dashboard/`   | `DashboardRepositoryService.go`                                                                                                         |
| `service/insights/`    | `InsightsRepositoryService.go`                                                                                                          |
| `service/scan/`        | `ScanRepositoryService.go`                                                                                                              |

Each repository file defines the store interface(s) for one DB table, then implements them with GORM — all in the same package. This keeps each domain fully self-contained.
