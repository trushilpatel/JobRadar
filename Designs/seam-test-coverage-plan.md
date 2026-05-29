# Seam Test Coverage — Living Plan & Status

**Goal:** Stop the recurring "we fixed A and broke B" class of regressions across the
console frontend by hardening the **seams between features** — page by page — with
typed contracts, interaction/contract tests, and a zero-`any` typing bar.

This is a **living document**. It is the source of truth for progress and is written so
**any agent can resume cold** if the current one stops or hits a limit.

---

## ⏯️ Resume here (read first)

1. Invoke the **`seam-test-coverage`** skill and read its Methodology + Seam Catalog.
2. Find the first page in the **Status table** below whose status is not `Done`.
   - `In progress` → read that page's section for what already landed; check `git status` /
     `git log` / open branches; continue from the `> RESUME HERE` note if present.
   - `Not started` → begin the per-page workflow.
3. Do **exactly one page**, then stop at the checkpoint (gates → update this doc → commit →
   PR via `git-push-to-pr`).
4. Update this doc **before** you stop, always leaving the repo resumable.

---

## Methodology (summary — full version in the skill)

Per page, audit 4 seam categories + enforce 1 typing gate:

1. **Navigation seams** — cross-feature `navigate`/`<Link>` carrying params → route through a
   **typed URL/param builder** (e.g. `jobsUrl()`); interaction test proves the param transmits
   and no stray `location.state` is used.
2. **Shared-state seams** — `nuqs` URL state + Zustand: one exported source-of-truth shape;
   round-trip test between writer and reader.
3. **Cache seams** — every mutation's `invalidateQueries` keys come from the query-key factory;
   test asserts the exact keys are invalidated.
4. **API contracts** — fixture-based normalization tests per `api/*.ts` (server `null`/missing →
   typed, non-null output).
5. **Typing gate** — `@typescript-eslint/no-explicit-any` as an **error**; sweep the page's slice
   to **zero `any` / no untyped bindings**. A page is not `Done` while an `any` remains in its files.

**Gates per page (all must pass):**
`cd ui/console && npm run lint && npx tsc --noEmit && npx vitest run && npm run build`

**Workflow per page:** map seams → add typed contracts → write tests TDD (red→green) →
sweep `any` → run gates → update this doc → commit → `git-push-to-pr`. One PR per page.

---

## Scope

**In scope (in order):** Jobs → Companies (admin) → Dashboard → Insights → Settings →
Account → Admin Users → Admin Audit.

**Out of scope (deferred):** Resume Library / Detail / Editor pages, Apply-Queue
(resume/auto-apply-coupled) — revisit later as a separate effort.

**Checkpoint:** after the **Jobs** PR, pause for user confirmation of the pattern before
running the loop across the remaining pages.

---

## Status table

| #   | Page                            | Route              | Seams found                     | Tests added                  | `any` removed | PR  | Status          |
| --- | ------------------------------- | ------------------ | ------------------------------- | ---------------------------- | ------------- | --- | --------------- |
| 0   | Phase 0: strict typing (global) | —                  | n/a                             | n/a                          | n/a           | —   | Not started     |
| 1   | Jobs                            | `/jobs`            | nav: company→jobs (shipped PR1) | jobsUrl + drawer guard (PR1) | _tbd_         | —   | **In progress** |
| 2   | Companies (admin)               | `/admin/companies` | _tbd_                           | _tbd_                        | _tbd_         | —   | Not started     |
| 3   | Dashboard                       | `/dashboard`       | _tbd_                           | _tbd_                        | _tbd_         | —   | Not started     |
| 4   | Insights                        | `/insights`        | _tbd_                           | _tbd_                        | _tbd_         | —   | Not started     |
| 5   | Settings                        | `/settings`        | _tbd_                           | _tbd_                        | _tbd_         | —   | Not started     |
| 6   | Account                         | `/account`         | _tbd_                           | _tbd_                        | _tbd_         | —   | Not started     |
| 7   | Admin Users                     | `/admin/users`     | _tbd_                           | _tbd_                        | _tbd_         | —   | Not started     |
| 8   | Admin Audit                     | `/admin/audit`     | _tbd_                           | _tbd_                        | _tbd_         | —   | Not started     |

Status values: `Not started` · `In progress` · `In review` (PR open) · `Done` (PR merged).

---

## Per-page detail

### 1. Jobs (`/jobs`) — In progress

**Already landed in the working tree** (seed of this effort, not yet committed):

- Typed `jobsUrl()` builder in `ui/console/src/features/jobs/hooks/useJobUrlState.ts`,
  derived from the page's `nuqs` PARSERS; exported from the jobs barrel.
- Fixed nav seam: `CompanyDetailDrawer` "View N new jobs" now navigates via `jobsUrl({companies})`
  instead of the silently-dropped `location.state`.
- Tests: `jobsUrl` builder unit tests; `CompanyDetailDrawer` interaction guard test
  (asserts URL query carries the company + no `state` arg).

> RESUME HERE: complete the rest of the Jobs-page seam audit —
>
> - **Nav:** other links into/out of Jobs (mobile job route `/jobs/:id` uses `location.state.idx` — verify it has a typed contract + test; row-click navigation).
> - **Shared-state:** `useJobUrlState` ↔ `useJobs` filter shape; hidden-companies localStorage store.
> - **Cache:** `useJobActions` mutations (setStatus, dismiss, setSaved) — assert they invalidate the exact `jobQueryKeys.list/counts` other views rely on.
> - **API contracts:** `api/jobs.ts` (`fetchJobs`, `fetchJobCounts`, `fetchJobCompanies`, `normalizeJob`) fixture tests incl. null/missing `user_state`.
> - **Typing:** grep `features/jobs`, `pages/JobsPage.tsx`, `api/jobs.ts` for `any`/`as any`/`as unknown as`; remove.
>   Then run gates, update this row, commit, PR, and STOP at the checkpoint.

### 2. Companies (admin) (`/admin/companies`) — Not started

_To fill during the page's run._

### 3. Dashboard (`/dashboard`) — Not started

_To fill during the page's run._

### 4. Insights (`/insights`) — Not started

_To fill during the page's run._

### 5. Settings (`/settings`) — Not started

_To fill during the page's run._

### 6. Account (`/account`) — Not started

_To fill during the page's run._

### 7. Admin Users (`/admin/users`) — Not started

_To fill during the page's run._

### 8. Admin Audit (`/admin/audit`) — Not started

_To fill during the page's run._

---

## Decision log

- **2026-05-28** — Scope set to broadest seam coverage incl. API contracts; fixture-based
  API-contract approach chosen (no new runtime-validation dependency). Plan + skill live at
  `Designs/seam-test-coverage-plan.md` and `.claude/skills/seam-test-coverage/`. Resume + zero-`any`
  bar baked into the methodology. Resumes/Apply-Queue deferred.
