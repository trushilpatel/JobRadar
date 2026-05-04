# Multi-User Rollout — Implementation Plan

**Status:** Design locked, ready to execute.
**Cutover:** Hard-stop, big-bang. Local k3d only — no production users.
**Branch:** `claude/secure-api-multiuser-IQbZ2`

---

## Locked decisions

- Companies + crawling = global infrastructure. Every user sees every job (no follow/subscribe v1, no per-user relevance v1).
- Magic-link auth only (Supabase). Open signup. Admin via env var `ADMIN_EMAILS` matched on first login → user gets `user` + `admin` roles.
- Hard delete (sync to Supabase via service-role key). `users.status` = `active|suspended` for ban-without-delete.
- RBAC0: `roles` + `user_roles`. Permissions table deferred.
- Routes split: `/api/*` (RequireAuth) and `/admin/*` (RequireAuth + RequireRole("admin")).
- Per-user rate limit: 30 rpm GET, 20 rpm write. Method-derived scope. Redis token bucket. Fail-open on Redis outage.
- Decorator chain: `Rest → Validation → Cache → Impl → Repo`. Cache TTL 60s via Redis; invalidate on write. Single-flight to avoid stampedes.
- Job feed: keyset pagination + Redis page cache (30s TTL) + per-user state overlay merged in Go.
- Companies split: public table + `company_scan_settings` (admin-only). Existing companies start without scan_settings; re-add via admin UI.
- Per-user state dropped on cutover. Global crawl history kept.
- `job_dismissal_log` dropped; reason/note folded into `user_job_state`.
- Identity flows via `context.Context`. Repos take `userID` explicit; services pull from context. Test guard: every user-scoped repo method asserts on empty userID.

## Final tables

**New:** `users`, `roles`, `user_roles`, `user_job_state`, `company_scan_settings`, `admin_audit_log`
**Renamed:** `job_feed` → `jobs` (per-user columns dropped)
**Modified:** `job_application_tracker` adds `user_id NOT NULL` (existing rows wiped); `companies` drops operational columns
**Dropped:** `job_dismissal_log`
**Untouched:** `company_scan_queue`, `company_scan_log`, `adapter_configs`, `job_pipeline_ledger`, `job_analytics`

## Final routes

`/api/*` (18 routes — `me`, `jobs`, `applications`, `companies` read, `dashboard/me`)
`/admin/*` (22 routes — companies write, scans, queue, adapter-configs, insights, users, audit-log, dashboard/pipeline)

---

## Phase 1 — Auth foundations & platform primitives

**Outcome:** Server authenticates users, identity flows through context, `/me` works. Nothing user-facing changes.

**Additive only — no breaking changes. Can ship without downtime.**

### What ships
- Migrations: `users`, `roles`, `user_roles`, `admin_audit_log`
- `backend/common/identity/` — Identity struct, FromContext/MustFromContext/UserIDFrom/IsAdmin/HasRole/WithTestIdentity
- `backend/platform/cache/` — Cache interface + Redis/Memory/Noop impls + `GetOrLoad[T]` with single-flight
- `backend/platform/ratelimit/` — Limiter interface + Redis/Memory impls + scope-derived middleware
- `backend/platform/supabaseadmin/` — `DeleteUser(ctx, userID)`
- `middleware/auth.go` extended — DB lookup, lazy provision, ADMIN_EMAILS match, Identity context attach (via UserCacheService)
- `middleware/role.go` — `RequireRole(role)`
- `middleware/ratelimit.go` — method → scope → Redis bucket
- `middleware/audit.go` — append admin_audit_log on admin mutations
- `service/user/` — UserService (Get, Provision, UpdateProfile, Delete) + UserCacheService decorator
- `service/role/` — RoleService (Grant, Revoke, ListForUser, ListAll)
- `service/audit/` — AuditService (Append, List)
- Endpoints: `GET /api/me`, `PATCH /api/me`, `DELETE /api/me`

### Parallel buckets
1. **Migrations** — schema + seed `roles`
2. **Platform packages** — identity, cache, ratelimit, supabaseadmin
3. **Middleware** — auth (extend), role, ratelimit, audit
4. **User/Role/Audit services** — three sibling service slices
5. **`/me` endpoints** — handler + router wire-up

### Definition of done
- [ ] Migrations run cleanly
- [ ] First-login provisions `users` + `user_roles` row; `ADMIN_EMAILS` match adds `admin`
- [ ] `GET /api/me` returns full identity bundle
- [ ] `DELETE /api/me` deletes Supabase auth then DB cascade
- [ ] Rate limit returns 429 + `Retry-After`; Redis outage falls open
- [ ] Cache decorators hit/miss/invalidate correctly
- [ ] All Phase 1 unit tests pass; lint clean

---

## Phase 2 — Multi-user data model & route segregation

**Outcome:** Every existing route user-scoped. Admin/user route trees split. Workers handle company split.

**Hard-stop cutover.** Cannot ship piecemeal.

### What ships
- Migrations: rename `job_feed` → `jobs`; drop per-user columns; create `user_job_state`; create `company_scan_settings` (empty); drop `job_dismissal_log`; `job_application_tracker.user_id` NOT NULL after wipe; drop operational columns from `companies`
- `service/job/` refactor: feed query LEFT JOINs `user_job_state`; save/status/notes/dismiss upsert into `user_job_state`; bulk dismiss
- `service/application/` refactor: every method takes userID
- `service/company/` split: public path + admin path; CompanyCacheService decorator
- `service/scan/` refactor: read scan settings from new table
- `service/dashboard/` split: `GetUserStats(ctx, userID)` + `GetPipelineStats(ctx)` + cache decorators
- Workers: scheduler + crawler read `company_scan_settings`; companies without settings skipped (logged)
- Router split: `/api/*` and `/admin/*`; insights moved to admin; new pipeline-dashboard endpoint
- Job feed: keyset pagination + Redis page cache (30s) + per-user overlay merge

### Parallel buckets
1. **Schema** — all phase 2 migrations in one goose file
2. **Job + UserJobState** — repo, service, handlers, feed cache integration
3. **Application** — repo, service, handlers
4. **Company split** — companies repo, scan_settings repo, cache decorator, public + admin handlers
5. **Workers** — scheduler + crawler reading scan_settings; graceful skip
6. **Dashboard split** — user dashboard + pipeline dashboard with caches
7. **Router split** — register all routes under correct trees

### Definition of done
- [ ] Migrations run cleanly during hard-stop
- [ ] Every user-scoped repo method takes `userID`; userid_guard test in place
- [ ] `/api/*` routes return only requesting user's data
- [ ] `/admin/*` routes return 403 for non-admins (verified by test)
- [ ] Workers process companies that have scan_settings; skip those without (one-line log per skip)
- [ ] Job feed cursor-paginates; Redis page cache hits; user state correctly overlaid
- [ ] Cache decorators wired for company, user, dashboard
- [ ] All Phase 2 tests pass; lint clean

---

## Phase 3 — Frontend rewiring, admin UI & polish

**Outcome:** End-to-end usable. Users sign up and use. Admins manage from the console.

**Frontend-only deploy. No schema or API changes.**

### What ships
- Auth: `/me` query on session ready; identity + roles in Zustand
- Layout: admin nav hidden when role lacks `admin`; admin route group with shell
- Job feed: `useInfiniteQuery` with cursor; per-user state badges (saved/applied/dismissed)
- Dashboard: user view consumes `/api/dashboard/me`; pipeline view in admin
- Account page: edit display_name; delete account with confirm modal (Supabase signs out post-delete)
- Admin Users page: list + filter + role grant/revoke + suspend + force-delete
- Admin Audit log: paginated, filter by actor + action
- Admin Company scan-settings: re-onboard existing companies (add scan_settings rows)
- Insights moved into admin section
- 429 toast in `apiClient`
- E2E: signup → feed → save/dismiss/apply → account delete; admin grant/revoke; non-admin blocked from `/admin/*` URLs

### Parallel buckets
1. **Auth + `/me`** — store, query hook, role flag
2. **Layout + nav guards** — admin shell, hidden routes
3. **Account page** — profile edit + delete flow
4. **Admin Users** — list, filters, role management, suspend, delete
5. **Admin Audit** — paginated log viewer
6. **Admin Scan-settings** — per-company form + table
7. **Feed/dashboard rewiring** — keyset pagination, per-user wiring
8. **Polish** — 429 toast, error boundaries, loading skeletons
9. **E2E suite** — Playwright tests

### Definition of done
- [ ] User can sign up via magic link and use the app
- [ ] User can edit profile, delete account
- [ ] Admin can manage users, view audit log, re-add scan_settings, view pipeline
- [ ] Non-admin URLs to `/admin/*` are blocked client-side (UX) and server-side (security — verified)
- [ ] E2E suite green; frontend lint + typecheck clean

---

## Cutover runbook (between Phase 1 ship and Phase 2 deploy)

```
1. make k8s-down                    # stop API + workers
2. cd backend && goose up           # run all phase 2 migrations
3. make k8s-build                   # build + import new images
4. make k8s-up                      # start with new schema
5. make k8s-logs-crawler            # verify scheduler skips unsettled companies cleanly
6. Sign in as admin → re-onboard companies via /admin/companies/{id}/scan-settings
```

Phase 1 ships normally (additive). Phase 3 ships normally (frontend-only).

---

## Test plan (cross-phase)

| Layer | Test type | Scope |
|---|---|---|
| `identity` helpers | Unit | Context attach/read; admin/role checks |
| Repo userID guard | Unit | Every user-scoped repo method asserts on empty userID |
| Auth + role middleware | Integration | Provisioning, ADMIN_EMAILS, role gating, status=suspended block |
| Cache decorator | Unit | Hit, miss, write-invalidate, Redis-down pass-through |
| Rate limit middleware | Unit + integration | Bucket exhaustion, Retry-After, Redis-down fail-open |
| Service layer | Unit | All user-scoped services with `WithTestIdentity` |
| Worker | Integration | Scheduler/crawler with mock scan_settings; skip-on-missing path |
| E2E | Playwright | Signup, feed, account delete, admin gates, role grant/revoke |

---

## Deferred (not in v1)

- Permissions table (RBAC1)
- Per-user relevance filter
- Follow/subscribe to companies
- User-submitted companies
- Cache pub/sub invalidation across instances (Redis is shared — not needed yet)
- Cache key strategy for filtered list queries (revisit when filters land)
- Admin moderator role
- Multi-region / multi-tenant
