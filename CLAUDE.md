# JobRadar

Single-user automated job tracker. Monitors careers pages, filters via LLM, surfaces relevant jobs.

Full design spec: `Designs/job-tracker-final-architecture.md`

## Pipeline

```
company_scan_scheduler → company_scan_queue (DB) → job_listing_crawler → job_listings.raw (Kafka) → job_listing_relevance_filter → job_listings.filtered (Kafka) → job_post_analyzer → job_feed (DB) → API
```

## Directory map

| Path                           | What lives there                                                                  |
| ------------------------------ | --------------------------------------------------------------------------------- |
| `backend/cmd/`                 | Binary entry points — one per worker + api                                        |
| `backend/common/config/`       | Config struct + env var loading via godotenv                                      |
| `backend/common/domain/`       | Shared error types: ServiceError, ValidationError, sentinel errors + `IsNotFound` |
| `backend/pipeline/`            | The entire data pipeline                                                          |
| `backend/pipeline/ats/`        | Careers page fetchers (Greenhouse, Lever, Ashby, Google, Netflix, Uber)           |
| `backend/pipeline/crawler/`    | Polls scan queue, fetches careers pages, publishes raw jobs                       |
| `backend/pipeline/filter/`     | Scores batches via LLM, routes relevant jobs                                      |
| `backend/pipeline/analyzer/`   | Fetches full job page, LLM enriches, writes to job_feed                           |
| `backend/pipeline/techtag/`    | Normalizes and deduplicates raw tech_stack strings from LLM                       |
| `backend/service/`             | Feature-slice packages — each owns handler + service + store + models + validator |
| `backend/service/job/`         | Job listing operations — models in `job/models/` (package `jobmodel`)             |
| `backend/service/company/`     | Company CRUD + scan trigger — models in `company/models/` (package `companymodel`)|
| `backend/service/application/` | Application tracking — models in `application/models/` (package `appmodel`)       |
| `backend/service/dashboard/`   | Dashboard stats + pipeline status                                                 |
| `backend/service/insights/`    | Analytics and insights queries                                                    |
| `backend/service/scan/`        | Scan result recording — models in `scan/models/` (package `scanmodel`)            |
| `backend/service/llm/`         | LLM business logic — `JobsRelevanceFilter` + `JobPostEnricher` interfaces, Gemini impl, prompts |
| `backend/api/`                 | Router, respond helpers, middleware, RouteGroup interface                         |
| `backend/infra/`               | Infrastructure plumbing (db, kafka, logger, metrics, tracing)                     |
| `backend/infra/tracing/`       | OTel tracing init, Start/End span helpers, Kafka propagation                      |
| `backend/migrations/`          | goose SQL migration files                                                         |
| `backend/deploy/`              | Docker / Kafka / Loki / Prometheus entrypoint scripts                             |

## Workers

| Binary                             | Role                                                                                              |
| ---------------------------------- | ------------------------------------------------------------------------------------------------- |
| `cmd/company_scan_scheduler`       | Queries DB for companies due for a scan, inserts into `company_scan_queue`                        |
| `cmd/job_listing_crawler`          | Polls `company_scan_queue` (DB), fetches careers pages, publishes to `job_listings.raw`           |
| `cmd/job_listing_relevance_filter` | Scores batches via LLM, routes relevant to `job_listings.filtered`                                |
| `cmd/job_post_analyzer`            | Fetches full job page (or uses pre-fetched FullContent), LLM enriches, writes to `job_feed` table |
| `cmd/api`                          | REST API on `:8080`                                                                               |

## Database tables

| Table                     | Role                                                                                      |
| ------------------------- | ----------------------------------------------------------------------------------------- |
| `companies`               | Tracked companies                                                                         |
| `company_scan_queue`      | Durable work queue for pending/in-progress company scans; deduped by unique partial index |
| `job_pipeline_ledger`     | Dedup + pipeline blocking; `removed_at` tracks when a job disappears from a careers page  |
| `company_scan_log`        | Per-run scan summary (fetched, new, duration, error)                                      |
| `job_feed`                | Operational feed — only `new`, `saved`, `applied`, `dismissed` rows live here             |
| `job_analytics`           | Append-only analytics record written by job_post_analyzer; survives job dismissal         |
| `job_application_tracker` | Application stage tracking                                                                |
| `job_dismissal_log`       | Dismiss log with reason and note                                                          |

## Job status flow

```
new → saved → applied    (user advances)
new → dismissed          (user dismisses — job row kept for FK integrity, hidden from feed)
```

`dismissed` jobs still exist in `job_feed` but are excluded from feed queries (`WHERE status = 'new'`). Analytics come from `job_analytics`, not `job_feed`.

## Scan scheduling

- `SCAN_INTERVAL_MINUTES` (default `1440` = 24h) controls how often each company is scanned
- Scheduler checks every 1 min; only enqueues companies where `last_scan_at` is NULL or older than the interval
- Manual trigger (`POST /companies/:id/scan`) bypasses the cooldown — always enqueues immediately

## Dashboard stats fields

`GET /dashboard/stats` returns:

| Field                | Description                                                                         |
| -------------------- | ----------------------------------------------------------------------------------- |
| `jobs_scanned_today` | Total job pages fetched today across all companies                                  |
| `jobs_new`           | Jobs in feed with status `new`                                                      |
| `jobs_saved`         | Jobs with status `saved`                                                            |
| `jobs_applied`       | Jobs with status `applied`                                                          |
| `pipeline_pending`   | Jobs in `job_pipeline_ledger` with `reason='fetch'` but not yet in `job_feed` table |
| `pipeline_errors`    | Jobs in `job_pipeline_ledger` with `reason='error'`                                 |

## Key rules (detail in subdirectory CLAUDE.md files)

- Workers depend on **interfaces**, never concrete types
- All DB access goes through `{Domain}RepositoryService.go` — no raw GORM outside those files
- Domain errors come from `common/domain/` (`domain.ErrNotFound`, `domain.ErrInternal`, etc.) — never use `fmt.Errorf` for sentinel errors that handlers inspect
- All typed values (status, stage, reason) use constants from the owning service's `common/` package — never raw strings
- Insights query `job_analytics`, not `job_feed` — snapshot rows are never deleted
- job_listing_relevance_filter batch failures mark jobs as `error` in `job_pipeline_ledger` — never re-queue to Kafka
- All logging goes through `infra/logger.Logger` — never use stdlib `log` package directly
- Field keys must use constants from `infra/logger/fields.go` — never raw strings
- Each service domain follows the decorator chain pattern — see `backend/service/CLAUDE.md`

## Implementation workflow

Before writing any code:

- Gather and confirm all requirements; ask for clarification on anything ambiguous
- Map all edge cases before starting implementation
- Only begin coding once edge cases are fully identified and understood

During implementation:

- Proceed autonomously
- Pause and surface the tradeoff when a decision is ambiguous or has meaningful consequences — do not silently pick one

## Backend code standards

These extend the rules in `.claude/rules/backend-solid.md` and `.claude/rules/backend-architecture.md`.

- Keep structs small with one clear reason to change
- Prefer composition; keep interfaces small and focused — one interface per concern
- Add new behaviour via new types/functions; do not modify working code to accommodate it
- Every abstraction must be reusable and extendable without touching existing implementations
- Error handling: wrap with `fmt.Errorf("context: %w", err)` at every layer; log only at top-level boundaries via `infra/logger.Logger`
- API responses: always use `Respond[T]`, `RespondPaged[T]`, `RespondError` helpers — never write raw `json.NewEncoder` directly in handlers

## Frontend code standards

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
- Frontend linting passing: `cd frontend && npm run lint`
- Backend linting passing: `cd backend && golangci-lint run --config ../.github/.golangci.yml --timeout 5m`
- Do not mark a task complete if any test or lint check fails — fix the failure first

After implementation, write a brief spec-style doc:

- File-level references only (which file does what) — no code snippets
- Concise — no padding or filler prose
- Saved to `docs/superpowers/specs/` with date prefix

## Plan execution mode

When executing an implementation plan, always use the **subagent-driven approach**:

- Dispatch a fresh subagent per task using the `superpowers:subagent-driven-development` skill
- Review each task's output before dispatching the next
- Never execute plans inline in the same session
