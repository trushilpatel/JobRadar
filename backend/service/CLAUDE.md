# service/ — Feature-slice packages

Each subdirectory is a fully self-contained vertical slice for one domain.

## Slice structure

Every service package has the same layout:

| File           | Role                                                                         |
| -------------- | ---------------------------------------------------------------------------- |
| `service.go`   | Service interface + implementation; all business logic lives here            |
| `store.go`     | Store interfaces (what the service needs from the DB) + GORM implementations |
| `handler.go`   | HTTP handler — implements `handler.RouteGroup`, delegates to Service         |
| `types.go`     | Domain types: input/output structs, filter types                             |
| `validator.go` | Input validation — called by handler before service                          |
| `wire.go`      | Wire provider set for this domain                                            |

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

- Store interfaces are defined in `{domain}/store.go` — the service only depends on the interfaces it needs
- GORM implementations live in the same file — no separate package for implementations
- All store methods take `context.Context` and use `tracing.Start`/`tracing.End`
- Use `store.IsNotFound(err)` to distinguish "not found" from real errors

## Handler layer rules

- Handlers only parse, validate, call service, and respond — no business logic
- All responses use `handler.Respond`, `handler.RespondPaged`, or `handler.HandleError`
- Validation happens before any service call: `h.validator.ValidateX(input)` → `handler.HandleError` on failure

## Wire

Each package exports a `ProviderSet` for Google Wire. The root `cmd/api/wire.go` composes all provider sets.
