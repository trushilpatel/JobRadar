# Companies Page — Production-Grade Hardening

**Date:** 2026-05-11

## What changed

### New files

- `frontend/src/constants/routes.ts` — App-wide route path constants (`ROUTES.JOBS`, `ROUTES.ADMIN.COMPANIES`, etc.). Single source of truth for all navigation paths.
- `frontend/src/features/companies/queryKeys.ts` — TanStack Query key factories for the companies feature (`companyQueryKeys`). All query keys are structured arrays, never raw strings.

### Modified files

- `frontend/src/features/companies/constants.ts` — Added `STORAGE_KEYS` (localStorage key constants), `EMPTY_MESSAGES` (table empty-state strings), and `AUTO_REFRESH_INTERVAL_MS`.
- `frontend/src/features/companies/hooks/useCompanies.ts` — Uses `STORAGE_KEYS.AUTO_REFRESH` and `companyQueryKeys`; is the single source of truth for auto-refresh state.
- `frontend/src/features/companies/hooks/useCompanyActions.ts` — Uses `companyQueryKeys`; gained `handleCompanyAdded` and `handleCompanyUpdated` actions so all post-mutation toast/invalidation logic lives in the hook, not in the page.
- `frontend/src/features/companies/hooks/usePipelineStats.ts` — Uses `companyQueryKeys.pipelineStats()`.
- `frontend/src/features/companies/components/CompaniesHeader.tsx` — Accepts `autoRefresh` and `onToggleAutoRefresh` as props; no longer owns its own localStorage state. Uses `companyQueryKeys` for manual refresh invalidation. Removed duplicate auto-refresh interval.
- `frontend/src/features/companies/components/CompanyDetailDrawer.tsx` — Replaced raw `useEffect`/`useState` fetch for scan stats with a `useQuery` call using `companyQueryKeys.companyScanStats`. Uses `ROUTES.JOBS` for navigation.
- `frontend/src/features/companies/index.ts` — Exports `companyQueryKeys` for external consumers.
- `frontend/src/pages/CompaniesPage.tsx` — Passes `autoRefresh`/`toggleAutoRefresh` from `useCompanies` to `CompaniesHeader`; uses `EMPTY_MESSAGES` constants; delegates post-add/update logic to `actions.handleCompanyAdded`/`handleCompanyUpdated`.

## Bugs fixed

- **Auto-refresh duplication** — `CompaniesHeader` and `useCompanies` both read/wrote the same localStorage key independently, meaning toggling one would not update the other. Now `useCompanies` owns the state and passes it down.
- **`CompanyDetailDrawer` scan stats** — was using a raw `useEffect` + `setState` fetch loop outside TanStack Query, losing cache benefits and requiring manual loading/error state. Now uses `useQuery`.

## Architecture rules enforced

- No raw strings as query keys — all keys go through `companyQueryKeys` factory functions.
- No raw strings as storage keys — all keys go through `STORAGE_KEYS` constants.
- No raw strings for route navigation — all paths come from `ROUTES`.
- Page components compose hooks; they do not contain mutation logic or toast calls directly.
