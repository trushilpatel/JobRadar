# JobRadar

Production-grade automated job tracker. Monitors careers pages, filters via LLM, surfaces relevant jobs.

## Pipeline

```
scheduler → company_scan_queue (DB) → crawler → jobs.raw.<adapter> (NATS JetStream) → filter → jobs.filtered.<adapter> (NATS JetStream) → analyzer → jobs (DB) → API
```

Each binary lives in `backend/apps/<name>/` (wiring + worker loop only) and depends on domain logic in `backend/service/<domain>/`. Subdirectory `CLAUDE.md` files auto-load when you work in that area.

## Database tables

| Table                     | Role                                                                                                                                                                               |
| ------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `companies`               | Tracked company catalog; operational config lives in `company_scan_settings`                                                                                                       |
| `adapter_configs`         | Canonical adapter registry (PK `adapter_type`); `company_scan_settings` and `company_scan_queue` carry FK constraints to this table — never add raw adapter type strings elsewhere |
| `company_scan_settings`   | Per-company scan config (admin-managed); `adapter_type` FK → `adapter_configs`; rows missing here are skipped by workers                                                           |
| `company_scan_queue`      | Durable work queue for pending/in-progress scans; deduped by unique partial index; `adapter_type` FK → `adapter_configs`                                                           |
| `company_scan_log`        | Per-run scan summary (fetched, new, duration, error)                                                                                                                               |
| `job_pipeline_ledger`     | Dedup + pipeline blocking; `removed_at` marks disappearance; `reason='retry'` carries `retry_count`/`next_attempt_at`, written via filter `UpdateRetryBatch`                       |
| `jobs`                    | Operational feed — global rows; per-user status/notes live in `user_job_state`                                                                                                     |
| `user_job_state`          | Per-user overlay: saved_at, applied_at, status, notes, dismissal reason/note                                                                                                       |
| `job_analytics`           | Append-only snapshot from analyzer; survives job dismissal                                                                                                                         |
| `job_application_tracker` | Application stage tracking; `user_id NOT NULL`                                                                                                                                     |
| `users`                   | Registered users (id, email citext, display_name, status, timestamps)                                                                                                              |
| `roles`, `user_roles`     | RBAC0 catalog + join. Cascades: `user_id` CASCADE, `role_id` RESTRICT, `granted_by` SET NULL. Seeded by migration 043                                                              |
| `admin_audit_log`         | Append-only audit trail for admin mutations                                                                                                                                        |

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

All three phases shipped. Status doc: [`Designs/multi-user-rollout-plan.md`](Designs/multi-user-rollout-plan.md).

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
- All logging goes through `platform/logger.Logger` — never stdlib `log`; field keys come from `platform/logger/fields.go` only
- Crawler image (`infra/Dockerfile.crawler`) is **pure Go** — no Node, no Chromium, no Playwright. Do not add browser deps
- Each service domain follows the decorator chain — see `backend/service/CLAUDE.md`

Frontend rule highlights (full set in `ui/console/CLAUDE.md`):

- Server state: TanStack Query only — never `fetch` + `useState` for API data
- Shared UI state: Zustand; local-only: `useState` / `useReducer`
- Styling: Tailwind composed via `cn()` (`clsx` + `tailwind-merge`) — no inline styles
- Radix UI primitives for accessible complex components (dialogs, selects, tooltips, switches)

## Git policy

- Never commit or push directly to `main` — `.husky/pre-commit` blocks it. Every change lands via PR, including hotfixes, doc-only changes, and config tweaks. Never run `git push origin <branch>:main`, `git push origin HEAD:main`, or any equivalent
- Never use `git commit --no-verify` or any hook-skipping flag (`--no-gpg-sign`, `core.hooksPath=/dev/null`, …) — fix the underlying failure and recommit
- Hook is the floor: formatting, Go lint, Go unit tests, frontend lint, TypeScript check, and the main-branch guard must all pass
- **No AI attribution** in commit messages or PR descriptions — no `Co-Authored-By: Claude`, no `🤖 Generated with Claude Code`, no equivalent. Author / `Co-Authored-By` reflect human contributors only. Holds even when the user says "commit this"

## Implementation workflow

Before writing code: confirm requirements, ask about anything ambiguous, map all edge cases.

During implementation: proceed autonomously, but pause and surface the tradeoff when a decision is ambiguous or has meaningful consequences — never silently pick.

## Testing and quality gates

Every feature must ship with:

- Tests alongside implementation — not after
- Frontend lint passing: `cd ui/console && npm run lint`
- Backend lint passing: `cd backend && golangci-lint run --config ../.github/.golangci.yml --timeout 5m`
- Backend per-package coverage ≥80%: `scripts/check-coverage.sh /tmp/cov.out 80` (pre-commit + PR CI)
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

- `infra/CLAUDE.md` — local k8s deployment + iteration
- `docs/QUALITY.md` — quality attributes reference (`.claude/rules/quality-attributes.md` auto-loads on backend files; `frontend-quality-attributes.md` on frontend files)
- Subdirectory `CLAUDE.md` files cover each `backend/*` and `ui/console/*` area and auto-load when you work there
- `.claude/skills/` — project skills; each has a self-improvement footer that invokes `skill-self-improvement` after completion
