---
name: quality-team
description: Use when asked to review, audit, or QA a feature, page, service, or the full product — covering architecture, code quality, API design, DB performance, missing tests (backend + frontend), UX gaps, security, and observability. Triggers on phrases like "check the Jobs page", "review the API", "QA the filter service", "what's missing", or any comprehensive quality review request.
---

# Quality Team

## Overview

Coordinates a team of specialized review agents dispatched in parallel, each owning a distinct quality dimension. A synthesizer then consolidates all findings into a single prioritized action plan.

**Required sub-skill:** `superpowers:dispatching-parallel-agents` — use it for agent dispatch.

## Step 1 — Establish Scope

Before dispatching, clarify scope with the user if not stated:

- **Target**: which feature, page, service, or "full product"?
- **Layers**: full-stack, backend-only, or frontend-only?
- **Entry points**: e.g., `backend/service/jobs/`, `ui/console/src/features/jobs/`

If the user's message is clear (e.g., "check the Jobs UI page and API"), derive scope from it — don't ask.

Read `CLAUDE.md` (root + any relevant subdirectory) to load project rules before dispatching.

## Step 2 — Dispatch the Team

Dispatch **all applicable agents in parallel** (skip agents irrelevant to scope; Security always runs). Use `superpowers:dispatching-parallel-agents`.

---

### Agent 1 — Architecture Reviewer

**Reads:** service directory tree, interface definitions, `wire_gen.go`, decorator chain files, `CLAUDE.md`

**Finds:**

- Violations of decorator chain (`Rest → Validation → Cache → Impl → Repo`)
- Concrete types leaking across boundaries
- Workers calling other workers or importing domain implementations directly
- Service domains that grew beyond one concern
- Missing or misused abstractions

---

### Agent 2 — Backend Code Quality Reviewer

**Reads:** Service implementations, repository files, handlers in scope

**Finds:**

- Raw GORM outside repository layer
- `fmt.Errorf` used as sentinel errors (should use `domain.Err*`)
- Errors not wrapped with `fmt.Errorf("ctx: %w", err)` at each layer
- Logging via stdlib `log` or `fmt.Println` instead of `platform/logger.Logger`
- Typed values (status, stage, reason) expressed as raw strings
- Responses not using `handler.Respond[T]` / `handler.RespondError`

---

### Agent 3 — API Design Reviewer

**Reads:** Handler files, DTO structs, route definitions, middleware chain

**Finds:**

- Inconsistent response envelope shapes
- Wrong HTTP status codes (e.g., 200 on not-found, 500 on validation failure)
- Missing or broken pagination (should use keyset cursor)
- Endpoints missing auth/rate-limit middleware that should have it
- Missing or inconsistent error response bodies

---

### Agent 4 — Database Reviewer

**Reads:** Repository SQL queries, migration files, `CLAUDE.md` table descriptions

**Finds:**

- Missing indexes for common query predicates or JOIN conditions
- N+1 query patterns (loop + single-row query)
- Missing `CHECK` constraints for constrained value columns
- Unsafe migrations (adding NOT NULL without default on large table, no rollback)
- `removed_at` / soft-delete columns queried without index

---

### Agent 5 — Backend Test Auditor

**Reads:** All `*_test.go` files alongside their implementation counterparts

**Finds:**

- Implementation files with no corresponding test file
- Happy-path-only tests missing error path coverage
- Missing table-driven test cases for validation logic
- Tests that use real GORM/Postgres where an interface double would do
- Missing race-safe assertions (channels, atomic reads)
- Functions/methods below 80% statement coverage (estimate from reading, flag)

---

### Agent 6 — Frontend Code Quality Reviewer

**Reads:** Feature directories under `ui/console/src/features/`, shared components, `api/*.ts`, hooks

**Finds:**

- Direct `fetch()` calls outside `lib/api-client.ts`
- `useEffect` + `useState` for server data (should be TanStack Query)
- `any` types or untyped API responses
- Inline styles (should use Tailwind via `cn()`)
- Components that fetch AND render AND manage complex local state (split them)
- Missing `queryClient.invalidateQueries()` after mutations

---

### Agent 7 — Frontend Test Auditor

**Reads:** `*.test.tsx` / `*.test.ts` files alongside components, hooks, and API functions

**Finds:**

- Components with zero test coverage
- Hooks not tested via `renderHook`
- Missing error-state test cases (API failure, 401, empty response)
- Real `fetch` called in tests (should mock at fetch boundary)
- Missing loading-state assertions

---

### Agent 8 — UX Completeness Reviewer

**Reads:** Page components, list/table components, form components, data-fetching hooks

**Finds:**

- Missing loading skeletons (data-fetching without pending state UI)
- Missing empty states (list/table with no items — no illustration or message)
- Missing error states (query error with no user-facing message)
- Destructive actions without confirmation dialogs
- Forms missing field-level validation messages
- Interactive elements missing keyboard/focus support
- Missing 429 toast handling on rate-limited endpoints

---

### Agent 9 — Security Reviewer

**Reads:** Middleware, auth guards, handlers, input validation, any auth-touching code

**Finds:**

- Routes missing `RequireIdentity` or `RequireRole` that should have them
- User-supplied input reaching DB queries without sanitization
- IDs in responses that expose other users' data (missing user-scoping)
- Missing CORS or overly permissive CORS on sensitive endpoints
- Secrets in logs, responses, or error messages
- Missing rate limiting on write or auth endpoints

**Note:** Always run this agent regardless of scope.

---

### Agent 10 — Observability Reviewer

**Reads:** Service and worker implementations, logger usage, trace span setup

**Finds:**

- Errors caught and dropped (logged at wrong level or silently swallowed)
- Log statements missing required structured fields (use constants from `platform/logger/fields.go`)
- OTel spans missing on cross-service boundaries
- Kafka consumer processing loops with no per-job log context
- `company_scan_log` or `job_pipeline_ledger` writes missing for significant events

---

## Step 3 — Synthesize

After all agents complete, dispatch one final **Synthesizer** agent with all reports as input:

> Given these specialist reports, produce a single consolidated output:
>
> 1. **Executive Summary** — 3–5 bullets on the biggest gaps found
> 2. **P0 — Critical** — broken behavior, security holes, data loss risk (fix before anything else)
> 3. **P1 — Important Gaps** — meaningful missing coverage, pattern violations, UX blockers
> 4. **P2 — Improvements** — quality upgrades, consistency wins, observability additions
> 5. **Test Backfill Checklist** — all missing tests as checkboxes, grouped by package/feature, copy-paste ready
> 6. **Cross-cutting Themes** — patterns that appeared in 3+ agent reports
> 7. **Suggested Order of Attack** — P0s first, then quick wins (< 30 min each), then larger investments
>
> For every item: what, where (file path or component name), why it matters, effort (S / M / L).
> Deduplicate across agent reports. Do not repeat findings already addressed by another P0.

## Step 4 — Present to User

Deliver:

1. **Executive summary** (top of response — 3–5 bullets)
2. **Full prioritized findings** (P0 → P1 → P2)
3. **Test backfill checklist** (checkboxes, grouped, copy-paste ready)
4. **Suggested order of attack**

If scope was full-product, note which areas were **not** covered due to agent count limits and offer a follow-up pass.

## Common Mistakes

| Mistake                                            | Fix                                                                               |
| -------------------------------------------------- | --------------------------------------------------------------------------------- |
| Skipping security agent for "just UI" reviews      | Security always runs — auth bugs live in frontend too                             |
| Dispatching agents without reading CLAUDE.md first | Project rules define what counts as a violation; read them first                  |
| Synthesizer only summarizing without deduplication | Require explicit dedup: "do not repeat findings already in P0"                    |
| Presenting all 10 agent raw reports to the user    | Always synthesize — raw reports are internal; the user gets the consolidated view |
| Scope creep — agent reading the entire codebase    | Give each agent a directory/file glob; bound their search                         |

---

## Self-Improvement

After this skill completes, invoke `skill-self-improvement` to check whether any finding warrants updating this file or `CLAUDE.md`.
