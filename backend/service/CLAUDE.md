# service/ — Feature-slice packages

Each subdirectory is a fully self-contained vertical slice for one domain.

## Slice structure

Every service package has the same layout:

```
{domain}/
├── wire.go
├── common/
│   └── {Domain}ServiceConstants.go    # Domain constants (package common)
├── dto/
│   └── {TypeName}.go                  # One file per input/output type (package dto)
├── models/
│   └── {table_name}.go                # One file per DB table — GORM struct, package {domain}model
├── services/
│   ├── {Domain}Service.go             # CONTRACT — single interface + BaseService only
│   ├── {Domain}RepositoryService.go   # DB terminal — struct + GORM methods, no next, no BaseService
│   ├── {Domain}ImplService.go         # Business logic — embeds BaseService, overrides what it owns
│   ├── {Domain}ValidationService.go   # Input validation — embeds BaseService, overrides what it validates
│   ├── {Domain}RestService.go         # HTTP handlers — has next {Domain}Service, no routing
│   └── {Domain}RestRouter.go          # Route registration only — Register(r chi.Router)
└── tests/
    └── {Domain}{Role}_test.go         # Tests mirroring the services/ file they cover
```

**Model package naming:** each `models/` directory uses a domain-prefixed package name — `jobmodel`, `companymodel`, `appmodel`, `scanmodel`. This prevents import alias conflicts when a file needs structs from multiple service domains.

## Service communication pattern — decorator chain

Each domain uses a **decorator chain** where every layer wraps the next via a single shared contract interface.

### Chain order

```
RestRouter → RestService → ValidationService → ImplService → RepositoryService
                ↓               ↓                  ↓               ↓
           (has next)      (BaseService)       (BaseService)    (terminal — no next)
```

### Contract file — `{Domain}Service.go`

The **only** file in a domain allowed to define interfaces. Contains:

1. The single domain interface (e.g. `ApplicationService`)
2. `BaseService` struct — auto-delegates all methods to `next`

```go
type ApplicationService interface {
    ListByJob(ctx context.Context, jobID string) ([]model.Job, error)
    Create(ctx context.Context, input dto.CreateInput) error
    // ...
}

type BaseService struct {
    next ApplicationService
}

func NewBaseService(next ApplicationService) BaseService {
    if next == nil {
        panic("BaseService: next must not be nil — check wire setup")
    }
    return BaseService{next: next}
}

// All methods delegate to next — layers override only what they own
func (b *BaseService) ListByJob(ctx context.Context, jobID string) ([]model.Job, error) {
    return b.next.ListByJob(ctx, jobID)
}
// ... one delegation method per interface method
```

### RepositoryService — terminal

- Implements **all** interface methods by hitting the DB
- No `next`, no `BaseService` — it is the end of the chain
- Compiler enforces completeness — every interface method must be implemented here
- Returns the domain interface from its constructor

```go
type repositoryService struct { db *gorm.DB }

func NewRepositoryService(db *gorm.DB) ApplicationService {
    return &repositoryService{db: db}
}
```

### ImplService — business logic

- Embeds `BaseService`
- Overrides only the methods that require orchestration or cross-cutting logic
- Unoverridden methods auto-delegate to `next` (RepositoryService) via `BaseService`

```go
type implService struct{ BaseService }

func NewImplService(next ApplicationService) ApplicationService {
    return &implService{BaseService: NewBaseService(next)}
}

func (s *implService) Create(ctx context.Context, input dto.CreateInput) error {
    // business logic, then...
    return s.next.Create(ctx, input)
}
```

### ValidationService — input validation

- Embeds `BaseService`
- Overrides methods that require input validation — validates, then calls `next`
- Methods with no validation rules are not overridden; `BaseService` passes them through

```go
type validationService struct{ BaseService }

func NewValidationService(next ApplicationService) ApplicationService {
    return &validationService{BaseService: NewBaseService(next)}
}

func (v *validationService) Create(ctx context.Context, input dto.CreateInput) error {
    if err := validateCreate(input); err != nil { return err }
    return v.next.Create(ctx, input)
}
```

### RestService — HTTP adapter

- Has `next ApplicationService` as its **only** dependency
- Parses HTTP request → calls `next` → responds
- Does **not** implement the domain interface — it is not part of the business chain
- No routing logic — routing belongs in `RestRouter`

```go
type RestService struct{ next ApplicationService }

func NewRestService(next ApplicationService) *RestService {
    return &RestService{next: next}
}

func (h *RestService) create(w http.ResponseWriter, r *http.Request) {
    // parse → call next → respond
}
```

### RestRouter — route registration

- Depends on `*RestService`
- Only responsibility: mount routes via `Register(r chi.Router)`
- No business logic, no parsing, no responses

```go
type RestRouter struct{ handler *RestService }

func NewRestRouter(h *RestService) *RestRouter {
    return &RestRouter{handler: h}
}

func (r *RestRouter) Register(router chi.Router) {
    router.Get("/applications", r.handler.list)
    router.Post("/applications", r.handler.create)
}
```

### Wire setup

```go
repo    := NewRepositoryService(db)
impl    := NewImplService(repo)
valid   := NewValidationService(impl)
handler := NewRestService(valid)
router  := NewRestRouter(handler)
```

## Enforcement rules

- `{Domain}Service.go` is the **only** file allowed to contain `type X interface` declarations
- No interface definitions in `RepositoryService.go`, `ImplService.go`, `ValidationService.go`, or `RestService.go`
- Every constructor except `NewRestService` and `NewRestRouter` returns the domain interface
- `RepositoryService` has no `next` field and does not embed `BaseService`
- `RestService` has `next {Domain}Service` but does not implement the domain interface
- `RestRouter` owns `Register(r chi.Router)` — this method must not appear on `RestService`

## Packages

| Package        | Owns                                                                                    |
| -------------- | --------------------------------------------------------------------------------------- |
| `job/`         | Job listing: list, get, dismiss, save, set status, set notes                            |
| `company/`     | Company CRUD, scan trigger, scan queue reads                                            |
| `application/` | Application tracker: insert, update stage/note                                          |
| `dashboard/`   | Dashboard stats (job counts, daily activity) + pipeline status (DB stats + Kafka lag)   |
| `insights/`    | Analytics queries: tech stack, funnel, location, sponsorship, velocity, role categories |
| `scan/`        | Scan result recording — used by the crawler pipeline worker (not the API)               |

## General rules

- All service methods take `context.Context` as first argument and use `tracing.Start`/`tracing.End` for OTel spans
- Services return `domain.ServiceError` (via sentinel `.WithMessage()`) — never `fmt.Errorf` for errors handlers inspect
- All responses use `handler.Respond`, `handler.RespondPaged`, or `handler.HandleError` — never raw `json.NewEncoder`
- No HTTP types (`http.Request`, `http.ResponseWriter`) outside `RestService` and `RestRouter`
- GORM structs only in `RepositoryService` — never referenced by Impl, Validation, or REST layers
