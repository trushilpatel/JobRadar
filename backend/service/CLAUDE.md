# service/ — Feature-slice packages

Each subdirectory is a fully self-contained vertical slice for one domain.

## Slice structure

Every service package has the same layout:

```
{domain}/
├── wire.go                            # Wire ProviderSet — re-exports constructors from services/
├── common/
│   └── {Domain}ServiceConstants.go    # Domain constants (package common)
├── dto/
│   └── {TypeName}.go                  # One file per input/output type (package dto)
├── models/
│   └── {table_name}.go                # One file per DB table — GORM struct, package {domain}model
├── services/
│   ├── {Domain}Service.go             # All service-level interfaces: Service, Validator, LedgerService, cross-service writers
│   ├── {Domain}ImplService.go         # Business logic implementation
│   ├── {Domain}RestService.go         # HTTP handler — implements handler.RouteGroup
│   ├── {Domain}ValidationService.go   # Input validation
│   ├── {Table}Repository.go           # One file per DB table: store interface + GORM implementation
│   └── {Domain}*ImplService.go        # Additional impl files for sub-services (Pipeline, Ledger, etc.)
└── tests/
    └── {Domain}{Role}_test.go         # Tests mirroring the services/ file they cover
```

**Model package naming:** each `models/` directory uses a domain-prefixed package name — `jobmodel`, `companymodel`, `appmodel`, `scanmodel`. This prevents import alias conflicts when a file needs structs from multiple service domains.

**Repository file naming:** use the table's logical name, not the domain prefix — e.g. `JobFeedRepository.go`, `JobSeenLedgerRepository.go`, `JobSnapshotRepository.go`. Each file owns exactly one DB table.

### Root wire.go pattern

```go
package {domain}

import (
    "github.com/google/wire"
    svc "github.com/trushilpatel/jobradar/service/{domain}/services"
)

var ProviderSet = wire.NewSet(svc.NewRestService, svc.NewValidator, svc.NewService, ...)
```

## Packages

| Package        | Owns                                                                                    |
| -------------- | --------------------------------------------------------------------------------------- |
| `job/`         | Job listing: list, get, dismiss, save, set status, set notes                            |
| `company/`     | Company CRUD, scan trigger, scan queue reads                                            |
| `application/` | Application tracker: insert, update stage/note                                          |
| `dashboard/`   | Dashboard stats (job counts, daily activity) + pipeline status (DB stats + Kafka lag)   |
| `insights/`    | Analytics queries: tech stack, funnel, location, sponsorship, velocity, role categories |
| `scan/`        | Scan result recording — used by the crawler pipeline worker (not the API)               |

## Service layer rules

- Services return `*domain.ServiceError` (via sentinel `.WithMessage()`) — never `fmt.Errorf` for errors that handlers inspect
- Services hold store interfaces, never concrete GORM structs — dependencies injected via constructor
- No HTTP types in services — `http.Request`, `http.ResponseWriter`, chi context are handler concerns only
- All service methods take `context.Context` as first argument and use `tracing.Start`/`tracing.End` for OTel spans

## Store layer rules

- One repository file per DB table — `{Table}Repository.go` — containing the store interface(s) and GORM implementation for that table only
- Cross-service writer interfaces (e.g. `ApplicationWriter` in the job service) live in `{Domain}Service.go`, not in a repository file, because they are service-level dependencies, not DB table implementations
- GORM implementations live in the same file as their interface — no separate package for implementations
- All store methods take `context.Context` and use `tracing.Start`/`tracing.End`
- Use `store.IsNotFound(err)` to distinguish "not found" from real errors

## Handler layer rules

- Handlers only parse, validate, call service, and respond — no business logic
- All responses use `handler.Respond`, `handler.RespondPaged`, or `handler.HandleError`
- Validation happens before any service call: `h.validator.ValidateX(input)` → `handler.HandleError` on failure

## Wire

Each package root exports a `ProviderSet` for Google Wire via `wire.go`. The root `cmd/api/wire.go` composes all provider sets. Constructors are always imported from the `services/` subpackage.
