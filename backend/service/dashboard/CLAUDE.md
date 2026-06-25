# dashboard/ — Dashboard aggregations: global stats + user panels + pipeline status

## Three sub-domains, not one

The dashboard slice contains three independent service contracts, each with its own impl and REST pair:

```
DashboardService       — global stats (GetStats, GetActivity) — shared read interface
UserDashboardService   — per-user panels (GetProgressStats, GetMarketStats, GetHeatmap, GetInterviews, GetVelocity, GetTechTrends)
PipelineService        — pipeline health (GetPipelineStats, TriggerRetry)
```

All three are wired in a single `ProviderSet` in `wire.go` and composed inside one `RestService`/`RestRouter` pair for the main dashboard route, plus a separate `PipelineRestService`/`PipelineRestRouter` pair for pipeline routes.

## Wire shape

```go
// ProviderSet wires:
NewRestRouter            // main dashboard routes
NewDashboardRestService  // depends on DashboardService + UserDashboardService
NewDashboardImplService  // implements DashboardService
NewUserDashboardStore    // DB terminal for user dashboard queries
NewUserDashboardService  // implements UserDashboardService
NewPipelineStore         // DB terminal for pipeline stats
NewPipelineService       // implements PipelineService
```

## Interfaces

| Interface               | Declared in                   | Purpose                                            |
| ----------------------- | ----------------------------- | -------------------------------------------------- |
| `DashboardService`      | `DashboardService.go`         | Global stats aggregate (GetStats, GetActivity)     |
| `UserDashboardService`  | `DashboardUserService.go`     | Per-user panel data                                |
| `PipelineService`       | `DashboardPipelineService.go` | Pipeline health stats + retry trigger              |
| `ApplicationListReader` | `DashboardUserService.go`     | Narrow cross-domain read from `application/` slice |

## Key invariants

- `DashboardInterfaces.go` is a placeholder file (`package services` with no exported symbols) — cross-domain interfaces are declared in the file of the service that owns them, not in a separate interfaces file.
- `UserDashboardService` depends on `ApplicationListReader` (a narrow interface over `application.ApplicationService`) — this is the only cross-domain dependency in the dashboard slice. It must remain an interface, never a concrete import.
- `PipelineService.TriggerRetry` mutates in-flight retry state — it is admin-only and must remain behind `RequireRole("admin")` at the router level.
- `RestService` holds both `DashboardService` and `UserDashboardService` as fields — this is the one place in the slice where two service interfaces coexist in a single struct. It does not implement either interface.

## Non-canonical files

| File                                                                 | Purpose                                                                             |
| -------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| `DashboardUserService.go`                                            | Contract + `BaseService` for per-user panels; also declares `ApplicationListReader` |
| `DashboardPipelineService.go`                                        | Contract + `BaseService` for pipeline health                                        |
| `DashboardUserImplService.go`                                        | Impl of `UserDashboardService`                                                      |
| `DashboardPipelineImplService.go`                                    | Impl of `PipelineService`                                                           |
| `DashboardPipelineRestService.go` + `DashboardPipelineRestRouter.go` | Separate REST pair for pipeline routes                                              |
| `DashboardRepositoryService.go`                                      | DB terminal for global stats; also serves as `UserDashboardStore`                   |
| `compat.go`                                                          | Slice-root package — no domain logic                                                |
