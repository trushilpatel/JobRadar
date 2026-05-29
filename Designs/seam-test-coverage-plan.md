# Seam Test Coverage — Living Plan & Status

**Goal:** Stop the recurring "we fixed A and broke B" class of regressions across the console frontend by hardening the **seams between features** — page by page — with typed contracts, interaction/contract tests, and a zero-`any` typing bar.

This is a **living document**. It is the source of truth for progress and is written so **any agent can resume cold** if the current one stops or hits a limit.

---

## ⏯️ Resume here (read first)

1. Invoke the **`seam-test-coverage`** skill and read its Methodology + Seam Catalog.
2. Find the first page in the **Status table** below whose status is not `Done`.
   - `In progress` → read that page's section for what already landed; check `git status` / `git log`; continue from the `> RESUME HERE` note if present.
   - `Not started` → begin the per-page workflow.
3. Do **exactly one page**, then stop at the checkpoint (gates → update this doc → commit → PR via `git-push-to-pr`).
4. Update this doc **before** you stop.

---

## Methodology (summary — full version in the skill)

Per page, audit 4 seam categories + enforce 1 typing gate:

1. **Navigation seams** — cross-feature `navigate`/`<Link>` carrying params → route through a **typed URL/param builder**; interaction test proves the param transmits and no stray `location.state` is used.
2. **Shared-state seams** — `nuqs` URL state + Zustand: one exported source-of-truth shape; round-trip test between writer and reader.
3. **Cache seams** — every mutation's `invalidateQueries` keys come from the query-key factory; test asserts the exact keys are invalidated.
4. **API contracts** — fixture-based tests (server `null`/missing → typed, non-null output). Covered globally by Phase 0.5 zod schemas.
5. **Typing gate** — `no-restricted-imports` is now an **error** (architecture complete); sweep the page's slice to **zero `any`**.

**Gates per page (all must pass):**
`cd ui/console && npm run lint && npx tsc --noEmit && npx vitest run && npm run build`

**Workflow per page:** map seams → add typed contracts → write tests TDD (red→green) → sweep `any` → run gates → update this doc → commit → `git-push-to-pr`. One PR per page.

---

## Scope

**In scope (in order):** Jobs → Companies (admin) → Dashboard → Insights → Settings → Account → Admin Users → Admin Audit.

**Out of scope (deferred):** Resume Library / Detail / Editor pages, Apply-Queue.

**Checkpoint:** after the **Jobs** PR, pause for user confirmation before running the loop across the remaining pages.

---

## Status table

| #   | Page              | Route              | Seams found              | Tests added                    | `any` removed | PR  | Status          |
| --- | ----------------- | ------------------ | ------------------------ | ------------------------------ | ------------- | --- | --------------- |
| 1   | Jobs              | `/jobs`            | nav: company→jobs (done) | jobsUrl builder + drawer guard | tbd           | —   | **In progress** |
| 2   | Companies (admin) | `/admin/companies` | tbd                      | tbd                            | tbd           | —   | Not started     |
| 3   | Dashboard         | `/dashboard`       | tbd                      | tbd                            | tbd           | —   | Not started     |
| 4   | Insights          | `/insights`        | tbd                      | tbd                            | tbd           | —   | Not started     |
| 5   | Settings          | `/settings`        | tbd                      | tbd                            | tbd           | —   | Not started     |
| 6   | Account           | `/account`         | tbd                      | tbd                            | tbd           | —   | Not started     |
| 7   | Admin Users       | `/admin/users`     | tbd                      | tbd                            | tbd           | —   | Not started     |
| 8   | Admin Audit       | `/admin/audit`     | tbd                      | tbd                            | tbd           | —   | Not started     |

Status values: `Not started` · `In progress` · `In review` (PR open) · `Done` (PR merged).

---

## Per-page detail

### 1. Jobs (`/jobs`) — In progress

**Already landed:**

- Typed `jobsUrl()` builder in `features/jobs/hooks/useJobUrlState.ts`, derived from the page's `nuqs` PARSERS; exported from the jobs barrel.
- Fixed nav seam: `CompanyDetailDrawer` "View N new jobs" now navigates via `jobsUrl({companies})` instead of the silently-dropped `location.state`.
- Tests: `jobsUrl` builder unit tests (3); `CompanyDetailDrawer` interaction guard test (asserts URL query carries the company + no `state` arg).

> RESUME HERE: complete the remaining Jobs-page seam audit —
>
> - **Nav:** mobile job route `/jobs/:id` uses `location.state.idx` for back-nav — verify it has a typed contract + test.
> - **Shared-state:** `useJobUrlState` ↔ `useJobs` filter shape round-trip.
> - **Cache:** `useJobActions` mutations (setStatus, dismiss, setSaved) — assert they invalidate the exact `jobQueryKeys.list/counts` keys that other views rely on.
> - **API contracts:** covered by Phase 0.5 zod (`features/jobs/api/jobs.api.ts`) — verify `normalizeJob` handles null `user_state` and all zod schemas are tested.
> - **Typing:** grep `features/jobs`, `pages/JobsPage.tsx` for `any`/`as any`; remove.
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

- **2026-05-28** — Scope set; fixture-based API-contract approach chosen. Plan + skill live at `Designs/seam-test-coverage-plan.md` and `.claude/skills/seam-test-coverage/`. Resumes/Apply-Queue deferred.
- **2026-05-29** — Architecture migration (PRs 1–10) complete. Boundary rules are now CI errors. `api/` moved into feature slices. Skill and plan re-applied on top of current main.
