# ATS Service Slice

**Date:** 2026-06-04

## What was built

Replaced the `adapter_configs` table and scattered `AdapterConfig*` service files with a first-class `ats` domain slice and in-process cache.

## Migration (077)

`backend/migrations/077_ats_table.sql` — creates `ats` table (PK `name`, adds `display_name`, `base_url`, `is_active`, `is_admin_creatable`, `scan_interval_min`, `inter_company_delay_s`, `last_scheduled_at`). Migrates all 14 adapter rows from `adapter_configs`, re-points FKs on `company_scan_settings` and `company_scan_queue`, drops `adapter_configs`. **Deploy order: run after new binary is live.**

## Backend

`backend/service/ats/` — full decorator chain: `RepositoryService → ImplService → CacheService → ValidationService → RestService → RestRouter`. Cache uses `platform/cache.MemoryCache` (in-process, 5-min TTL); keys `"ats:list"` and `"ats:{name}"` invalidated on PATCH. Admin routes: `GET /admin/ats`, `GET /admin/ats/:name`, `PATCH /admin/ats/:name`.

`backend/service/scan/services/` — `GetEffectiveScanConfig` and `UpdateScanIntervalOverride` moved from deleted `AdapterConfig*` files onto the `ScanService` interface. SQL updated throughout to reference `ats` table (`name`, `scan_interval_min`). All `AdapterConfig*` service/test/model/dto files deleted.

`backend/service/company/services/CompanyAdminValidationService.go` — adapter_type validation now calls `AtsService.GetATS` live; `is_admin_creatable` check replaces the deleted `AdminUnsupportedAdapters` map.

`backend/apps/api/wire_gen.go` — ATS chain wired with `cache.NewMemoryCache()` and `5*time.Minute` TTL.

## Frontend

`frontend/src/features/ats/` — new slice with `api/ats.api.ts` (Zod-validated), `hooks/useAts.ts` (staleTime 10min), `hooks/useAtsActions.ts` (invalidates list + detail on mutation), `queryKeys.ts`, barrel export.

`frontend/src/features/settings/components/AtsTable.tsx` — replaces deleted `AdapterConfigsTable.tsx`; shows `display_name` + `name` subtitle; consumes `useAts` via props from `SettingsPage`.

`frontend/src/features/companies/components/AddCompanyModal.tsx` — ATS dropdown populated from `useAts()` filtered by `is_admin_creatable`; URL auto-fill uses `ats.base_url` prefix; empty `base_url` signals fixed-URL adapter.

`frontend/src/features/companies/components/CompanyDetailDrawer.tsx` — shows `display_name` via `atsEntry` prop.
