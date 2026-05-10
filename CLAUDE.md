# JobRadar

Production-grade automated job tracker. Monitors careers pages, filters via LLM, surfaces relevant jobs.

## Pipeline

```
company_scan_scheduler → company_scan_queue (DB) → job_listing_crawler → job_listings.raw (Kafka) → job_listing_relevance_filter → job_listings.filtered (Kafka) → job_post_analyzer → jobs (DB) → API
```

## Directory map

| Path                              | What lives there                                                                                       |
| --------------------------------- | ------------------------------------------------------------------------------------------------------ |
| `k8s/helm/jobradar/`              | Helm chart for all app services (api, console, workers, kafka, redis, migrate)                         |
| `k8s/helm/observability/`         | Helm chart for observability stack (Prometheus, Grafana, Loki, Jaeger, audit-sync)                     |
| `backend/apps/crawler/`           | Crawler binary + engine + ATS adapters (Greenhouse, Lever, Ashby, Google, etc.)                        |
| `backend/apps/scheduler/`         | Scheduler binary — enqueues companies due for a scan                                                   |
| `backend/apps/filter/`            | Filter binary + worker — LLM batch scoring, routes relevant jobs                                       |
| `backend/apps/analyzer/`          | Analyzer binary + worker + techtag normalizer — LLM enrichment, writes jobs table                      |
| `backend/apps/api/`               | API binary + router + handlers + middleware + request/response helpers                                 |
| `backend/apps/migrate/`           | Migration runner binary                                                                                |
| `backend/common/config/`          | Config struct + env var loading via godotenv                                                           |
| `backend/common/domain/`          | Shared error types: ServiceError, ValidationError, sentinel errors + `IsNotFound`                      |
| `backend/service/`                | Feature-slice packages — each owns handler + service + store + models + validator                      |
| `backend/service/job/`            | Job listing operations — models in `job/models/` (package `jobmodel`)                                  |
| `backend/service/company/`        | Company CRUD + scan trigger — models in `company/models/` (package `companymodel`)                     |
| `backend/service/application/`    | Application tracking — models in `application/models/` (package `appmodel`)                            |
| `backend/service/dashboard/`      | Dashboard stats + pipeline status                                                                      |
| `backend/service/insights/`       | Analytics and insights queries                                                                         |
| `backend/service/scan/`           | Scan result recording — models in `scan/models/` (package `scanmodel`)                                 |
| `backend/service/llm/`            | LLM business logic — `JobsRelevanceFilter` + `JobPostEnricher` interfaces, Gemini impl, prompts        |
| `backend/service/user/`           | User domain — Get/Provision/UpdateProfile/Delete + cache decorator + `/api/me` + admin user management |
| `backend/service/role/`           | Role domain — RBAC0: Grant/Revoke/ListForUser/ListAll                                                  |
| `backend/service/audit/`          | Admin audit log domain — Append/List for `admin_audit_log`                                             |
| `backend/common/identity/`        | Identity struct + `context.Context` helpers                                                            |
| `backend/common/`                 | Infrastructure plumbing (config, db, errors, kafka, logger, metrics, redis, tracing)                   |
| `backend/common/tracing/`         | OTel tracing init, Start/End span helpers, Kafka propagation                                           |
| `backend/platform/cache/`         | Cache interface + Redis/Memory/Noop impls + `GetOrLoad[T]` single-flight                               |
| `backend/platform/ratelimit/`     | Token-bucket Limiter + Redis/Memory impls + `ScopeFromMethod` helper                                   |
| `backend/platform/supabaseadmin/` | Supabase admin REST client — `DeleteUser` for hard account deletion                                    |
| `backend/migrations/`             | goose SQL migration files                                                                              |
| `infra/`                          | Dockerfiles + Loki/Prometheus/Kafka ops configs                                                        |

## Workers

| Binary           | Role                                                                                          |
| ---------------- | --------------------------------------------------------------------------------------------- |
| `apps/scheduler` | Queries DB for companies due for a scan, inserts into `company_scan_queue`                    |
| `apps/crawler`   | Polls `company_scan_queue` (DB), fetches careers pages, publishes to `job_listings.raw`       |
| `apps/filter`    | Scores batches via LLM, routes relevant to `job_listings.filtered`                            |
| `apps/analyzer`  | Fetches full job page (or uses pre-fetched FullContent), LLM enriches, writes to `jobs` table |
| `apps/api`       | REST API on `:8080`                                                                           |

## Database tables

| Table                     | Role                                                                                                                                                                                                                                             |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `companies`               | Tracked company catalog; operational columns dropped; `company_scan_settings` holds scan config                                                                                                                                                  |
| `company_scan_settings`   | Per-company scan configuration (admin-managed); companies without a row are skipped by workers                                                                                                                                                   |
| `company_scan_queue`      | Durable work queue for pending/in-progress company scans; deduped by unique partial index                                                                                                                                                        |
| `company_scan_log`        | Per-run scan summary (fetched, new, duration, error)                                                                                                                                                                                             |
| `job_pipeline_ledger`     | Dedup + pipeline blocking; `removed_at` tracks when a job disappears; `reason` includes `retry` (transient in-flight) with `retry_count` + `next_attempt_at` populated only while `reason='retry'`, written by the filter via `UpdateRetryBatch` |
| `jobs`                    | Operational feed — global job rows; per-user status/notes live in `user_job_state`                                                                                                                                                               |
| `user_job_state`          | Per-user job state overlay: saved_at, applied_at, status, notes, dismissal reason/note                                                                                                                                                           |
| `job_analytics`           | Append-only analytics record written by job_post_analyzer; survives job dismissal                                                                                                                                                                |
| `job_application_tracker` | Application stage tracking; `user_id NOT NULL`                                                                                                                                                                                                   |
| `users`                   | Registered users (id, email citext, display_name, status, timestamps)                                                                                                                                                                            |
| `roles`                   | RBAC0 role catalog (`user`, `admin`); seeded by migration 043                                                                                                                                                                                    |
| `user_roles`              | user↔role join; cascade rules: user_id CASCADE, role_id RESTRICT, granted_by SET NULL                                                                                                                                                            |
| `admin_audit_log`         | Append-only audit trail for admin mutations (actor, action, target, payload)                                                                                                                                                                     |

## Job status flow

```
new → saved → applied    (user advances)
new → dismissed          (user dismisses — job row kept in `jobs` for FK integrity)
```

Per-user status lives in `user_job_state` (upserted on every save/dismiss/status change). The "new" tab uses a global feed query with a per-user overlay (GetMany on the fetched IDs); all other status tabs drive from `user_job_state` directly (INNER JOIN to `jobs`) so every page is a full limit-sized result set. Historical analytics come from `job_analytics`; current-state aggregations (e.g. funnel stage counts) read `user_job_state.status` directly.

## Scan scheduling

- `SCAN_INTERVAL_MINUTES` (default `1440` = 24h) controls how often each company is scanned
- Scheduler checks every 1 min; only enqueues companies where `last_scan_at` is NULL or older than the interval
- Manual trigger (`POST /api/companies/:id/scan`) bypasses the cooldown — always enqueues immediately
- Workers skip companies that have no row in `company_scan_settings` (logged, not errored)

## Multi-user

All three phases shipped. See [`Designs/multi-user-rollout-plan.md`](Designs/multi-user-rollout-plan.md) for status and remaining E2E work.

**What's live:**

- Tables: `users`, `roles`, `user_roles`, `admin_audit_log`, `user_job_state`, `company_scan_settings`
- Platform packages: `backend/common/identity/`, `backend/platform/cache/`, `backend/platform/ratelimit/`, `backend/platform/supabaseadmin/`
- Service slices: `backend/service/user/` (with cache + admin), `backend/service/role/`, `backend/service/audit/`
- Middleware: `RequireIdentity`, `RequireRole`, `RateLimit`, `AuditAdminMutation`
- Company slice split: public path (`CompanyService`) + admin path (`CompanyAdminService`)
- Job feed: "new" tab uses global feed + per-user overlay; other status tabs drive from `user_job_state` (INNER JOIN jobs); cursor (keyset) pagination; per-user state upserted on every action
- Route split: `/api/*` (RequireIdentity + RateLimit); `/admin/*` (+ RequireRole("admin") + AuditAdminMutation)
- Frontend: identity-aware Zustand store, role-gated admin nav, admin pages (pipeline, companies, users, audit log, insights), account page, 429 toast

**Core concepts:**

- RBAC0 — `roles` + `user_roles` join only; no permissions table yet
- Identity flows via `context.Context` — repos take `userID` explicitly, services pull from ctx via `identity.FromContext` / `UserIDFrom` / `IsAdmin`
- Decorator chain: `Rest → Validation → Cache → Impl → Repo`; CacheService optional per slice
- Source of truth for role membership is Postgres (`user_roles`) — never JWT custom claims

Env vars: `SUPABASE_URL`, `SUPABASE_SERVICE_ROLE_KEY`, `RATE_LIMIT_READ_RPM`, `RATE_LIMIT_WRITE_RPM`, `RATE_LIMIT_ADMIN_RPM`, `CACHE_TTL_USER_SECONDS`.

## Dashboard endpoints

User-facing dashboard routes (all under `/api/dashboard/`):

| Route                           | Description                                                           |
| ------------------------------- | --------------------------------------------------------------------- |
| `GET /api/dashboard/stats`      | Global scan stats (jobs scanned today, pipeline pending/errors)       |
| `GET /api/dashboard/activity`   | Daily activity counts (scan log, `?days=N`)                           |
| `GET /api/dashboard/progress`   | Per-user progress (new/saved/applied/dismissed from `user_job_state`) |
| `GET /api/dashboard/market`     | Market stats (new jobs last 7d, companies active, sponsorship %)      |
| `GET /api/dashboard/heatmap`    | Per-user weekly activity heatmap                                      |
| `GET /api/dashboard/velocity`   | Per-company new-job velocity (last 30d)                               |
| `GET /admin/dashboard/pipeline` | Admin-only — pipeline stats (total/active companies, Kafka lag)       |

## Core quality attributes

Reliability · Availability · Fault Tolerance · Recoverability · Faultlessness · Maintainability · Modularity · Reusability · Analyzeability · Modifiability · Testability

Full reference: [`docs/QUALITY.md`](docs/QUALITY.md). Detailed rules auto-load via `.claude/rules/quality-attributes.md` (backend) and `.claude/rules/frontend-quality-attributes.md` (frontend) for every session.

## Key rules (detail in subdirectory CLAUDE.md files)

- Workers depend on **interfaces**, never concrete types
- All DB access goes through `{Domain}RepositoryService.go` — no raw GORM outside those files
- Domain errors come from `common/domain/` (`domain.ErrNotFound`, `domain.ErrInternal`, etc.) — never use `fmt.Errorf` for sentinel errors that handlers inspect
- All typed values (status, stage, reason) use constants from the owning service's `common/` package — never raw strings
- Insights query `job_analytics` for historical/time-series data (snapshot rows are never deleted); `user_job_state` is the source of truth for current per-user status aggregations (e.g. the pipeline funnel reads `user_job_state.status` for saved/applied/dismissed counts)
- job_listing_relevance_filter batch failures mark jobs as `error` in `job_pipeline_ledger` — never re-queue to Kafka
- All logging goes through `platform/logger.Logger` — never use stdlib `log` package directly
- Field keys must use constants from `platform/logger/fields.go` — never raw strings
- Each service domain follows the decorator chain pattern — see `backend/service/CLAUDE.md`

## Implementation workflow

Before writing any code:

- Gather and confirm all requirements; ask for clarification on anything ambiguous
- Map all edge cases before starting implementation
- Only begin coding once edge cases are fully identified and understood

During implementation:

- Proceed autonomously
- Pause and surface the tradeoff when a decision is ambiguous or has meaningful consequences — do not silently pick one

## Git commit policy

- Never commit directly to `main` — `.husky/pre-commit` blocks it; always work on a feature branch
- **Never use `git commit --no-verify`** (or any other flag that skips hooks: `--no-gpg-sign`, `core.hooksPath=/dev/null`, etc.) under any circumstances, including "small" fixes, doc-only changes, or hotfixes
- If the pre-commit hook fails, fix the underlying issue (formatter, lint error, failing test) and recommit — bypassing is never the answer
- The hook is the floor, not a suggestion: formatting, Go lint, Go unit tests, frontend lint, TypeScript check, and the main-branch guard must all pass before a commit is accepted

## Backend code standards

These extend the rules in `.claude/rules/backend-solid.md`, `.claude/rules/backend-architecture.md`, and `.claude/rules/quality-attributes.md`.

- Keep structs small with one clear reason to change
- Prefer composition; keep interfaces small and focused — one interface per concern
- Add new behaviour via new types/functions; do not modify working code to accommodate it
- Every abstraction must be reusable and extendable without touching existing implementations
- Error handling: wrap with `fmt.Errorf("context: %w", err)` at every layer; log only at top-level boundaries via `platform/logger.Logger`
- API responses: always use `Respond[T]`, `RespondPaged[T]`, `RespondError` helpers — never write raw `json.NewEncoder` directly in handlers

## Frontend code standards

These extend the rules in `.claude/rules/frontend-components.md` and `.claude/rules/frontend-quality-attributes.md`.

**Components**

- Small, single-purpose, reusable — one concern per component
- No god components; split when a component handles more than one responsibility

**State management**

- Server state: TanStack Query only — never `fetch` + `useState` for API data
- Shared UI state (filters, modal open/closed, cross-component selections): Zustand
- Local state (component-scoped only): `useState` / `useReducer`

**UI and styling**

- Use Radix UI primitives for accessible complex components (dialogs, selects, tooltips, switches)
- Use existing libraries before writing custom implementations
- Styling: Tailwind classes composed with `clsx` and `tailwind-merge` — no inline styles
- Stay consistent with existing design tokens and class patterns in the codebase
- Prioritise good UI/UX — accessible, clear, consistent layouts

## Testing and quality gates

Every feature delivery must include:

- Tests written alongside implementation — not as an afterthought
- All tests passing before marking work complete
- Frontend linting passing: `cd ui/console && npm run lint`
- Backend linting passing: `cd backend && golangci-lint run --config ../.github/.golangci.yml --timeout 5m`
- Do not mark a task complete if any test or lint check fails — fix the failure first

After implementation, write a brief spec-style doc:

- File-level references only (which file does what) — no code snippets
- Concise — no padding or filler prose
- Saved to `docs/superpowers/specs/` with date prefix

## Kubernetes local deployment

### Prerequisites — always verify before deploying

- **kubectl context must be `k3d-jobradar`** — never `docker-desktop`. Check with `kubectl config current-context`. Switch with `kubectl config use-context k3d-jobradar`.
- **Storage class is `local-path`** in both Helm charts — this is the only storage class k3d provides. Do NOT change it to `standard` (that's Docker Desktop's class).
- **Images must be built and imported into k3d** before deploying — `make k8s-build` or individual `make k8s-build-{backend,crawler,console}`. Images imported into k3d are NOT available in Docker Desktop's Kubernetes and vice versa.

### Deploy sequence

```bash
kubectl config use-context k3d-jobradar   # always verify context first
make k8s-build                             # build + import all images into k3d
make k8s-up                                # preflight check + helm install + wait for kafka/redis
make k8s-logs-crawler                      # verify with logs
```

### Iterate on crawler only (fastest path)

```bash
make k8s-build-crawler                     # rebuild + reimport crawler image only
make k8s-restart-crawler                   # rolling restart crawler pod
make k8s-logs-crawler                      # watch logs
```

### Trigger a Google scan manually (for testing)

```bash
curl -X POST http://localhost:30080/companies/<company-id>/scan
make k8s-logs-crawler
```

### Common failure modes

| Symptom                         | Cause                                                                 | Fix                                                                                                                                                                                                                       |
| ------------------------------- | --------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------- |
| `ErrImagePull` on all pods      | Wrong kubectl context (docker-desktop) or images not imported         | Switch context; run `make k8s-build`                                                                                                                                                                                      |
| Kafka/Redis PVC stuck `Pending` | StatefulSet storage class mismatch or first-time WaitForFirstConsumer | Run `make k8s-nuke` then `make k8s-up`                                                                                                                                                                                    |
| `TLS handshake timeout`         | k3d API server still starting after restart                           | Wait ~30s; run `until kubectl get nodes 2>/dev/null                                                                                                                                                                       | grep -q Ready; do sleep 3; done` |
| Namespace stuck `Terminating`   | Finalizers not cleared                                                | Force-clear: `kubectl get ns <name> -o json \| python3 -c "import sys,json; d=json.load(sys.stdin); d['spec']['finalizers']=[]; print(json.dumps(d))" \| kubectl replace --raw "/api/v1/namespaces/<name>/finalize" -f -` |

### Crawler Dockerfile note

The crawler image (`infra/Dockerfile.crawler`) is **pure Go** — no Node.js, no Chromium, no Playwright. All adapters speak plain HTTP. Do not add browser dependencies back.

Adapter implementation status (all pure HTTP):

- **Google** — HTTP + RE2 regex over server-rendered HTML (`backend/apps/crawler/ats/google/http.go`).
- **Meta** — HTTP + sitemap-driven listing + JSON-LD detail (`backend/apps/crawler/ats/meta/meta.go`, `sitemap.go`, `http_detail.go`). Implements both `Adapter` (listing) and `ContentFetcher` (detail).
- ATS adapters (Greenhouse, Lever, Ashby, Apple, Microsoft, Netflix, Rippling, Uber) — HTTP / JSON APIs.

## Plan execution mode

When executing an implementation plan, always use the **parallel subagent approach**:

- Use the `superpowers:dispatching-parallel-agents` skill
- Group the plan's tasks into independent buckets (no shared files, no sequential dependencies) and dispatch all buckets in parallel via a single message with multiple Agent tool calls
- Only serialise tasks that truly depend on each other; everything else runs concurrently
- After all parallel agents finish, dispatch one final QA reviewer over the combined result
- Never execute plans inline in the same session
