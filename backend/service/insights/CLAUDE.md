# insights/ — Analytics queries: tech stack, funnel, location, sponsorship, velocity, role categories, pipeline counts

## Interface

`InsightsService` — 8 methods covering all analytics surfaces. Standard chain: `RepositoryService → ImplService → ValidationService → RestService → RestRouter`.

## Key invariants

- `TechStack`, `CompanyActivity`, `Funnel`, `Location`, `Velocity`, `RoleCategories`, `GetPipelineCounts` are **global** reads — no `userID` scoping. `Sponsorship` takes an `InsightsFilter` for optional date-range narrowing.
- Historical / time-series data reads `job_analytics` (append-only snapshot rows, never deleted). Current-state per-user funnel aggregations (`Funnel`) read `user_job_state.status`. Never mix the two tables for a single aggregation — they answer different questions.
- `GetPipelineCounts` provides pipeline visibility (ledger reason distribution) — it reads `job_pipeline_ledger`, not `jobs`.

## Test layout deviation

Internal package tests live in `services/` alongside the production files:

| File                                   | Why it's internal                                              |
| -------------------------------------- | -------------------------------------------------------------- |
| `services/repository_internal_test.go` | Tests unexported SQL helpers in `InsightsRepositoryService.go` |
| `services/velocity_internal_test.go`   | Tests unexported velocity computation helpers                  |

Standard external tests live in `tests/` as `package tests`. Do not move the internal tests to `tests/` — they require package-internal access.
