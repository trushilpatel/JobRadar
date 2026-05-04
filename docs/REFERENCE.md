# JobRadar — Reference

All API routes require auth (`Authorization: Bearer <jwt>` or `X-Dev-Token` locally).
Base URL: `http://localhost:8080`

> **Phase 1 (multi-user)** is shipped — identity flows via context, RBAC0 tables exist, `/api/me` works. Phase 2 (per-user data model, `/api` vs `/admin` split, companies public/operational split, `job_feed`→`jobs` rename) and Phase 3 (frontend admin UI) are **planned**. See [`Designs/multi-user-rollout-plan.md`](../Designs/multi-user-rollout-plan.md).

---

## API Routes

### Companies
| Method | Path | Source |
|---|---|---|
| GET | `/companies` | [CompanyRestRouter.go](../backend/service/company/services/CompanyRestRouter.go) |
| POST | `/companies` | [CompanyRestRouter.go](../backend/service/company/services/CompanyRestRouter.go) |
| PATCH | `/companies/active` | [CompanyRestRouter.go](../backend/service/company/services/CompanyRestRouter.go) |
| PATCH | `/companies/{id}` | [CompanyRestRouter.go](../backend/service/company/services/CompanyRestRouter.go) |
| DELETE | `/companies/{id}` | [CompanyRestRouter.go](../backend/service/company/services/CompanyRestRouter.go) |

### Scan
| Method | Path | Source |
|---|---|---|
| GET | `/companies/queue` | [ScanRestRouter.go](../backend/service/scan/services/ScanRestRouter.go) |
| DELETE | `/companies/queue/{entry_id}` | [ScanRestRouter.go](../backend/service/scan/services/ScanRestRouter.go) |
| POST | `/companies/scan-all` | [ScanRestRouter.go](../backend/service/scan/services/ScanRestRouter.go) |
| POST | `/companies/{id}/scan` | [ScanRestRouter.go](../backend/service/scan/services/ScanRestRouter.go) |
| GET | `/companies/{id}/scan-stats` | [ScanRestRouter.go](../backend/service/scan/services/ScanRestRouter.go) |
| GET | `/companies/{id}/analytics` | [ScanRestRouter.go](../backend/service/scan/services/ScanRestRouter.go) |

### Adapter Config
| Method | Path | Source |
|---|---|---|
| GET | `/adapter-configs` | [AdapterConfigRestRouter.go](../backend/service/scan/services/AdapterConfigRestRouter.go) |
| PATCH | `/adapter-configs/{adapter_type}` | [AdapterConfigRestRouter.go](../backend/service/scan/services/AdapterConfigRestRouter.go) |
| PATCH | `/companies/{id}/scan-interval-override` | [AdapterConfigRestRouter.go](../backend/service/scan/services/AdapterConfigRestRouter.go) |
| GET | `/companies/{id}/scan-config` | [AdapterConfigRestRouter.go](../backend/service/scan/services/AdapterConfigRestRouter.go) |

### Jobs
| Method | Path | Source |
|---|---|---|
| GET | `/jobs` | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go) |
| GET | `/jobs/counts` | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go) |
| GET | `/jobs/{id}` | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go) |
| DELETE | `/jobs/{id}` | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go) |
| POST | `/jobs/dismiss/bulk` | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go) |
| PATCH | `/jobs/{id}/save` | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go) |
| PATCH | `/jobs/{id}/status` | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go) |
| PATCH | `/jobs/{id}/notes` | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go) |

### Applications
| Method | Path | Source |
|---|---|---|
| GET | `/jobs/{id}/applications` | [ApplicationRestRouter.go](../backend/service/application/services/ApplicationRestRouter.go) |
| POST | `/jobs/{id}/applications` | [ApplicationRestRouter.go](../backend/service/application/services/ApplicationRestRouter.go) |
| PATCH | `/jobs/{id}/applications/{app_id}/stage` | [ApplicationRestRouter.go](../backend/service/application/services/ApplicationRestRouter.go) |
| PATCH | `/jobs/{id}/applications/{app_id}/note` | [ApplicationRestRouter.go](../backend/service/application/services/ApplicationRestRouter.go) |
| GET | `/applications` | [ApplicationRestRouter.go](../backend/service/application/services/ApplicationRestRouter.go) |
| PATCH | `/applications/{id}/stage` | [ApplicationRestRouter.go](../backend/service/application/services/ApplicationRestRouter.go) |

### Dashboard
| Method | Path | Source |
|---|---|---|
| GET | `/dashboard/stats` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET | `/dashboard/activity` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET | `/dashboard/progress` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET | `/dashboard/market` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET | `/dashboard/heatmap` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET | `/dashboard/interviews` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET | `/dashboard/velocity` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET | `/dashboard/tech-trends` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET | `/dashboard/pipeline` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |

### Insights
| Method | Path | Source |
|---|---|---|
| GET | `/insights/tech-stack` | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |
| GET | `/insights/company-activity` | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |
| GET | `/insights/funnel` | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |
| GET | `/insights/location` | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |
| GET | `/insights/sponsorship` | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |
| GET | `/insights/velocity` | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |
| GET | `/insights/role-categories` | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |

### Identity (Phase 1)
| Method | Path | Source |
|---|---|---|
| GET | `/api/me` | [UserRestRouter.go](../backend/service/user/services/UserRestRouter.go) — returns Identity bundle (id, email, display_name, status, roles) |
| PATCH | `/api/me` | [UserRestRouter.go](../backend/service/user/services/UserRestRouter.go) — update `display_name` |
| DELETE | `/api/me` | [UserRestRouter.go](../backend/service/user/services/UserRestRouter.go) — hard delete: Supabase admin DELETE → DB cascade |

> **Planned (Phase 2):** route tree splits into `/api/*` (RequireAuth) and `/admin/*` (RequireAuth + RequireRole("admin")). Insights, scans, queue, adapter-configs, pipeline dashboard move under `/admin/*`. See [`Designs/multi-user-rollout-plan.md`](../Designs/multi-user-rollout-plan.md).

---

## Database Tables (Phase 1 additions)

| Table | Role | Migration |
|---|---|---|
| `users` | Registered users — id (UUID), email (citext, unique), display_name, status (`active`\|`suspended`), timestamps | [043](../backend/migrations/043_phase1_users_roles_audit.sql) |
| `roles` | RBAC0 role catalog — seeded with `user`, `admin` | [043](../backend/migrations/043_phase1_users_roles_audit.sql) |
| `user_roles` | User↔role join (composite PK); cascades: user_id CASCADE, role_id RESTRICT, granted_by SET NULL | [043](../backend/migrations/043_phase1_users_roles_audit.sql) |
| `admin_audit_log` | Append-only admin audit trail (actor, action, target_type, target_id, payload JSONB) | [043](../backend/migrations/043_phase1_users_roles_audit.sql) |

> **Planned (Phase 2):** `user_job_state`, `company_scan_settings`; rename `job_feed` → `jobs` (per-user columns dropped); `job_application_tracker.user_id` NOT NULL; drop `job_dismissal_log`; `companies` operational columns extracted.

---

## Environment variables (Phase 1 additions)

| Var | Purpose |
|---|---|
| `ADMIN_EMAILS` | Comma-separated emails granted `admin` role on first login |
| `SUPABASE_URL` | Supabase project URL (used by `backend/platform/supabaseadmin`) |
| `SUPABASE_SERVICE_ROLE_KEY` | Supabase service-role key for admin DELETE user calls |
| `RATE_LIMIT_READ_RPM` | Per-user rate limit for GET/HEAD/OPTIONS (default 30) |
| `RATE_LIMIT_WRITE_RPM` | Per-user rate limit for mutating methods (default 20) |
| `RATE_LIMIT_ADMIN_RPM` | Per-user rate limit for admin scope (default 120) |
| `CACHE_TTL_USER_SECONDS` | TTL for user-cache decorator (default 60) |

---

## Kafka Topics

| Topic | Producer | Consumer | Notes |
|---|---|---|---|
| `job_listings.raw` | crawler | filter | Main pipeline — new raw jobs |
| `job_listings.raw.retry.1m` | filter | filter | Transient failures — retry after 1 min |
| `job_listings.raw.retry.10m` | filter | filter | Retry after 10 min |
| `job_listings.raw.retry.1h` | filter | filter | Retry after 1 hour |
| `job_listings.raw.dlq` | filter | — | Unrecoverable — manual inspection |
| `job_listings.filtered` | filter | analyzer | Relevant jobs passed to enrichment |
| `job_listings.filtered.retry.1m` | analyzer | analyzer | Transient failures — retry after 1 min |
| `job_listings.filtered.retry.10m` | analyzer | analyzer | Retry after 10 min |
| `job_listings.filtered.retry.1h` | analyzer | analyzer | Retry after 1 hour |
| `job_listings.filtered.dlq` | analyzer | — | Unrecoverable — manual inspection |

Topic constants: [`backend/platform/kafka/topics.go`](../backend/platform/kafka/topics.go)

---

## LLM Slots

Config file: [`backend/common/config/llm-slots.yaml`](../backend/common/config/llm-slots.yaml)
API keys expand from env vars at load time (`${GEMINI_KEY}`).

| Worker | Model | RPM | RPD | TPM in |
|---|---|---|---|---|
| filter | `gemini-3.1-flash-lite-preview` | 15 | 200 | 250k |
| enricher | `gemma-4-31b-it` | 15 | 1,500 | 1M |
