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

| #   | Page              | Route              | Seams found                                           | Tests added                                         | `any` removed | PR  | Status        |
| --- | ----------------- | ------------------ | ----------------------------------------------------- | --------------------------------------------------- | ------------- | --- | ------------- |
| 1   | Jobs              | `/jobs`            | nav: company→jobs; cache: mutation invalidation roots | jobsUrl (3) + drawer guard (2) + cache seam (1) = 6 | 0             | —   | **In review** |
| 2   | Companies (admin) | `/admin/companies` | tbd                                                   | tbd                                                 | tbd           | —   | Not started   |
| 3   | Dashboard         | `/dashboard`       | tbd                                                   | tbd                                                 | tbd           | —   | Not started   |
| 4   | Insights          | `/insights`        | tbd                                                   | tbd                                                 | tbd           | —   | Not started   |
| 5   | Settings          | `/settings`        | tbd                                                   | tbd                                                 | tbd           | —   | Not started   |
| 6   | Account           | `/account`         | tbd                                                   | tbd                                                 | tbd           | —   | Not started   |
| 7   | Admin Users       | `/admin/users`     | tbd                                                   | tbd                                                 | tbd           | —   | Not started   |
| 8   | Admin Audit       | `/admin/audit`     | tbd                                                   | tbd                                                 | tbd           | —   | Not started   |

Status values: `Not started` · `In progress` · `In review` (PR open) · `Done` (PR merged).

---

## Per-page detail

### 1. Jobs (`/jobs`) — In review

**Already landed:**

- Typed `jobsUrl()` builder in `features/jobs/hooks/useJobUrlState.ts`, derived from the page's `nuqs` PARSERS; exported from the jobs barrel.
- Fixed nav seam: `CompanyDetailDrawer` "View N new jobs" now navigates via `jobsUrl({companies})` instead of the silently-dropped `location.state`.
- Tests: `jobsUrl` builder unit tests (3); `CompanyDetailDrawer` interaction guard test (asserts URL query carries the company + no `state` arg).

**All seams closed (2026-05-29):**

- **Nav:** `CompanyDetailDrawer` → Jobs page uses `jobsUrl()` typed builder via `onViewJobs` prop at pages layer. Regression guard test asserts URL carries param + no `state` arg.
- **Cache:** `invalidateUserData` now uses `jobMutationInvalidationRoots` (typed constants in `jobs/queryKeys.ts`). Seam test asserts all three roots (`jobs`, `dashboard`, `insights`) are invalidated by factory key, not inline string.
- **Mobile route `location.state.idx`:** intentional same-page back-nav guard; already typed `{ idx?: number }`. No cross-feature seam — no additional test needed.
- **Shared-state:** `useJobUrlState` PARSERS are single source of truth; `useJobs` composes from them directly — no drift possible. Existing round-trip tests cover this.
- **API contracts:** Phase 0.5 zod schemas cover all `jobs.api.ts` boundaries; `normalizeJob` tested via fixture in `jobsApi.test.ts`.
- **Typing:** zero `any` in `features/jobs/` and `pages/JobsPage.tsx` (verified by grep).

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
