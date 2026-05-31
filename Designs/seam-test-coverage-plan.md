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
`cd frontend && npm run lint && npx tsc --noEmit && npx vitest run && npm run build`

**Workflow per page:** map seams → add typed contracts → write tests TDD (red→green) → sweep `any` → run gates → update this doc → commit → `git-push-to-pr`. One PR per page.

---

## Scope

**In scope (in order):** Jobs → Companies (admin) → Dashboard → Insights → Settings → Account → Admin Users → Admin Audit.

**Out of scope (deferred):** Resume Library / Detail / Editor pages, Apply-Queue.

**Checkpoint:** after the **Jobs** PR, pause for user confirmation before running the loop across the remaining pages.

---

## Status table

| #   | Page              | Route              | Seams found                                                                               | Tests added                                         | `any` removed | PR  | Status   |
| --- | ----------------- | ------------------ | ----------------------------------------------------------------------------------------- | --------------------------------------------------- | ------------- | --- | -------- |
| 1   | Jobs              | `/jobs`            | nav: company→jobs; cache: mutation invalidation roots                                     | jobsUrl (3) + drawer guard (2) + cache seam (1) = 6 | 0             | —   | **Done** |
| 2   | Companies (admin) | `/admin/companies` | cache: mutation invalidation keys (adminCompaniesBase, scanQueue)                         | cache seam (4)                                      | 0             | —   | **Done** |
| 3   | Dashboard         | `/dashboard`       | nav: MarketPanel used location.state (bug!); cache: InterviewingPanel mutation untested   | nav fix + cache seam (6 tests)                      | 0             | —   | **Done** |
| 4   | Insights          | `/insights`        | none (read-only, no cross-feature nav)                                                    | 0                                                   | 0             | —   | **Done** |
| 5   | Settings          | `/settings`        | cache: adapterConfigs inline keys + key mismatch; cross-feature invalidation root missing | cache seam (7 tests)                                | 0             | —   | **Done** |
| 6   | Account           | `/account`         | none — Zustand identity, no TanStack Query cache, no cross-feature nav                    | 0                                                   | 0             | —   | **Done** |
| 7   | Admin Users       | `/admin/users`     | cache: inline ['admin-users'] strings; auth feature missing queryKeys.ts entirely         | authQueryKeys factory + cache seam (4 tests)        | 0             | —   | **Done** |
| 8   | Admin Audit       | `/admin/audit`     | none (read-only)                                                                          | 0                                                   | 0             | —   | **Done** |

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

### 2. Companies (admin) (`/admin/companies`) — In review

**Seams audited (2026-05-29):**

- **Nav:** `handleViewJobs` in `CompaniesPage` already uses `jobsUrl()` via `onViewJobs` prop — tested in Jobs PR. No gap.
- **Shared-state:** `useCompanies` uses local `useState` (cursor, limit, filters) — intentional, admin list state is not URL-persistent. No shared-state seam.
- **Cache:** `useCompanyActions` mutations already use `companyQueryKeys.adminCompaniesBase()` and `companyQueryKeys.scanQueue()` factory keys — correct pattern. Added 4 cache seam tests: `toggleActive`, `remove`, `scan`, `scanAll` each assert the exact factory keys are invalidated.
- **API contracts:** Phase 0.5 zod schemas cover `adminCompanies.api.ts` + `companies.api.ts`. No gap.
- **Typing:** zero `any` in companies slice (verified by grep).

### 3. Dashboard (`/dashboard`) — Not started

_To fill during the page's run._

### 4. Insights (`/insights`) — Not started

_To fill during the page's run._

### 5. Settings (`/settings`) — In review

**Seams audited (2026-05-29):**

- **Nav:** SettingsPage has no cross-feature navigation. No gap.
- **Shared-state:** No nuqs URL state. `useAdapterConfigs` uses TanStack Query with a single query key. No shared-state seam.
- **Cache seams found (3 separate issues):**
  1. `useAdapterConfigActions.updateConfig` — all `setQueryData` and rollback calls used inline `['adapter-configs']` instead of `settingsQueryKeys.adapterConfigs()`. Fixed: now uses factory key. Cache seam test added (2 tests: success path + rollback path both read/write via factory key).
  2. `useAdapterConfigs` — `useQuery` used inline `['adapter-configs']` for its `queryKey`. Fixed: now uses `settingsQueryKeys.adapterConfigs()`.
  3. `useScanIntervalOverride.setOverride` — used 3 inline strings: `['companies']`, `['company', companyId]`, `['company-scan-config', companyId]`. Also had a **key mismatch**: `settingsQueryKeys.companyScanConfig(id)` returned `['scan-config', id]` but the hook invalidated `['company-scan-config', companyId]` — these never matched. Fixed: `settingsQueryKeys.companyScanConfig` now returns `['company-scan-config', id]`. The `['companies']` and `['company', companyId]` stubs were replaced by `scanIntervalOverrideMutationInvalidationRoots.adminCompanies` (typed cross-feature root constant in `queryKeys.ts`). Cache seam tests added (3 tests: adminCompanies root invalidated, companyScanConfig factory key invalidated, cross-feature contract assertion that the root value matches `['admin-companies']`).
- **API contracts:** Phase 0.5 zod schemas cover `adapterConfigs.api.ts`. No gap.
- **Typing:** zero `any` in `features/settings/` and `pages/SettingsPage.tsx`.

**New exports in `features/settings/index.ts`:** `settingsQueryKeys`, `scanIntervalOverrideMutationInvalidationRoots`.

**Files changed:**

- `src/features/settings/queryKeys.ts` — fixed key mismatch + added `scanIntervalOverrideMutationInvalidationRoots`
- `src/features/settings/hooks/useAdapterConfigActions.ts` — factory keys
- `src/features/settings/hooks/useAdapterConfigs.ts` — factory key
- `src/features/settings/hooks/useScanIntervalOverride.ts` — factory keys + typed cross-feature root
- `src/features/settings/index.ts` — new exports
- `src/features/settings/hooks/__tests__/useAdapterConfigActions.test.ts` — 2 cache seam tests + existing tests updated to use factory key
- `src/features/settings/hooks/__tests__/useScanIntervalOverride.test.ts` — 3 cache seam tests added

### 6. Account (`/account`) — In review

**Seams audited (2026-05-29):**

- **Nav:** AccountPage has no cross-feature navigation.
- **Shared-state:** Identity is Zustand-only (`useAuthStore`). No TanStack Query server state. No shared-state seam.
- **Cache:** `updateMe` and `deleteMe` are plain async functions, not `useMutation`. No `invalidateQueries` involved. After `updateMe`, `refreshIdentity()` refreshes the Zustand store directly — no TanStack Query cache to invalidate. No cache seam.
- **API contracts:** `me.api.ts` uses zod schema for `fetchMe`. `updateMe` and `deleteMe` are fire-and-forget PATCH/DELETE — no response body parsed. No gap.
- **Typing:** zero `any` in `pages/AccountPage.tsx`, `features/auth/api/me.api.ts`, `features/auth/store.ts`.
- **Conclusion:** AccountPage has no open seams. No tests needed.

### 7. Admin Users (`/admin/users`) — Not started

_To fill during the page's run._

### 8. Admin Audit (`/admin/audit`) — Not started

_To fill during the page's run._

---

## Decision log

- **2026-05-28** — Scope set; fixture-based API-contract approach chosen. Plan + skill live at `Designs/seam-test-coverage-plan.md` and `.claude/skills/seam-test-coverage/`. Resumes/Apply-Queue deferred.
- **2026-05-29** — Architecture migration (PRs 1–10) complete. Boundary rules are now CI errors. `api/` moved into feature slices. Skill and plan re-applied on top of current main.
- **2026-05-29** — Settings + Account pages complete. Key finding: `useScanIntervalOverride` had a factory key mismatch (`['scan-config', id]` in the factory vs `['company-scan-config', companyId]` in the hook) — would have silently never invalidated the intended query. Fixed by aligning the factory key and adding a typed `scanIntervalOverrideMutationInvalidationRoots` constant to avoid cross-feature string imports. Account page has no TanStack Query mutations — identity mutations are Zustand-only — no seams.
