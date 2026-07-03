# company/ — Company catalog: dual public/admin surfaces with shared scan-settings repository

## Dual chain architecture

This slice exposes **two independent decorator chains** against **two interfaces**, wired by two separate `ProviderSet`s in `wire.go`:

```
Public  (read-only, /api/companies):
  RepositoryService → ImplService → CacheService → ValidationService → RestService → RestRouter

Admin   (full CRUD, /admin/companies):
  AdminRepositoryService → AdminImplService → AdminValidationService → AdminRestService → AdminRestRouter

Shared:
  ScanSettingsRepositoryService  (implements ScanSettingsRepository — used by both chains and background workers)
```

Both chains share the same `*cache.Cache` instance. The admin `ImplService` invalidates `company:public:{id}` after every mutation so public readers never see stale data.

## Interfaces

| Interface                | Declared in         | Purpose                                                                                                                                   |
| ------------------------ | ------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `CompanyService`         | `CompanyService.go` | Public read-only contract (`ListPublic`, `GetPublic`)                                                                                     |
| `CompanyAdminService`    | `CompanyService.go` | Admin CRUD + worker reads (`List`, `Get`, `Create`, `UpdatePublic`, `UpdateScanSettings`, `Delete`, `SetAllActive`, `GetByIDs`, `GetAll`) |
| `ScanSettingsRepository` | `CompanyService.go` | Single point of access to `company_scan_settings`; consumed by the admin chain and background workers                                     |
| `ScanHistoryReader`      | `CompanyService.go` | Narrow cross-domain read of scan history; implemented by `scan.ScanLogStore`                                                              |

## Key invariants

- `ListPublic` / `GetPublic` return `CompanyPublicView` — no `adapter_type`, no `adapter_config`, no scan timestamps. **Never** add operational fields to the public view.
- `CompanyAdminService.List` returns `CompanyAdminListResult` (containing `CompanyAdminListView`) — adapter credentials are **still omitted** from list responses. Only `CompanyAdminService.Get` returns the full `CompanyAdminView` with `adapter_config`.
- `ScanSettingsRepository` rejects an empty `companyID` with `ErrInternal.WithMessage("company id required")` on every method that takes a `companyID` — a forgotten ctx scoping must never silently match all rows. `CountAutoPaused` takes no `companyID` and is exempt by design (it aggregates across all rows).
- `Create` inserts a `companies` row plus an optional `company_scan_settings` row in a single transaction. When `CompanyScanSettingsInput.IsEmpty()`, no settings row is written and the company starts un-onboarded for scanning.
- `DELETE` on `companies` cascades to `company_scan_settings` via the DB `ON DELETE CASCADE` constraint — no explicit settings delete in application code.

## Non-canonical files

| File                                               | Purpose                                                                                                        |
| -------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `services/CompanyScanSettingsRepositoryService.go` | Standalone terminal repository for `company_scan_settings`; no `BaseService`, returns `ScanSettingsRepository` |
| `services/CompanyAdminPgErrors_internal_test.go`   | Internal package test for Postgres error classification helpers in the admin repo                              |
