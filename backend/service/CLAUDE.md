# service/ — Feature-slice packages

Each subdirectory is a self-contained vertical slice for one domain.

## Slice layout

```
{domain}/
├── wire.go
├── common/{Domain}ServiceConstants.go     # Domain constants (package common)
├── dto/{TypeName}.go                      # One file per input/output type (package dto)
├── models/{table_name}.go                 # One file per DB table — GORM struct, package {domain}model
├── services/
│   ├── {Domain}Service.go                 # CONTRACT — single interface + BaseService only
│   ├── {Domain}RepositoryService.go       # DB terminal — struct + GORM methods, no next, no BaseService
│   ├── {Domain}ImplService.go             # Business logic — embeds BaseService, overrides what it owns
│   ├── {Domain}ValidationService.go       # Input validation — embeds BaseService
│   ├── {Domain}RestService.go             # HTTP handlers — has next {Domain}Service, no routing
│   └── {Domain}RestRouter.go              # Route registration only — Register(r chi.Router)
└── tests/{Domain}{Role}_test.go           # Tests mirror the services/ file they cover
```

**Model package naming:** each `models/` dir uses a domain-prefixed package — `jobmodel`, `companymodel`, `appmodel`, `scanmodel`. Avoids import alias clashes when a file needs structs from multiple service domains.

---

## Decorator chain

Every layer wraps the next via a single shared contract interface.

```
RestRouter → RestService → ValidationService → CacheService → ImplService → RepositoryService
                ↓               ↓                  ↓               ↓             ↓
           (has next)      (BaseService)       (BaseService)   (BaseService)  (terminal — no next)
```

`CacheService` is **optional** — present only in slices reading user-scoped or hot data (e.g. `service/user/`). It embeds `BaseService`, overrides reads to call `cache.GetOrLoad`, and invalidates on writes. Backed by `backend/platform/cache.Cache` (Redis in prod, Memory/Noop in tests).

### Contract file — `{Domain}Service.go`

The **only** file allowed to declare interfaces in a domain. Contains:

1. The single domain interface (e.g. `ApplicationService`)
2. `BaseService` struct — auto-delegates every interface method to `next`

```go
type ApplicationService interface {
    ListByJob(ctx context.Context, jobID string) ([]model.Job, error)
    Create(ctx context.Context, input dto.CreateInput) error
}

type BaseService struct{ next ApplicationService }

func NewBaseService(next ApplicationService) BaseService {
    if next == nil { panic("BaseService: next must not be nil — check wire setup") }
    return BaseService{next: next}
}

// One pass-through method per interface method
func (b *BaseService) ListByJob(ctx context.Context, jobID string) ([]model.Job, error) {
    return b.next.ListByJob(ctx, jobID)
}
```

### RepositoryService — terminal

Implements **all** interface methods by hitting the DB. No `next`, no `BaseService` — end of the chain. Compiler enforces completeness. Constructor returns the domain interface.

```go
type repositoryService struct{ db *gorm.DB }
func NewRepositoryService(db *gorm.DB) ApplicationService { return &repositoryService{db: db} }
```

### ImplService — business logic

Embeds `BaseService`. Overrides only methods needing orchestration or cross-cutting logic; unoverridden methods auto-delegate.

```go
type implService struct{ BaseService }
func NewImplService(next ApplicationService) ApplicationService {
    return &implService{BaseService: NewBaseService(next)}
}
func (s *implService) Create(ctx context.Context, input dto.CreateInput) error {
    // business logic, then…
    return s.next.Create(ctx, input)
}
```

### ValidationService — input validation

Same shape as `ImplService` — embeds `BaseService`, overrides only methods that need validation. Methods without validation rules pass through unmodified.

### RestService — HTTP adapter

Has `next {Domain}Service` as its **only** dependency. Parses request → calls `next` → responds. Does **not** implement the domain interface. No routing logic — that belongs in `RestRouter`.

### RestRouter — route registration

Depends on `*RestService`. Only responsibility is `Register(r chi.Router)` — no business logic, no parsing, no responses.

### Wire setup

```go
repo    := NewRepositoryService(db)
impl    := NewImplService(repo)
valid   := NewValidationService(impl)
handler := NewRestService(valid)
router  := NewRestRouter(handler)
```

---

## Enforcement rules

- `{Domain}Service.go` is the **only** file in a domain that may declare `type X interface`
- No interfaces in `RepositoryService.go`, `ImplService.go`, `ValidationService.go`, or `RestService.go`
- Every constructor except `NewRestService` and `NewRestRouter` returns the domain interface
- `RepositoryService` has no `next` field and does not embed `BaseService`
- `RestService` has `next {Domain}Service` but does not implement the domain interface
- `RestRouter` owns `Register(r chi.Router)` — never appears on `RestService`
- Every struct in `models/` must be a GORM model: it must have a `TableName()` method and GORM struct tags. Plain domain types (DTOs, config structs, value objects) belong in `dto/` — placing them in `models/` forces non-repository layers to import the models package, violating the GORM isolation rule

---

## Packages

| Package        | Owns                                                                                                                                           |
| -------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `job/`         | Job listing: list, get, dismiss, save, set status, set notes                                                                                   |
| `company/`     | Company CRUD, scan trigger, scan queue reads                                                                                                   |
| `application/` | Application tracker: insert, update stage/note                                                                                                 |
| `scan/`        | Scan result recording — used by the crawler worker, not the API                                                                                |
| `user/`        | User profile (Get / Provision / UpdateProfile / Delete), cache decorator, `/api/me`                                                            |
| `audit/`       | Admin audit log (Append / List); consumed by `AuditAdminMutation` middleware                                                                   |
| `llm/`         | LLM business logic — `JobsRelevanceFilter` + `JobPostEnricher` + `ResumeKeywordExtractor`, Gemini impl, prompts                                |
| `resume/`      | Resume CRUD + object storage + best-effort keyword extraction (`textextract/` subpackage: PDF/DOCX → plain text)                               |
| `applyqueue/`  | Apply-queue list + resume attachment; computes on-read keyword-match resume recommendations (Jaccard over `job_tech_tags` vs. resume keywords) |

---

## General rules

- All list endpoints use **keyset cursor pagination** — `filter.Cursor string` in the DTO, `next_cursor *string` in the result, tuple comparison in SQL (e.g. `(sort_col, id) > (?, ?)`). Never use `OFFSET`-based pagination. **Exception:** `audit/` and `user/admin` use page+offset because they are admin-only, operate on small result sets, and support page-jump UX in the admin console. Do not extend this exception to any new slice.
- All service methods take `context.Context` as the first argument and use `tracing.Start` / `tracing.End` for OTel spans
- Services return `domain.ServiceError` via sentinel `.WithMessage()` — never `fmt.Errorf` for errors handlers inspect
- All responses use `handler.Respond`, `handler.RespondPaged`, or `handler.HandleError` — never raw `json.NewEncoder`
- No HTTP types (`http.Request`, `http.ResponseWriter`) outside `RestService` and `RestRouter`
- GORM structs live in `RepositoryService` only — never referenced by Impl, Validation, or REST layers
- A store UPDATE/DELETE that targets specific rows must check `result.RowsAffected` and return a sentinel (e.g. `ErrLedgerRowMissing`, `ErrQueueRowMissing`, `ErrNotFound`) when it matched fewer rows than expected — never return nil for a write that silently did nothing. GORM's `Update`/`Exec` return nil error on zero rows; a swallowed no-op here is what caused the phantom-`fetch` ledger bug. Idempotent inserts use `clause.OnConflict{DoNothing}` and surface the conflict (`ErrJobAlreadyExists`) when the caller needs to know.
