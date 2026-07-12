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

The crawler tier is **distributed pull-based**: the orchestrator (`backend/apps/orchestrator/`) exposes an HTTP API; one or more stateless crawler-node workers (`backend/apps/crawler-node/`) poll it for work, crawl the assigned company, and submit results back. Concurrency is controlled entirely by the replica count — no in-process worker pool needed. The ATS adapter library lives in `backend/apps/crawler-v2/adapters/` (listing + detail fetch); crawler-node uses it for listing and the analyzer uses it for `FetchJobPost`. The standalone v1 monolith crawler has been removed.

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
| `user_job_state`          | Per-user overlay: saved_at, applied_at, status, notes. `dismissal_reason`/`dismissal_note`/`dismissed_at` are historical-only — the dismiss feature was removed (migration 091); the CHECK constraint blocks new `dismissed` writes but existing rows are kept                  |
| `job_analytics`           | Append-only snapshot from analyzer; survives job dismissal                                                                                                                                                                                                                      |
| `job_application_tracker` | Application stage tracking; `user_id NOT NULL`                                                                                                                                                                                                                                  |
| `users`                   | Registered users (id, email citext, display_name, status, timestamps)                                                                                                                                                                                                           |
| `admin_audit_log`         | Append-only audit trail for admin mutations                                                                                                                                                                                                                                     |

## Job status flow

```
new → saved → applied    (user advances)
```

`user_job_state` is upserted on every save/status change. The "new" tab uses a global feed query with a per-user overlay (`GetMany` on the fetched IDs); other status tabs INNER JOIN `user_job_state → jobs` so every page returns a full limit-sized result. Historical analytics read `job_analytics` (snapshot rows never deleted); current-state aggregations (funnel counts) read `user_job_state.status`.

**Dismiss has been removed** (UI, API, and the ability to write new `dismissed` rows — migration 091). Historical `status='dismissed'` rows are intentionally left in the DB forever: migration 084 used that status to backfill-hide non-USA jobs for every user, and the "new" tab's exclusion logic is simply "status IS NULL" in application code, so deleting or reassigning those rows would resurface them. `jobcommon.JobStatusDismissed` and the `user_job_counters.dismissed_count` bucket remain in code for this reason — never remove them without re-verifying the non-USA-hiding mechanism and the "New" tab count math (`New = total - sum(workflow buckets)`) stay correct.

## Scan scheduling

- `SCAN_INTERVAL_MINUTES` (default `1440` = 24h) controls per-company cadence
- Scheduler ticks every 1 min; only enqueues companies where `last_scan_at` is NULL or older than the interval
- Manual trigger (`POST /api/companies/:id/scan`) bypasses the cooldown — always enqueues immediately
- Workers skip companies with no `company_scan_settings` row (logged, not errored)

## Multi-user

All three phases shipped.

- **Decorator chain:** `Rest → Validation → Cache → Impl → Repo`. `CacheService` is optional per slice.
- **Identity flow:** repos take `userID` explicitly; services pull from ctx via `identity.FromContext` / `UserIDFrom` / `IsAdmin`.
- **RBAC:** roles come from Clerk `publicMetadata.role` (a scalar string injected into the JWT via a Clerk JWT template). Source of truth is Clerk dashboard — never a DB roles table. Set admin: Clerk dashboard → User → publicMetadata → `{"role":"admin"}`. `RequireRole("admin")` reads `identity.Roles` set by `RequireIdentity` from the JWT claim; no DB lookup.
- **Route split:** `/api/*` uses `RequireIdentity` + `RateLimit`; `/admin/*` adds `RequireRole("admin")` + `AuditAdminMutation`.
- **Job feed:** "new" = global feed + per-user overlay; other tabs = `user_job_state` INNER JOIN; cursor (keyset) pagination.
- **Company slice:** split into public `CompanyService` and admin `CompanyAdminService` paths. List endpoint returns `CompanyAdminListView` (no `adapter_config`); detail endpoint returns full `CompanyAdminView` — never expose adapter credentials in paginated list responses.
- **Cursor (keyset) pagination:** all list endpoints use `next_cursor` / `cursor` params, not `page` / `offset`. Frontend uses a `cursorHistory` stack for prev/next navigation.
- **Frontend:** identity via `useIdentity()` hook (wraps Clerk `useAuth`/`useUser` + TanStack Query `fetchMe`), role-gated admin nav (pipeline / companies / users / audit log / insights), 429 toast.

Env vars: `CLERK_JWKS_URL`, `CLERK_ISSUER_URL`, `CLERK_SECRET_KEY`, `VITE_CLERK_PUBLISHABLE_KEY`, `RATE_LIMIT_{READ,WRITE,ADMIN}_RPM`, `CACHE_TTL_USER_SECONDS`.

## Secret management

All secrets and config values are managed in **Doppler** (project `job-radar`, configs `dev` / `prod`). The `doppler.yaml` at the repo root binds the project — no manual env file setup is needed.

- **New developer onboarding:** `doppler login && doppler setup` — one-time, then all commands just work.
- **Running services locally:** every `make dev*`, `make run-*`, and `make migrate` target already wraps with `doppler run --`. Do not source `.env` files manually.
- **k8s bootstrap:** `make k8s-bootstrap` fetches secrets from Doppler at runtime (`DOPPLER_CONFIG=prod make k8s-bootstrap` for production). No `.env` file is read.
- **Changing a secret:** update it in the Doppler dashboard or via `doppler secrets set KEY=value --config dev`. The next `doppler run --` picks it up — no file edits, no restarts of other processes.
- **Local snapshot (rare):** `doppler secrets download --format env-no-quotes --no-file > .env.local` (gitignored) for tools that can't use `doppler run --`.
- Never create or commit a `.env` file with real values. `.env.example` (committed) is the canonical list of keys — values live in Doppler only.
- **Frontend proxy target:** `API_PROXY_TARGET` in Doppler controls where the Vite dev server forwards `/api` calls. Dev default: `http://localhost:8080` (local Go API via air). Set to `http://localhost:30080` to target k3d instead.
- **Vite proxy routing rule:** user API routes (`/api/*`) are forwarded **with** the `/api` prefix intact (Go chi router mounts at `/api/`). Admin routes (`/api/admin/*`) have the prefix stripped (Go registers admin groups at root `/admin/*`). Never add a blanket rewrite that strips `/api` from all paths — it breaks every user-facing endpoint.
- **Local dev stack:** `make dev` is the single entry point — kills stale port processes, migrates, starts API (air hot-reload on :8080) and Vite HMR (:5173) in parallel. No Docker or k3d needed for UI+API work. See `.claude/skills/run/SKILL.md` for the full runbook.
- **Frontend-only dev (no local API):** `cd frontend && API_PROXY_TARGET=http://localhost:30080 npx vite --port 5173` — starts Vite HMR proxying to the k3d API at `:30080`. Use this when only editing UI files and the local Go API isn't running. Clerk auth is port-scoped: sign in once at http://localhost:5173 and the session persists for that port.

## Dev iteration rule — Vite first, k3d only for final verification

**Never use k3d/Docker builds during UI development.** The inner loop for frontend changes is:

1. Edit source files in `frontend/src/`
2. Vite HMR reflects changes instantly at http://localhost:5173 — no build step, no restart
3. Verify visually in the browser at http://localhost:5173

**k3d is for final deployment verification only** — rebuild the console image (`make k8s-build-console`) only once, after all UI changes are complete and lint/tests pass, to confirm the production build works. Never rebuild k3d mid-iteration to check a CSS or component change.

A worktree Vite server (`.worktrees/*/frontend/`) runs on `:5173` and serves a different branch — **do not edit main-repo files and check them against a worktree Vite server**. If `:5173` is serving a worktree, kill it (`lsof -ti:5173 | xargs kill`) and start the main-repo server.

## Key rules

- Workers depend on **service interfaces**, never on concrete store types or GORM
- All DB access goes through `{Domain}RepositoryService.go` — no raw GORM elsewhere
- Domain errors come from `common/domain/` (`domain.ErrNotFound`, `domain.ErrInternal`, …) — never `fmt.Errorf` for sentinel errors handlers inspect
- Wrap errors with `fmt.Errorf("context: %w", err)` at every layer; log only at top-level boundaries via `platform/logger.Logger`
- API responses use `handler.Respond[T]`, `handler.RespondPaged[T]`, `handler.RespondError` — never raw `json.NewEncoder` in handlers
- Typed values (status, stage, reason) use constants from the owning service's `common/` package — never raw strings
- Insights: `job_analytics` for historical / time-series; `user_job_state.status` for current per-user aggregations
- Filter batch failures mark jobs `error` in `job_pipeline_ledger` — never re-queue to NATS
- Ledger/queue UPDATE store methods check `RowsAffected` and return a sentinel (`ErrLedgerRowMissing` / `ErrQueueRowMissing`) when 0/partial rows match — never silently return nil. Workers act on it: filter Naks for redelivery (closes the crawler-INSERT vs filter-UPDATE race), analyzer treats it as non-fatal. The crawler-node writes the `fetch` ledger row **before** publishing to NATS so the row exists before the message can be consumed
- All logging goes through `platform/logger.Logger` — never stdlib `log`; field keys come from `platform/logger/fields.go` only
- The crawler tier images — crawler-node (`infra/Dockerfile.crawler-node`) and orchestrator (`infra/Dockerfile.orchestrator`) — are **pure Go**: no Node, no Chromium, no Playwright. The ATS adapter library (`apps/crawler-v2/adapters/`) speaks plain HTTP/JSON/GraphQL/sitemap. Do not add browser deps to either image
- Crawler-node count is controlled by `crawlerNode.replicas` in `k8s/helm/jobradar/values.local.yaml` — change the number there and run `make k8s-up` (no rebuild needed; replicas are a Deployment field). `maxConcurrent` in the same block controls how many assignments one node holds simultaneously
- `ScanReasonNonUSA = "non_usa_location"` in `service/crawlernode/common/` — must match the DB CHECK constraint on `job_pipeline_ledger.reason`; never use the raw string
- Each service domain follows the decorator chain — see `backend/service/CLAUDE.md`

Frontend rule highlights (full set in `frontend/CLAUDE.md`):

- Server state: TanStack Query only — never `fetch` + `useState` for API data
- Shared UI state: Zustand; local-only: `useState` / `useReducer`
- Styling: Tailwind composed via `cn()` (`clsx` + `tailwind-merge`) — no inline styles
- Radix UI primitives for accessible complex components (dialogs, selects, tooltips, switches)
- **UI component boundary:** `components/ui/` = shadcn/ui origin OR pure custom (data in → render out → callbacks out). Zero `useEffect` with side effects. Zero business logic. Domain/navigation decisions belong in hooks.
- **New component decision tree:** (1) shadcn has it → `npx shadcn@latest add`; (2) Radix has a headless primitive → thin styled wrapper; (3) neither → pure custom with PR justification.
- **Forbidden:** `createPortal` directly (use Radix Portal/shadcn Popover); `useInfiniteQuery` for page-by-page cursor nav (use `useQuery` + cursor history — see `features/jobs/hooks/useJobs.ts`); dropdowns/multi-selects built from scratch (use `Popover + Command + Checkbox`).
- **Theme-adaptive colors in JS must use CSS custom properties** — colors used in inline `style` props, Recharts `stroke`/`fill`, or JS color arrays must be `'var(--name)'` strings referencing pairs defined in `index.css` (`:root` = light, `.dark` = dark). The `dark:` Tailwind prefix is only valid inside `className` strings, never in JS values.

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

**Coverage gate details.** Statement-weighted: `covered_statements / total_statements` per package. Excluded from the unit tier: `main.go`, `wire.go`, `wire_*.go`, `wire_gen.go`, `/compat/`, `/dto/`, `/models/`, `/testutil/`, plus broker/Postgres infrastructure (`common/db`, `common/jetstream`). The integration tier exists in code with `//go:build integration` but has no active CI workflow — run locally via `go test -tags=integration -race ./...` against real Postgres/NATS/Redis.

**End-to-end (Playwright).** `frontend/e2e/` is a full-flow browser suite (both user and admin roles) against a live stack — see `frontend/e2e/CLAUDE.md` for conventions, known gaps, and the how-to. Gated in CI on every dev→main PR (`.github/workflows/ci.yml`'s `e2e` job — queues rather than cancels on overlap, since every run hits the same shared dev Supabase DB and Clerk test accounts). Run it locally too (`cd frontend && npm run test:e2e` with `make dev` already running) when touching a flow it covers — Jobs, Account, Resumes, Apply Queue, auth/admin-gating, or any admin page.

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

- `infra/CLAUDE.md` — k8s deployment + iteration (k3d for local dev) **and OCI production deployment** (two ARM A1 nodes: INFRA=serving, APP=pipeline). Full CI/CD cycle, bootstrap script, GitHub secrets, and failure modes documented there.
- `docs/QUALITY.md` — quality attributes reference (`.claude/rules/quality-attributes.md` auto-loads on backend files; `frontend-quality-attributes.md` on frontend files). Covers intrinsic qualities incl. Scalability + Performance — _"is the code good?"_
- `.claude/rules/engineering-principles.md` — hard rules applied globally: SOLID (strict), loose coupling, no magic values, extensibility by design, test-first, formatting — _"how must the code be written?"_
- `.claude/rules/dev-ergonomics.md` — developer-ergonomics model: friction to change, consistency, extensibility, agent-legibility — _"is it ergonomic to keep it good?"_ Audited by the `dev-ergonomics` skill (read-only, recommends; never edits) via the `dev-ergonomics-auditor` agent
- Subdirectory `CLAUDE.md` files cover each `backend/*` and `frontend/*` area and auto-load when you work there
- `frontend/e2e/CLAUDE.md` — Playwright E2E suite: how to run it, page-object/auth conventions, known gaps, real bugs it has surfaced
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
