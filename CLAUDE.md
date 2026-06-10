# JobRadar

Production-grade automated job tracker. Monitors careers pages, filters via LLM, surfaces relevant jobs.

## Development principle — build for a team, always

Every decision is made as if a multi-developer team owns this code, even when one person is writing it today. Never reason from, justify with, or optimise for a "solo developer", "single developer", "just one person", or "single-user" framing — and never weaken a choice because "only one person touches this". Concretely:

- Favour solutions that stay correct when many people change the code concurrently: shared sources of truth over hand-synced duplicates, generated/enforced contracts over convention, CI guards over "I'll remember".
- Treat clarity, documentation, and guardrails as mandatory — the next contributor is a stranger, not the author.
- When weighing effort vs. payoff, assume team scale and API/feature churn; do not discount robustness because the current headcount is small.

This supersedes any cost/benefit argument that leans on small team size.

## Pipeline

```
scheduler → company_scan_queue (DB) → orchestrator ← crawler-node(s) → jobs.raw.<adapter> (NATS JetStream) → filter → jobs.filtered.<adapter> (NATS JetStream) → analyzer → jobs (DB) → API
```

The crawler tier is now **distributed pull-based**: the orchestrator (`backend/apps/orchestrator/`) exposes an HTTP API; one or more stateless crawler-node workers (`backend/apps/crawler-node/`) poll it for work, crawl the assigned company, and submit results back. Concurrency is controlled entirely by the replica count — no in-process worker pool needed.

Each binary lives in `backend/apps/<name>/` (wiring + worker loop only) and depends on domain logic in `backend/service/<domain>/`. Subdirectory `CLAUDE.md` files auto-load when you work in that area.

## Database tables

| Table                     | Role                                                                                                                                                                                                                                                                            |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `companies`               | Tracked company catalog; operational config lives in `company_scan_settings`                                                                                                                                                                                                    |
| `ats`                     | Canonical ATS registry (PK `name`); `company_scan_settings` and `company_scan_queue` carry FK constraints to this table — never add raw adapter type strings elsewhere. Served by `service/ats/` with in-process `MemoryCache` (5min TTL). Admin routes: `GET/PATCH /admin/ats` |
| `company_scan_settings`   | Per-company scan config (admin-managed); `adapter_type` FK → `ats(name)`; rows missing here are skipped by workers                                                                                                                                                              |
| `company_scan_queue`      | Durable work queue for pending/in-progress scans; deduped by unique partial index; `adapter_type` FK → `ats(name)`; `locked_by` holds the claiming crawler-node ID; `heartbeat_at` tracks liveness — `ResetStuckForNodes` reclaims expired rows                                 |
| `company_scan_log`        | Per-run scan summary (fetched, new, duration, error)                                                                                                                                                                                                                            |
| `crawler_nodes`           | Registered crawler-node registry; `token_hash` stores HMAC-SHA256 of the bearer token; `last_seen_at` drives online/offline status. Provisioned via `POST /admin/crawler-nodes`; token returned exactly once at creation                                                        |
| `job_pipeline_ledger`     | Dedup + pipeline blocking; `removed_at` marks disappearance; `reason='retry'` carries `retry_count`/`next_attempt_at`, written via filter `UpdateRetryBatch`. UPDATE ops return `ErrLedgerRowMissing` on 0/partial rows — never silent nil (migrations 071/073)                 |
| `jobs`                    | Operational feed — global rows; per-user status/notes live in `user_job_state`. `job_post_id` is **UNIQUE** (`uq_jobs_job_post_id`, migration 071); `InsertJob` is idempotent (`ON CONFLICT DO NOTHING` → `ErrJobAlreadyExists`)                                                |
| `user_job_state`          | Per-user overlay: saved_at, applied_at, status, notes, dismissal reason/note                                                                                                                                                                                                    |
| `job_analytics`           | Append-only snapshot from analyzer; survives job dismissal                                                                                                                                                                                                                      |
| `job_application_tracker` | Application stage tracking; `user_id NOT NULL`                                                                                                                                                                                                                                  |
| `users`                   | Registered users (id, email citext, display_name, status, timestamps)                                                                                                                                                                                                           |
| `roles`, `user_roles`     | RBAC0 catalog + join. Cascades: `user_id` CASCADE, `role_id` RESTRICT, `granted_by` SET NULL. Seeded by migration 043                                                                                                                                                           |
| `admin_audit_log`         | Append-only audit trail for admin mutations                                                                                                                                                                                                                                     |

## Job status flow

```
new → saved → applied    (user advances)
new → dismissed          (job row kept in `jobs` for FK integrity)
```

`user_job_state` is upserted on every save/dismiss/status change. The "new" tab uses a global feed query with a per-user overlay (`GetMany` on the fetched IDs); other status tabs INNER JOIN `user_job_state → jobs` so every page returns a full limit-sized result. Historical analytics read `job_analytics` (snapshot rows never deleted); current-state aggregations (funnel counts) read `user_job_state.status`.

## Scan scheduling

- `SCAN_INTERVAL_MINUTES` (default `1440` = 24h) controls per-company cadence
- Scheduler ticks every 1 min; only enqueues companies where `last_scan_at` is NULL or older than the interval
- Manual trigger (`POST /api/companies/:id/scan`) bypasses the cooldown — always enqueues immediately
- Workers skip companies with no `company_scan_settings` row (logged, not errored)

## Multi-user

All three phases shipped.

- **Decorator chain:** `Rest → Validation → Cache → Impl → Repo`. `CacheService` is optional per slice.
- **Identity flow:** repos take `userID` explicitly; services pull from ctx via `identity.FromContext` / `UserIDFrom` / `IsAdmin`.
- **RBAC0:** `roles` + `user_roles` join only, no permissions table. Source of truth is Postgres — never JWT custom claims.
- **Route split:** `/api/*` uses `RequireIdentity` + `RateLimit`; `/admin/*` adds `RequireRole("admin")` + `AuditAdminMutation`.
- **Job feed:** "new" = global feed + per-user overlay; other tabs = `user_job_state` INNER JOIN; cursor (keyset) pagination.
- **Company slice:** split into public `CompanyService` and admin `CompanyAdminService` paths. List endpoint returns `CompanyAdminListView` (no `adapter_config`); detail endpoint returns full `CompanyAdminView` — never expose adapter credentials in paginated list responses.
- **Cursor (keyset) pagination:** all list endpoints use `next_cursor` / `cursor` params, not `page` / `offset`. Frontend uses a `cursorHistory` stack for prev/next navigation.
- **Frontend:** identity-aware Zustand store, role-gated admin nav (pipeline / companies / users / audit log / insights), 429 toast.

Env vars: `SUPABASE_URL`, `SUPABASE_SERVICE_ROLE_KEY`, `RATE_LIMIT_{READ,WRITE,ADMIN}_RPM`, `CACHE_TTL_USER_SECONDS`.

## Key rules

- Workers depend on **service interfaces**, never on concrete store types or GORM
- All DB access goes through `{Domain}RepositoryService.go` — no raw GORM elsewhere
- Domain errors come from `common/domain/` (`domain.ErrNotFound`, `domain.ErrInternal`, …) — never `fmt.Errorf` for sentinel errors handlers inspect
- Wrap errors with `fmt.Errorf("context: %w", err)` at every layer; log only at top-level boundaries via `platform/logger.Logger`
- API responses use `handler.Respond[T]`, `handler.RespondPaged[T]`, `handler.RespondError` — never raw `json.NewEncoder` in handlers
- Typed values (status, stage, reason) use constants from the owning service's `common/` package — never raw strings
- Insights: `job_analytics` for historical / time-series; `user_job_state.status` for current per-user aggregations
- Filter batch failures mark jobs `error` in `job_pipeline_ledger` — never re-queue to NATS
- Ledger/queue UPDATE store methods check `RowsAffected` and return a sentinel (`ErrLedgerRowMissing` / `ErrQueueRowMissing`) when 0/partial rows match — never silently return nil. Workers act on it: filter Naks for redelivery (closes the crawler-INSERT vs filter-UPDATE race), analyzer treats it as non-fatal. The crawler writes the `fetch` ledger row **before** publishing to NATS so the row exists before the message can be consumed
- All logging goes through `platform/logger.Logger` — never stdlib `log`; field keys come from `platform/logger/fields.go` only
- Crawler image (`infra/Dockerfile.crawler`) is **pure Go** — no Node, no Chromium, no Playwright. Do not add browser deps
- Orchestrator + crawler-node images (`infra/Dockerfile.orchestrator`) are also **pure Go**. Same constraint applies
- Crawler-node count is controlled by `crawlerNode.replicas` in `k8s/helm/jobradar/values.local.yaml` — change the number there and run `make k8s-up` (no rebuild needed; replicas are a Deployment field). `maxConcurrent` in the same block controls how many assignments one node holds simultaneously
- `ScanReasonNonUSA = "non_usa_location"` in `service/crawlernode/common/` — must match the DB CHECK constraint on `job_pipeline_ledger.reason`; never use the raw string
- Each service domain follows the decorator chain — see `backend/service/CLAUDE.md`

Frontend rule highlights (full set in `frontend/CLAUDE.md`):

- Server state: TanStack Query only — never `fetch` + `useState` for API data
- Shared UI state: Zustand; local-only: `useState` / `useReducer`
- Styling: Tailwind composed via `cn()` (`clsx` + `tailwind-merge`) — no inline styles
- Radix UI primitives for accessible complex components (dialogs, selects, tooltips, switches)

## Git policy

- `dev` is the integration branch — all feature branches PR to `dev`; only `dev` → `main` PRs promote to production
- Never commit or push directly to `main` or `dev` — `.husky/pre-commit` blocks both. Every change lands via PR, including hotfixes, doc-only changes, and config tweaks. Never run `git push origin <branch>:main`, `git push origin <branch>:dev`, or any equivalent
- Never use `git commit --no-verify` or any hook-skipping flag (`--no-gpg-sign`, `core.hooksPath=/dev/null`, …) — fix the underlying failure and recommit
- Hook is the floor: formatting, Go lint, Go unit tests, frontend lint, TypeScript check, and the protected-branch guard must all pass
- **No AI attribution** in commit messages or PR descriptions — no `Co-Authored-By: Claude`, no `🤖 Generated with Claude Code`, no equivalent. Author / `Co-Authored-By` reflect human contributors only. Holds even when the user says "commit this"

## Implementation workflow

Before writing code: confirm requirements, ask about anything ambiguous, map all edge cases.

During implementation: proceed autonomously, but pause and surface the tradeoff when a decision is ambiguous or has meaningful consequences — never silently pick.

## Testing and quality gates

Every feature must ship with:

- Tests alongside implementation — not after
- Frontend lint passing: `cd frontend && npm run lint`
- Backend lint passing: `cd backend && golangci-lint run --config ../.github/.golangci.yml --timeout 5m`
- Backend per-package coverage ≥95%: `scripts/check-coverage.sh /tmp/cov.out 95` (pre-commit + PR CI)
- Race detector clean: `go test -race -short ./...` (pre-commit) / `-count=1` (PR CI)
- Do not mark complete with any test, lint, race, or coverage failure outstanding

**Per-service test workflow.** Follow the 3-phase per-package flow in `.claude/agents/service-test-lead.md`: edge-case discovery (read-only) → numbered test plan (read-only) → implementation + QA. For multi-package backfill, dispatch one general-purpose agent per package in parallel.

**Coverage gate details.** Statement-weighted: `covered_statements / total_statements` per package. Excluded from the unit tier: `main.go`, `wire.go`, `wire_*.go`, `wire_gen.go`, `/compat/`, `/dto/`, `/models/`, `/testutil/`, plus broker/Postgres infrastructure (`common/db`, `common/jetstream`). The integration tier (`.github/workflows/integration.yml`) runs nightly + on infra-touching PRs against real Postgres/NATS/Redis via testcontainers; gate with `//go:build integration`.

After implementation: write a brief spec doc to `docs/superpowers/specs/` with date prefix — file-level references only, no code snippets, no filler.

## Plan execution

Always parallel via `superpowers:dispatching-parallel-agents`:

- Group independent tasks (no shared files, no sequential deps) into buckets
- Dispatch all buckets in one message with multiple `Agent` calls
- Serialise only truly dependent tasks
- Run one QA reviewer over the combined output after parallel work finishes
- Never execute plans inline in the same session
- After each parallel batch, verify changes actually landed: run `git status` and spot-read key output files — agent summaries describe intent, not persistence; worktree isolation can leave the main tree untouched

## Self-improving documentation

This file and the project's skill files evolve as the codebase is worked on. When you discover something that belongs here — a new project-wide invariant, an outdated rule, a non-obvious constraint — update it inline. Follow `skill-self-improvement` for the protocol (when/how). Do not add session-specific state, bug descriptions, or anything derivable by reading the code.

## See also

- `infra/CLAUDE.md` — k8s deployment + iteration (k3d is the only deployment target; no Docker Compose)
- `docs/QUALITY.md` — quality attributes reference (`.claude/rules/quality-attributes.md` auto-loads on backend files; `frontend-quality-attributes.md` on frontend files). Covers intrinsic qualities incl. Scalability + Performance — _"is the code good?"_
- `.claude/rules/engineering-principles.md` — hard rules applied globally: SOLID (strict), loose coupling, no magic values, extensibility by design, test-first, formatting — _"how must the code be written?"_
- `.claude/rules/dev-ergonomics.md` — developer-ergonomics model: friction to change, consistency, extensibility, agent-legibility — _"is it ergonomic to keep it good?"_ Audited by the `dev-ergonomics` skill (read-only, recommends; never edits) via the `dev-ergonomics-auditor` agent
- Subdirectory `CLAUDE.md` files cover each `backend/*` and `frontend/*` area and auto-load when you work there
- `.claude/skills/` — project skills; each has a self-improvement footer that invokes `skill-self-improvement` after completion

## Docs layout

All non-code documentation lives under `docs/`:

| Folder                    | Purpose                                                                                                            |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `docs/design/`            | Feature specs and design docs (active or future work)                                                              |
| `docs/ux/`                | UX reviews and UI mockups                                                                                          |
| `docs/knowledge/`         | Post-mortems and incident root-cause notes                                                                         |
| `docs/superpowers/specs/` | Agent-generated implementation specs (written after each feature lands)                                            |
| `docs/superpowers/plans/` | Agent-generated implementation plans (active only — delete when merged)                                            |
| `docs/tech-debt/`         | Tech debt reports produced by the `tech-debt` skill — one file per scored area, named `YYYY-MM-DD-<scope>-debt.md` |
| `docs/ARCHITECTURE.md`    | System architecture reference                                                                                      |
| `docs/QUALITY.md`         | Quality attributes reference                                                                                       |
| `docs/REFERENCE.md`       | Quick-reference for common commands and URLs                                                                       |
