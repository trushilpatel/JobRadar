# JobRadar — Reference

All API routes require auth (`Authorization: Bearer <jwt>` or `X-Dev-Token` locally).
Base URL: `http://localhost:8080`

**Auth middleware:**

- All routes under `/api/*` — `RequireIdentity` + `RateLimit`. `RequireIdentity` performs JWT verification, DB-backed identity enrichment, cache-aside lookup, lazy provisioning into `users`/`user_roles`, `ADMIN_EMAILS` auto-grant, and suspended-user 403.
- All routes under `/admin/*` — additionally require `RequireRole("admin")` + `AuditAdminMutation`.
- No `ALLOWED_EMAIL` single-email allowlist. No legacy `RequireAuth` middleware.

Operational endpoints served on `:9090` — no auth required: `GET /health`, `GET /ready`, `GET /metrics`.

---

## API Routes

### Companies (public read-only)

| Method | Path                  | Source                                                                           |
| ------ | --------------------- | -------------------------------------------------------------------------------- |
| GET    | `/api/companies`      | [CompanyRestRouter.go](../backend/service/company/services/CompanyRestRouter.go) |
| GET    | `/api/companies/{id}` | [CompanyRestRouter.go](../backend/service/company/services/CompanyRestRouter.go) |

### Scan

| Method | Path                              | Source                                                                  |
| ------ | --------------------------------- | ----------------------------------------------------------------------- |
| GET    | `/api/companies/queue`            | [ScanRestRouter.go](../backend/service/scan/services/ScanRestRouter.go) |
| DELETE | `/api/companies/queue/{entry_id}` | [ScanRestRouter.go](../backend/service/scan/services/ScanRestRouter.go) |
| POST   | `/api/companies/{id}/scan`        | [ScanRestRouter.go](../backend/service/scan/services/ScanRestRouter.go) |
| GET    | `/api/companies/{id}/scan-stats`  | [ScanRestRouter.go](../backend/service/scan/services/ScanRestRouter.go) |

### Adapter Config

| Method | Path                                         | Source                                                                                    |
| ------ | -------------------------------------------- | ----------------------------------------------------------------------------------------- |
| GET    | `/api/adapter-configs`                       | [AdapterConfigRestRouter.go](../backend/service/scan/services/AdapterConfigRestRouter.go) |
| PATCH  | `/api/adapter-configs/{adapter_type}`        | [AdapterConfigRestRouter.go](../backend/service/scan/services/AdapterConfigRestRouter.go) |
| PATCH  | `/api/companies/{id}/scan-interval-override` | [AdapterConfigRestRouter.go](../backend/service/scan/services/AdapterConfigRestRouter.go) |
| GET    | `/api/companies/{id}/scan-config`            | [AdapterConfigRestRouter.go](../backend/service/scan/services/AdapterConfigRestRouter.go) |

### Jobs

Cursor-paginated feed — `GET /api/jobs` returns `{items, next_cursor}`.

| Method | Path                    | Source                                                                         |
| ------ | ----------------------- | ------------------------------------------------------------------------------ |
| GET    | `/api/jobs`             | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go)           |
| GET    | `/api/jobs/{id}`        | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go)           |
| DELETE | `/api/jobs/{id}`        | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go) — dismiss |
| PATCH  | `/api/jobs/{id}/save`   | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go)           |
| PATCH  | `/api/jobs/{id}/status` | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go)           |
| PATCH  | `/api/jobs/{id}/notes`  | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go)           |

### Applications

| Method | Path                                         | Source                                                                                       |
| ------ | -------------------------------------------- | -------------------------------------------------------------------------------------------- |
| GET    | `/api/jobs/{id}/applications`                | [ApplicationRestRouter.go](../backend/service/application/services/ApplicationRestRouter.go) |
| POST   | `/api/jobs/{id}/applications`                | [ApplicationRestRouter.go](../backend/service/application/services/ApplicationRestRouter.go) |
| PATCH  | `/api/jobs/{id}/applications/{app_id}/stage` | [ApplicationRestRouter.go](../backend/service/application/services/ApplicationRestRouter.go) |
| PATCH  | `/api/jobs/{id}/applications/{app_id}/note`  | [ApplicationRestRouter.go](../backend/service/application/services/ApplicationRestRouter.go) |
| GET    | `/api/applications`                          | [ApplicationRestRouter.go](../backend/service/application/services/ApplicationRestRouter.go) |

### Dashboard (user-facing)

| Method | Path                      | Source                                                                                 |
| ------ | ------------------------- | -------------------------------------------------------------------------------------- |
| GET    | `/api/dashboard/stats`    | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET    | `/api/dashboard/activity` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET    | `/api/dashboard/progress` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET    | `/api/dashboard/market`   | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET    | `/api/dashboard/heatmap`  | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET    | `/api/dashboard/velocity` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |

### Identity

| Method | Path      | Source                                                                                                                                     |
| ------ | --------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| GET    | `/api/me` | [UserRestRouter.go](../backend/service/user/services/UserRestRouter.go) — returns identity bundle (id, email, display_name, status, roles) |
| PATCH  | `/api/me` | [UserRestRouter.go](../backend/service/user/services/UserRestRouter.go) — update `display_name`                                            |
| DELETE | `/api/me` | [UserRestRouter.go](../backend/service/user/services/UserRestRouter.go) — hard delete: Supabase admin DELETE + DB cascade                  |

---

## Admin Routes

All routes under `/admin/*` require `RequireIdentity` + `RequireRole("admin")` + `AuditAdminMutation`.

### Admin — Companies

| Method | Path                                  | Source                                                                                     |
| ------ | ------------------------------------- | ------------------------------------------------------------------------------------------ |
| GET    | `/admin/companies`                    | [CompanyAdminRestRouter.go](../backend/service/company/services/CompanyAdminRestRouter.go) |
| POST   | `/admin/companies`                    | [CompanyAdminRestRouter.go](../backend/service/company/services/CompanyAdminRestRouter.go) |
| PATCH  | `/admin/companies/active`             | [CompanyAdminRestRouter.go](../backend/service/company/services/CompanyAdminRestRouter.go) |
| GET    | `/admin/companies/{id}`               | [CompanyAdminRestRouter.go](../backend/service/company/services/CompanyAdminRestRouter.go) |
| PATCH  | `/admin/companies/{id}`               | [CompanyAdminRestRouter.go](../backend/service/company/services/CompanyAdminRestRouter.go) |
| PATCH  | `/admin/companies/{id}/scan-settings` | [CompanyAdminRestRouter.go](../backend/service/company/services/CompanyAdminRestRouter.go) |
| DELETE | `/admin/companies/{id}`               | [CompanyAdminRestRouter.go](../backend/service/company/services/CompanyAdminRestRouter.go) |

### Admin — Users

| Method | Path                       | Source                                                                            |
| ------ | -------------------------- | --------------------------------------------------------------------------------- |
| GET    | `/admin/users`             | [UserAdminRestRouter.go](../backend/service/user/services/UserAdminRestRouter.go) |
| GET    | `/admin/users/{id}`        | [UserAdminRestRouter.go](../backend/service/user/services/UserAdminRestRouter.go) |
| PATCH  | `/admin/users/{id}/roles`  | [UserAdminRestRouter.go](../backend/service/user/services/UserAdminRestRouter.go) |
| PATCH  | `/admin/users/{id}/status` | [UserAdminRestRouter.go](../backend/service/user/services/UserAdminRestRouter.go) |
| DELETE | `/admin/users/{id}`        | [UserAdminRestRouter.go](../backend/service/user/services/UserAdminRestRouter.go) |

### Admin — Audit Log

| Method | Path               | Source                                                                               |
| ------ | ------------------ | ------------------------------------------------------------------------------------ |
| GET    | `/admin/audit-log` | [AuditAdminRestRouter.go](../backend/service/audit/services/AuditAdminRestRouter.go) |

### Admin — Dashboard

| Method | Path                        | Source                                                                               |
| ------ | --------------------------- | ------------------------------------------------------------------------------------ |
| GET    | `/admin/dashboard/pipeline` | [PipelineRestRouter.go](../backend/service/dashboard/services/PipelineRestRouter.go) |

### Admin — Insights

| Method | Path                               | Source                                                                              |
| ------ | ---------------------------------- | ----------------------------------------------------------------------------------- |
| GET    | `/admin/insights/tech-stack`       | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |
| GET    | `/admin/insights/company-activity` | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |
| GET    | `/admin/insights/funnel`           | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |
| GET    | `/admin/insights/location`         | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |
| GET    | `/admin/insights/sponsorship`      | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |
| GET    | `/admin/insights/velocity`         | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |
| GET    | `/admin/insights/role-categories`  | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |

---

## Database Tables

Latest migration: [`044_phase2_multiuser_data_model.sql`](../backend/migrations/044_phase2_multiuser_data_model.sql)

| Table                     | Role                                                                                                                                               |
| ------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| `companies`               | Tracked company catalog; operational columns dropped; scan config lives in `company_scan_settings`                                                 |
| `company_scan_settings`   | Per-company scan config (admin-managed); companies without a row are skipped by workers                                                            |
| `company_scan_queue`      | Durable work queue for pending/in-progress scans; deduped by unique partial index                                                                  |
| `company_scan_log`        | Per-run scan summary (fetched, new, duration, error)                                                                                               |
| `job_pipeline_ledger`     | Dedup + pipeline state tracking; `removed_at` records disappearance; `reason='retry'` with `retry_count` + `next_attempt_at` for in-flight retries |
| `jobs`                    | Global job rows (renamed from `job_feed`); per-user columns dropped — state lives in `user_job_state`                                              |
| `user_job_state`          | Per-user job state overlay: `saved_at`, `applied_at`, `status`, `notes`, dismissal reason/note                                                     |
| `job_analytics`           | Append-only enriched snapshot written by the analyzer; survives job dismissal                                                                      |
| `job_application_tracker` | Application stage history; `user_id NOT NULL`                                                                                                      |
| `users`                   | Registered users — id (UUID), email (citext, unique), display_name, status (`active`\|`suspended`), timestamps                                     |
| `roles`                   | RBAC0 role catalog — TEXT PK, seeded with `user`, `admin`                                                                                          |
| `user_roles`              | User↔role join (composite PK); cascades: user_id CASCADE, role_id RESTRICT, granted_by SET NULL                                                    |
| `admin_audit_log`         | Append-only admin audit trail (actor, action, target_type, target_id, payload JSONB)                                                               |

---

## Environment Variables

| Var                         | Purpose                                                          |
| --------------------------- | ---------------------------------------------------------------- |
| `DATABASE_URL`              | Postgres connection string                                       |
| `KAFKA_BROKERS`             | Kafka broker addresses                                           |
| `REDIS_URL`                 | Redis connection URL (`rediss://user:pass@host:port/db` for TLS) |
| `SUPABASE_URL`              | Supabase project URL (used by `backend/platform/supabaseadmin`)  |
| `SUPABASE_SERVICE_ROLE_KEY` | Supabase service-role key for admin DELETE user calls            |
| `ADMIN_EMAILS`              | Comma-separated emails auto-granted `admin` role on first login  |
| `RATE_LIMIT_READ_RPM`       | Per-user rate limit for GET/HEAD/OPTIONS (default 30)            |
| `RATE_LIMIT_WRITE_RPM`      | Per-user rate limit for mutating methods (default 20)            |
| `RATE_LIMIT_ADMIN_RPM`      | Per-user rate limit for admin scope (default 120)                |
| `CACHE_TTL_USER_SECONDS`    | TTL for user identity cache decorator (default 60)               |
| `SCAN_INTERVAL_MINUTES`     | Global scan interval in minutes (default 1440 = 24h)             |
| `DEV_TOKEN`                 | Dev bypass token — accepted as `X-Dev-Token` for local testing   |

---

## Kafka Topics

| Topic                             | Producer | Consumer | Notes                                  |
| --------------------------------- | -------- | -------- | -------------------------------------- |
| `job_listings.raw`                | crawler  | filter   | Main pipeline — new raw jobs           |
| `job_listings.raw.retry.1m`       | filter   | filter   | Transient failures — retry after 1 min |
| `job_listings.raw.retry.10m`      | filter   | filter   | Retry after 10 min                     |
| `job_listings.raw.retry.1h`       | filter   | filter   | Retry after 1 hour                     |
| `job_listings.raw.dlq`            | filter   | —        | Unrecoverable — manual inspection      |
| `job_listings.filtered`           | filter   | analyzer | Relevant jobs passed to enrichment     |
| `job_listings.filtered.retry.1m`  | analyzer | analyzer | Transient failures — retry after 1 min |
| `job_listings.filtered.retry.10m` | analyzer | analyzer | Retry after 10 min                     |
| `job_listings.filtered.retry.1h`  | analyzer | analyzer | Retry after 1 hour                     |
| `job_listings.filtered.dlq`       | analyzer | —        | Unrecoverable — manual inspection      |

Topic constants: [`backend/platform/kafka/topics.go`](../backend/platform/kafka/topics.go)

---

## LLM Slots

Config file: [`k8s/helm/jobradar/files/llm-slots.yaml`](../k8s/helm/jobradar/files/llm-slots.yaml)
API keys expand from env vars at load time (`${GEMINI_KEY}`, `${GEMINI_KEY_JP}`).

| Worker   | Model                           | RPM | RPD   |
| -------- | ------------------------------- | --- | ----- |
| filter   | `gemini-3.1-flash-lite-preview` | 15  | 200   |
| enricher | `gemma-4-31b-it`                | 15  | 1,500 |
| enricher | `gemma-4-26b-a4b-it`            | 15  | 1,500 |
