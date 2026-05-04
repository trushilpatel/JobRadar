# Phase 2 — Shared contracts

Phase 2 = hard-stop cutover: per-user data model, route split, companies split, workers update.

Every Phase 2 bucket must honor these contracts EXACTLY. They are the integration boundary between buckets running in parallel.

---

## Final tables (after migration)

### Renamed: `job_feed` → `jobs` (global, no per-user state)
Drop columns: `status`, `saved_at`, `applied_at`, `dismissed_at`, `notes`. Keep everything else (id, company_id, title, url, location, posted_at, description, tech_tags, etc.).

### NEW: `user_job_state` (per-user job interaction)
```
user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE
job_id            UUID NOT NULL REFERENCES jobs(id)  ON DELETE CASCADE
status            TEXT NOT NULL CHECK (status IN ('saved','applied','dismissed'))
notes             TEXT
dismissal_reason  TEXT
dismissal_note    TEXT
saved_at          TIMESTAMPTZ
applied_at        TIMESTAMPTZ
dismissed_at      TIMESTAMPTZ
created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
PRIMARY KEY (user_id, job_id)
```
Indexes: `(job_id)` (reverse lookup); partial `(user_id, saved_at DESC) WHERE status='saved'`; partial `(user_id, applied_at DESC) WHERE status='applied'`.

**Critical:** absence of row = `new`. Status enum does NOT include `new`.

### NEW: `company_scan_settings` (admin-only, operational)
```
company_id              UUID PRIMARY KEY REFERENCES companies(id) ON DELETE CASCADE
adapter_type            TEXT NOT NULL
adapter_config          JSONB
scan_interval_override  INT
active                  BOOL NOT NULL DEFAULT true
last_scan_at            TIMESTAMPTZ
last_success_at         TIMESTAMPTZ
last_error_at           TIMESTAMPTZ
last_error_message      TEXT
consecutive_errors      INT NOT NULL DEFAULT 0
created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
```

### Modified: `companies` (drop operational columns)
Drop: `adapter_type`, `adapter_config`, `scan_interval_override`/`scan_interval_minutes`, `active`, `last_scan_at`/`last_success_at`/`last_error_*`, `consecutive_errors`, etc.
Keep: `id`, `name`, `slug`, `website`, `careers_url`, `logo_url`, `created_at`, `updated_at`.
**Existing companies' scan settings are NOT backfilled** — they start without scan_settings rows. Workers must skip companies without settings (logged once per skip). Admin re-onboards via UI in Phase 3.

### Modified: `job_application_tracker`
Add `user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE`. Existing rows are wiped before the column is set NOT NULL (per "drop per-user state on cutover" decision in the rollout plan).
Indexes: `(user_id)`, `(user_id, job_id)`.

### DROPPED: `job_dismissal_log`
Reason + note now live on `user_job_state`.

### Untouched (stay global)
`company_scan_queue`, `company_scan_log`, `adapter_configs`, `job_pipeline_ledger`, `job_analytics`.

---

## Cascade rules summary

| FK | On delete | Why |
|---|---|---|
| `user_job_state.user_id` | CASCADE | User gone → state gone |
| `user_job_state.job_id` | CASCADE | Job purged → state irrelevant |
| `job_application_tracker.user_id` | CASCADE | User gone → applications gone |
| `company_scan_settings.company_id` | CASCADE | Company gone → settings gone |

---

## Service interfaces (signature changes)

All user-scoped service methods now take `userID` from context (services pull via `identity.UserIDFrom(ctx)`); repositories take `userID` as **explicit** parameter.

### `service/job/services/JobService.go`
```go
type JobService interface {
    List(ctx context.Context, filter JobFilter, cursor string, limit int) (PageResult, error)
    Get(ctx context.Context, id string) (*JobView, error)
    SetSaved(ctx context.Context, id string, saved bool) error
    SetStatus(ctx context.Context, id string, status string) error      // saved|applied|dismissed
    SetNotes(ctx context.Context, id string, notes string) error
    Dismiss(ctx context.Context, id string, reason, note string) error
    BulkDismiss(ctx context.Context, ids []string, reason, note string) error
    GetCounts(ctx context.Context) (Counts, error)                       // per-user counts
}
```
The `JobView` returned to user includes `UserState UserJobStateView` (joined from user_job_state, may be empty/`new`).

### NEW `service/job/services/UserJobStateRepositoryService.go`
```go
type UserJobStateRepository interface {
    Get(ctx context.Context, userID, jobID string) (*UserJobState, error)
    Upsert(ctx context.Context, userID, jobID string, state UpsertState) error
    UpsertBulkDismiss(ctx context.Context, userID string, jobIDs []string, reason, note string) error
    Delete(ctx context.Context, userID, jobID string) error
    CountsByUser(ctx context.Context, userID string) (Counts, error)
}
```

### `service/application/services/ApplicationService.go`
Every method takes ctx; pulls userID from ctx; passes userID to repo. Methods unchanged in name.

### `service/company/services/CompanyService.go` — **split into two interfaces**
```go
type CompanyService interface {            // PUBLIC — read-only by users
    ListPublic(ctx context.Context, filter CompanyFilter, page, limit int) (PublicListResult, error)
    GetPublic(ctx context.Context, id string) (*CompanyPublicView, error)
}

type CompanyAdminService interface {       // ADMIN — write + operational read
    List(ctx context.Context, filter CompanyFilter, page, limit int) (AdminListResult, error)  // joins scan_settings
    Get(ctx context.Context, id string) (*CompanyAdminView, error)
    Create(ctx context.Context, public CompanyPublicInput, settings CompanyScanSettingsInput) (*Company, error)
    UpdatePublic(ctx context.Context, id string, p CompanyPublicInput) error
    UpdateScanSettings(ctx context.Context, id string, p CompanyScanSettingsInput) error
    Delete(ctx context.Context, id string) error
    SetAllActive(ctx context.Context, active bool) error
    GetByIDs(ctx context.Context, ids []string) ([]CompanyView, error)   // existing
    GetAll(ctx context.Context) ([]CompanyView, error)                   // existing — workers
}
```
`CompanyPublicView` = lean public fields only. `CompanyAdminView` = public + scan_settings JOINed.

### NEW `service/company/services/CompanyScanSettingsRepositoryService.go`
```go
type ScanSettingsRepository interface {
    Get(ctx context.Context, companyID string) (*CompanyScanSettings, error)
    GetMissing(ctx context.Context) ([]string, error)    // admin: company IDs without settings
    Create(ctx context.Context, s CompanyScanSettings) error
    Update(ctx context.Context, companyID string, p CompanyScanSettingsInput) error
    UpdateScanState(ctx context.Context, companyID string, st ScanStateUpdate) error  // worker writes
    Delete(ctx context.Context, companyID string) error
}
```

### `service/dashboard/services/DashboardService.go` — **split**
```go
type DashboardService interface {
    GetUserStats(ctx context.Context) (UserStats, error)        // pulls userID from ctx
    GetUserDailyActivity(ctx context.Context, days int) ([]DayPoint, error)
}

type PipelineDashboardService interface {                       // admin-only
    GetPipelineStats(ctx context.Context) (PipelineStats, error)
}
```

### `service/insights/` — moved to admin
No interface change; mounted under `/admin/insights/*`.

### `service/scan/`
Reads from `CompanyScanSettings` instead of `companies` for adapter_type / adapter_config / scan_interval_override / active.

### NEW admin endpoints in `service/user/`
Extend existing UserService with admin methods (or add separate UserAdminService — agent's choice based on slice convention):
```go
List(ctx context.Context, filter UserFilter, page, limit int) (ListResult, error)
GetWithRoles(ctx context.Context, id string) (*UserWithRoles, error)
SetRoles(ctx context.Context, id string, roleIDs []string, grantedBy string) error
SetStatus(ctx context.Context, id string, status string) error
AdminDelete(ctx context.Context, id string) error
```

### NEW admin endpoints in `service/audit/`
```go
ListAdminAuditLog(ctx context.Context, filter ListFilter) (entries []Entry, total int64, err error)
```

---

## Final routes (route → method → middleware → handler)

### `/api/*` — RequireIdentity + RateLimit
```
GET    /api/me                                → user.RestService.getMe
PATCH  /api/me                                → user.RestService.patchMe
DELETE /api/me                                → user.RestService.deleteMe

GET    /api/jobs                              → job.RestService.list      (keyset pagination + Redis page cache)
GET    /api/jobs/counts                       → job.RestService.counts    (per-user, cached)
GET    /api/jobs/{id}                         → job.RestService.get
PATCH  /api/jobs/{id}/save                    → job.RestService.setSaved
PATCH  /api/jobs/{id}/status                  → job.RestService.setStatus
PATCH  /api/jobs/{id}/notes                   → job.RestService.setNotes
DELETE /api/jobs/{id}                         → job.RestService.dismiss
POST   /api/jobs/dismiss/bulk                 → job.RestService.bulkDismiss

GET    /api/jobs/{id}/applications            → application.RestService.listByJob
POST   /api/jobs/{id}/applications            → application.RestService.create
PATCH  /api/jobs/{id}/applications/{app_id}/stage → application.RestService.updateStage
PATCH  /api/jobs/{id}/applications/{app_id}/note  → application.RestService.updateNote
GET    /api/applications                      → application.RestService.listAll
PATCH  /api/applications/{id}/stage           → application.RestService.updateStageByID

GET    /api/companies                         → company.PublicRestService.list
GET    /api/companies/{id}                    → company.PublicRestService.get

GET    /api/dashboard/me                      → dashboard.RestService.getUserStats
```

### `/admin/*` — RequireIdentity + RequireRole("admin") + RateLimit + AuditAdminMutation
```
GET    /admin/companies                       → company.AdminRestService.list
POST   /admin/companies                       → company.AdminRestService.create
GET    /admin/companies/{id}                  → company.AdminRestService.get
PATCH  /admin/companies/{id}                  → company.AdminRestService.updatePublic
PATCH  /admin/companies/{id}/scan-settings    → company.AdminRestService.updateScanSettings
DELETE /admin/companies/{id}                  → company.AdminRestService.delete
PATCH  /admin/companies/active                → company.AdminRestService.setAllActive

GET    /admin/companies/queue                 → scan.RestService.listQueue
DELETE /admin/companies/queue/{entry_id}      → scan.RestService.removeFromQueue
POST   /admin/companies/{id}/scan             → scan.RestService.triggerScan
POST   /admin/companies/scan-all              → scan.RestService.triggerScanAll
GET    /admin/companies/{id}/scan-stats       → scan.RestService.scanStats
GET    /admin/companies/{id}/analytics        → scan.RestService.analytics

GET    /admin/adapter-configs                 → adapterconfig.RestService.list
PATCH  /admin/adapter-configs/{adapter_type}  → adapterconfig.RestService.update
PATCH  /admin/companies/{id}/scan-interval-override → adapterconfig.RestService.updateOverride
GET    /admin/companies/{id}/scan-config      → adapterconfig.RestService.getEffective

GET    /admin/insights/*                      → insights.RestService.* (all routes moved)

GET    /admin/dashboard/pipeline              → dashboard.PipelineRestService.getPipelineStats

GET    /admin/users                           → user.AdminRestService.list
GET    /admin/users/{id}                      → user.AdminRestService.get
PATCH  /admin/users/{id}/roles                → user.AdminRestService.setRoles
PATCH  /admin/users/{id}/status               → user.AdminRestService.setStatus
DELETE /admin/users/{id}                      → user.AdminRestService.delete

GET    /admin/audit-log                       → audit.AdminRestService.list
```

### Routes that go away
- The legacy global routes (`/jobs`, `/companies`, etc.) — replaced by /api or /admin equivalents.
- The legacy `RequireAuth` + email allowlist — REMOVED. All routes use `RequireIdentity` (and `RequireRole("admin")` where applicable).
- `Auth` middleware (legacy) is deleted in Phase 2.

---

## Performance — feed cache (Bucket 2 owns)

- Use `keyset pagination` on /api/jobs (cursor based on `posted_at, id`)
- Redis page cache key: `feed:page:{cursor}` → list of denormalized `[]JobBase` (without per-user state). 30s TTL.
- On request: Redis hit → fetch `user_job_state` rows for those job IDs (single `WHERE user_id=$1 AND job_id IN (...)` query) → merge in Go → return.
- Cache invalidated only by new job ingestion (first cache page).
- Per-user counts cached separately at `user:{id}:counts` with short TTL (~10s).

Implement under `service/job/services/JobImplService.go` calling into `cache.Cache`. Single-flight via `cache.GetOrLoad`.

---

## Cutover (hard stop, big-bang)

```
1. make k8s-down
2. cd backend && goose -dir migrations postgres "<dsn>" up
3. make k8s-build
4. make k8s-up
5. Verify scheduler skips unsettled companies
6. Sign in as admin → re-add scan_settings via /admin/companies/{id}/scan-settings
```

---

## File ownership map (no overlap)

| Bucket | Owns |
|---|---|
| **B1: Migrations** | `backend/migrations/<NNN>_phase2_*.sql` (one or more files) |
| **B2: Job + UserJobState** | `backend/service/job/` (all subdirs: models/, services/, dto/, common/, tests/, wire.go); add `UserJobState` model and repo; rename Job model from job_feed shape to jobs shape; refactor handlers to take userID from ctx; add feed cache + keyset pagination |
| **B3: Application** | `backend/service/application/` (all of it); add user_id wiring throughout |
| **B4: Company split** | `backend/service/company/` (all of it); add `CompanyScanSettings` model + repo; split into Public + Admin services + RestRouters |
| **B5: Workers** | `backend/apps/scheduler/`, `backend/apps/crawler/`; read scan_settings; graceful skip when missing |
| **B6: Dashboard + Insights + admin endpoints** | `backend/service/dashboard/` (split user vs pipeline); `backend/service/insights/` (no logic changes — just route prefix); admin endpoints in `service/user/services/User{Admin*}.go` and `service/audit/services/Audit{Admin*}.go` |
| **B7: Router integration** | `backend/apps/api/router.go` (or equivalent); `multiuser_setup.go`; `main.go`; final `/api/*` and `/admin/*` mounting; deletion of legacy `Auth` / `RequireAuth` middleware; tests in `apps/api/app_test.go` updated |

Insights stays in service/insights/ — only its mount path changes (handled by B7).

---

## Test obligations per bucket

- Every user-scoped repo method gets `testutil.AssertUserScoped(t, name, fn)` test
- Every service method that touches user data gets a unit test using `identity.WithTestIdentity(ctx, "test-user", "user")`
- `RequireRole("admin")` covered by route-level integration test
- Worker buckets: integration test that verifies graceful skip on missing scan_settings

---

## Don't forget

- Service slice rules: `Rest → Validation → Cache → Impl → Repo`. Cache is optional; add it where performance matters (jobs feed, user, dashboard, company public list).
- All errors via `domain.ServiceError` + `WithMessage`
- All logging via `platform/logger.Logger`
- Field constants from `platform/logger/fields.go`
- All HTTP responses via `handler.Respond[T]` / `handler.RespondPaged[T]` / `handler.HandleError`
- No HTTP types outside RestService and RestRouter
- No interface declarations outside `{Domain}Service.go`
- No commits per bucket — orchestrator commits atomically after QA
