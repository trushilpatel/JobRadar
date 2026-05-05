# Multi-User Rollout — Status

**Phase 1:** Complete  
**Phase 2:** Complete  
**Phase 3:** Complete (pending E2E)

---

## What shipped

### Phase 1 — Auth foundations
- `users`, `roles`, `user_roles`, `admin_audit_log` migrations
- `backend/common/identity/` — Identity struct, context helpers
- `backend/platform/cache/` — Redis/Memory/Noop impls + single-flight `GetOrLoad`
- `backend/platform/ratelimit/` — Redis/Memory token bucket + method-scoped middleware
- `backend/platform/supabaseadmin/` — `DeleteUser`
- Middleware: `RequireIdentity`, `RequireRole`, `RateLimit`, `AuditAdminMutation`
- Services: `user/`, `role/`, `audit/`
- Endpoints: `GET /api/me`, `PATCH /api/me`, `DELETE /api/me`

### Phase 2 — Multi-user data model
- Migrations: `job_feed` → `jobs`, `user_job_state`, `company_scan_settings`, `job_dismissal_log` dropped, `job_application_tracker.user_id NOT NULL`
- Job service: feed LEFT JOINs `user_job_state`, cursor (keyset) pagination, per-user overlay
- Application, company (public/admin split), scan, dashboard services refactored
- Workers skip companies with no `company_scan_settings` row (logged)
- Route split: `/api/*` (RequireIdentity + RateLimit) and `/admin/*` (+ RequireRole("admin") + AuditAdminMutation)

### Phase 3 — Frontend
- Auth store: `/me` identity + `isAdmin` in Zustand; 429 toast in `apiClient`
- `RequireAdmin` client-side guard; admin routes in `App.tsx`
- Sidebar: user nav + role-gated admin section (Pipeline, Companies, Users, Audit Log, Insights)
- Account page: edit `display_name`, delete account with confirm modal
- Admin Pipeline page: 8 KPI stats, 60s auto-refresh
- Admin Companies page: list with scan_settings status, inline configure/edit modal per company, enable/disable all
- Admin Users page: list, filter, role toggle, suspend, delete
- Admin Audit Log page: paginated, filter by actor + action
- Insights moved to `/admin/insights/*`
- Job feed cursor pagination (cursor stack replaces page number)
- Company admin router wired into `/admin` group

---

## Remaining

### E2E suite (Playwright)
The only item not yet implemented. Tests required by the definition of done:

| Flow | Assertions |
|---|---|
| Signup → magic link → feed | User provisioned, jobs visible |
| Save / dismiss / apply a job | `user_job_state` status reflected in UI |
| Edit display name, delete account | Profile updated; Supabase auth removed |
| Admin: grant / revoke role | Role change reflected on next login |
| Non-admin navigates to `/admin/*` | Redirected client-side; API returns 403 |

---

## Cutover runbook

```
1. make k8s-down
2. cd backend && goose up          # phase 2 migrations
3. make k8s-build
4. make k8s-up
5. make k8s-logs-crawler           # verify companies without scan_settings are skipped
6. Sign in as admin → /admin/companies → "Set up" each company that needs scan_settings
```
