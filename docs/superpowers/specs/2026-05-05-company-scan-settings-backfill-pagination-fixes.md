# Company Scan Settings Backfill + Pagination & UI Fixes

**Date:** 2026-05-05

## Overview

Three independent fixes shipped in this session: restoring scan settings for all seeded companies (workers were scanning nothing), fixing the broken company URL link icon, and unifying pagination across the jobs and companies pages.

---

## 1. Company Scan Settings Backfill

### Root Cause

Migration 044 (`044_phase2_multiuser_data_model.sql`) dropped `adapter_type` from `companies` and created `company_scan_settings` without backfilling it. The 583 companies seeded by migrations 038–040 all lost their adapter configuration. Workers check `company_scan_settings` before scanning; with no rows, every company was silently skipped.

### Fix

`backend/migrations/047_backfill_company_scan_settings.sql` — derives `adapter_type` from `companies.website` URL patterns (`greenhouse.io`, `jobs.lever.co`, `jobs.ashbyhq.com`, and 7 custom adapters). Uses `ON CONFLICT DO NOTHING` so manually-configured settings are never overwritten. Result: 549 rows inserted (259 greenhouse, 240 ashby, 43 lever, 7 custom adapters).

---

## 2. Company URL Link Icon Fix

### Root Cause

Migration 044 renamed `companies.url` → `companies.website`. The frontend `AdminCompany` interface still declared the field as `url`, so every company's URL was `undefined` after normalization — the link icon rendered but had no `href`.

### Fix

- `ui/console/src/api/adminCompanies.ts` — `AdminCompany.url` → `website`; `createAdminCompany` and `updateAdminCompany` payloads send `website`
- `ui/console/src/features/companies/hooks/normalizeAdminCompany.ts` — reads `c.website` to populate `Company.url`
- `ui/console/src/features/companies/components/AddCompanyModal.tsx` — form state, all bindings, and submit payloads updated from `url` → `website`
- `ui/console/src/features/companies/hooks/__tests__/normalizeAdminCompany.test.ts` — fixture updated

---

## 3. Pagination Unification

### Changes

**`ui/console/src/components/ui/pagination.tsx`** — layout changed from `justify-between` (showing label left, controls center/right) to fully right-aligned: showing label, page numbers, and rows selector all grouped on the right.

**`ui/console/src/features/companies/components/CompaniesTable.tsx`** — pagination moved out of `DataTable`'s `overflow-auto` scroll area into a `shrink-0` sticky footer (same layout as `JobsTable`). `DataTable` is now called without `serverPagination`.

**`ui/console/src/features/jobs/components/JobsTable.tsx`** — custom minimal footer replaced with the full `Pagination` component. Accepts a new `total` prop (from `counts[tab]`). Cursor-based prev/next is mapped to `onPageChange` direction detection; page number buttons follow the same cursor chain.

**`ui/console/src/pages/JobsPage.tsx`** — passes `total={jobs.counts[jobs.tab] ?? 0}` and `onPageChange` to `JobsTable`.

### Lint Fixes

- `useJobs.ts` — `useEffect(() => setCursors(['']))` replaced with render-time state adjustment (`prevFilters` ref pattern), eliminating the `react-hooks/set-state-in-effect` error.
- `CompaniesTable.tsx` — `eslint-disable-next-line` comment moved from before the `useMemo` call to before the deps array where the violation actually fires.
