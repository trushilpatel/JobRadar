# Service Package Structure Design

**Date:** 2026-04-18  
**Scope:** Redesign all service packages (`service/{domain}/`) to improve organization, discoverability, and maintainability

---

## Overview

This design standardizes how all service packages are organized. Each domain (job, company, application, dashboard, insights, scan) follows the same folder and file structure, making the codebase predictable and easier to navigate.

---

## Target Structure

Every service package follows this layout:

```
service/{domain}/
  services/
    {Domain}Service.go           — Interface contract
    {Domain}ImplService.go       — Business logic implementation
    {Domain}RestService.go       — HTTP handler (router, endpoints)
    {Domain}ValidationService.go — Input validation
    {Domain}RepositoryService.go — Database access (repository pattern)

  model/
    {Domain}Model.go           — Database struct with GORM tags

  dto/
    {Domain}Filter.go          — Query filter type
    Create{Domain}Input.go     — POST/PUT request body
    {Domain}ListResult.go      — Paginated response
    {Domain}View.go            — Single item response
    [each DTO in its own file]

  common/
    constants.go               — Domain constants (enums, status values)

  wire.go                       — Google Wire provider set
```

---

## Folder Breakdown

### `services/`

Contains all service implementations. Organized by concern:

| File                           | Role                                           | Depends On                                               |
| ------------------------------ | ---------------------------------------------- | -------------------------------------------------------- |
| `{Domain}Service.go`           | Public interface contract                      | Nothing (pure interface)                                 |
| `{Domain}ImplService.go`       | Business logic implementation                  | `{Domain}RepositoryService`, `dto/*`, `common/constants` |
| `{Domain}RestService.go`       | HTTP handler (replaces old `handler.go`)       | `{Domain}Service`, `dto/*`                               |
| `{Domain}ValidationService.go` | Input validation (replaces old `validator.go`) | `dto/*`, domain errors                                   |
| `{Domain}RepositoryService.go` | Database access (replaces old `store.go`)      | `model/{Domain}Model.go`, GORM                           |

**Key principle:** Each service has a single responsibility. Implementations can be swapped (e.g., `JobsRestService` ↔ `JobsGrpcService`).

### `model/`

Single file containing the database entity:

| File               | Role                                           |
| ------------------ | ---------------------------------------------- |
| `{Domain}Model.go` | Struct with GORM tags, mirrors database schema |

No business logic here — just the schema representation.

### `dto/`

Data Transfer Objects — each type in its own file for discoverability:

| File                     | Purpose                             | Used By                                       |
| ------------------------ | ----------------------------------- | --------------------------------------------- |
| `{Domain}Filter.go`      | Query parameters for LIST endpoints | RestService, DbService                        |
| `Create{Domain}Input.go` | POST body                           | RestService → ValidationService → ImplService |
| `Update{Domain}Input.go` | PUT/PATCH body                      | RestService → ValidationService → ImplService |
| `{Domain}ListResult.go`  | Paginated response (items + total)  | RestService → client                          |
| `{Domain}View.go`        | Single item response                | RestService → client                          |
| `{Domain}Counts.go`      | Summary counts                      | RestService → client                          |

Each file contains exactly one type. This makes searching (`types/JobFilter.go`) instant.

### `common/`

Domain-wide constants and utilities:

```go
// constants.go
const (
    StatusNew       = "new"
    StatusSaved     = "saved"
    StatusApplied   = "applied"
    StatusDismissed = "dismissed"
)
```

If constants grow large, split into `constants_status.go`, `constants_stage.go`, etc.

### `wire.go`

Google Wire provider set. Unchanged from current pattern:

```go
var ProviderSet = wire.NewSet(
    NewService,
    NewHandler,
    NewValidator,
    NewDbService,
)
```

---

## Dependency Flow

```
RestService
    ↓
ValidationService (returns domain.ValidationError if invalid)
    ↓
ImplService (business logic, calls RepositoryService)
    ↓
RepositoryService (database access)
    ↓
Model (GORM entity)

dto/* (used at boundaries)
common/constants.go (used everywhere)
```

**Key rules:**

- Handlers call services only — never RepositoryService directly
- Services call RepositoryService only — never raw GORM
- RepositoryService is the only place with SQL/GORM
- DTOs are passed between layers; services never return `Model`
- Constants from `common/` are used throughout

---

## Naming Conventions

### File names (CamelCase)

- Service interface: `{Domain}Service.go`
- Implementation: `{Domain}ImplService.go`
- Handler: `{Domain}RestService.go` (allows future `{Domain}GrpcService.go`)
- Validator: `{Domain}ValidationService.go`
- Database: `{Domain}DbService.go`

### Type names (match domain)

- Domain: `job` → types are `Job`, `JobFilter`, `JobView`, `CreateJobInput`
- Domain: `company` → types are `Company`, `CompanyFilter`, `CompanyView`, `CreateCompanyInput`

### Constants (SCREAMING_SNAKE_CASE)

```go
const (
    JobStatusNew       = "new"
    JobStatusSaved     = "saved"
    ApplicationStageInterview = "interview"
)
```

---

## Example: Jobs Service

```
service/jobs/
  services/
    JobsService.go              — interface Service { List(), Get(), Dismiss(), ... }
    JobsImplService.go          — type service struct { repo, logger } implementing Service
    JobsRestService.go          — HTTP handlers: ListJobs(), GetJob(), DismissJob()
    JobsValidationService.go    — ValidateDismiss(), ValidateFilter()
    JobsRepositoryService.go    — List(), Get(), SetStatus() using GORM

  model/
    JobsModel.go                — type Job struct { ID, Title, ... gorm:"..." }

  dto/
    JobFilter.go                — type JobFilter struct { Status, CompanyID, ... }
    CreateJobInput.go           — type CreateJobInput struct { ... }
    UpdateJobInput.go           — type UpdateJobInput struct { ... }
    JobView.go                  — type JobView struct { ID, Title, ... }
    JobListResult.go            — type JobListResult struct { Items []JobView, Total int64 }
    JobCounts.go                — type JobCounts struct { New, Saved, Applied int }

  common/
    constants.go                — const (JobStatusNew, JobStatusSaved, ...)

  wire.go
```

---

## Benefits

| Aspect              | Improvement                                                                              |
| ------------------- | ---------------------------------------------------------------------------------------- |
| **Discoverability** | Each type in own file → instant search (`dto/JobFilter.go`)                              |
| **Clarity**         | File names explicitly state purpose (`JobsImplService.go` vs vague `service.go`)         |
| **Scalability**     | Multiple handlers possible without conflict (`JobsRestService.go`, `JobsGrpcService.go`) |
| **Testability**     | Narrow interfaces, clear dependencies → easier mocks                                     |
| **Maintenance**     | Changes to one layer don't scatter across files                                          |
| **Consistency**     | All domains follow identical pattern                                                     |

---

## Migration Strategy

Apply this structure to all service packages in order:

1. Scheduler (`service/scan/`) — already partially refactored, finish this first
2. Company (`service/company/`)
3. Job (`service/job/`)
4. Application (`service/application/`)
5. Dashboard (`service/dashboard/`)
6. Insights (`service/insights/`)

Each migration is a single task: restructure folders, rename files, update imports, run tests.

---

## Future Considerations

- If constants file grows large (20+ constants), split into `constants_status.go`, `constants_stage.go`
- If DTOs folder grows large (15+ files), consider subfolders by request/response type
- For gRPC/GraphQL services, add `{Domain}GrpcService.go` or `{Domain}GraphQLService.go` to `services/` folder
- Config-specific to a domain goes in its own `config.go` file (e.g., `FilterConfig` lives in `service/filter/config.go`)

---

## Current State vs. New Structure

| Current                         | New                                               |
| ------------------------------- | ------------------------------------------------- |
| `service/job/service.go`        | `service/job/services/JobsImplService.go`         |
| `service/job/handler.go`        | `service/job/services/JobsRestService.go`         |
| `service/job/validator.go`      | `service/job/services/JobsValidationService.go`   |
| `service/job/store.go`          | `service/job/services/JobsRepositoryService.go`   |
| `service/job/types.go` (1 file) | `service/job/dto/` (many files, one per type)     |
| No standard structure           | `service/job/services/JobsService.go` (interface) |
| Constants scattered             | `service/job/common/constants.go`                 |
| No model folder                 | `service/job/model/JobsModel.go`                  |
