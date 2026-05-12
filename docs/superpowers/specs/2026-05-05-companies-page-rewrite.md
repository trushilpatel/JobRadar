# Companies Page — Full Production Rewrite

**Date:** 2026-05-05
**Superseded (partially) by:** [`2026-05-11-companies-page-production-grade.md`](2026-05-11-companies-page-production-grade.md)

> The implementation described below was shipped on 2026-05-05. On 2026-05-11 the following details were superseded:
>
> - **Query key format** — The explicit array `['admin-companies', debouncedSearch, ...]` is now encapsulated behind `companyQueryKeys.adminCompanies(filters, page, limit)` from `features/companies/queryKeys.ts`. The string `'admin-companies'` is no longer used directly anywhere.
> - **`CompaniesHeader` auto-refresh** — The header no longer owns its own `setInterval`/localStorage state. `useCompanies` is the single source of truth; the header receives `autoRefresh` and `onToggleAutoRefresh` as props. The query `refetchInterval` is 10 s (unchanged); the header's duplicate 30 s interval is removed.
> - **`CompanyDetailDrawer` scan stats** — The raw `useEffect`/`setState` fetch was replaced with a `useQuery` call using `companyQueryKeys.companyScanStats(id)`.
> - **New constants** — `STORAGE_KEYS`, `EMPTY_MESSAGES` added to `features/companies/constants.ts`; `ROUTES` added to `constants/routes.ts`.

## Problem

The companies page has three distinct failure modes:

1. **Status filter is client-side.** When error/running/queued is selected, `useCompanies` sets `limit=9999`, fetches every company, and filters in the browser. Catastrophic at scale and wrong in principle.

2. **Error filter produces wrong results.** The backend `CompanyAdminView` never includes scan log data. The frontend derives `last_scan_status` from `consecutive_errors` in `company_scan_settings` — an approximation, not the real last-scan outcome. The error filter silently returns incorrect results.

3. **Scan history bars are empty.** The list endpoint never fetches `company_scan_log` rows. The 10-slot history bar in the table renders with no data.

Secondary issues: a separate `/companies/queue` polling call (15s) merged client-side for running/queued status; mixed client/server filtering logic in `useCompanies`; filter state that doesn't reset cleanly.

## Scope

Full rewrite — backend list query, backend filter struct, frontend hook, frontend filter bar. Components that don't touch filtering or data shape (AddCompanyModal, CompanyDetailDrawer, PipelineStrip, CompanyDetailDrawer) are unchanged.

Offset-based pagination is kept. Companies is an admin page with a small, bounded dataset. Cursor pagination adds complexity with no benefit here.

---

## Backend

### Files changed

| File                                                       | Change                                          |
| ---------------------------------------------------------- | ----------------------------------------------- |
| `backend/service/company/models/CompanyAdminFilter.go`     | Add `ScanStatus string` field                   |
| `backend/service/company/models/CompanyAdminView.go`       | Add `QueueStatus *string`, `ScanHistory []bool` |
| `backend/service/company/CompanyAdminRepositoryService.go` | Update `queryCompanies`, update `List`          |
| `backend/service/company/CompanyAdminRestService.go`       | Parse `scan_status` query param                 |

### `CompanyAdminFilter`

Add one field:

```
ScanStatus string   // "all" | "ok" | "error" | "queued" | "running"; empty = all
```

### `CompanyAdminView`

Add two fields:

```
QueueStatus  *string  // "queued" | "running" | nil (not in queue)
ScanHistory  []bool   // last ≤10 scans oldest→newest; true=success, false=error
```

### `queryCompanies` — updated query

Add a `LEFT JOIN company_scan_queue q ON q.company_id = c.id AND q.status IN ('queued','running')` to the existing query. Select `q.status AS queue_status` in the SELECT clause. The join is safe because the unique partial index on `company_scan_queue` prevents more than one active row per company.

`scan_status` WHERE clause (mutually exclusive, single-value):

| Value       | Clause                                                    |
| ----------- | --------------------------------------------------------- |
| `queued`    | `q.status = 'queued'`                                     |
| `running`   | `q.status = 'running'`                                    |
| `error`     | `s.consecutive_errors > 0`                                |
| `ok`        | `s.consecutive_errors = 0 AND s.last_scan_at IS NOT NULL` |
| empty/`all` | no additional clause                                      |

Populate `QueueStatus` on each returned row from the joined `queue_status` column.

### `List` — scan history batch

After `queryCompanies` returns, extract all company IDs and call the existing `GetScanHistories(ctx, companyIDs)` on the `ScanLogStore`. Attach the returned `[]bool` to each `CompanyAdminView`. This is a single extra query, already optimised with a PARTITION BY window function.

### Handler

Parse `scan_status` from query string and pass into `CompanyAdminFilter.ScanStatus`.

---

## Frontend

### Files changed

| File                                                                  | Change                                                                   |
| --------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| `ui/console/src/types/index.ts`                                       | Add `queue_status?: string` to `Company` type                            |
| `ui/console/src/api/adminCompanies.ts`                                | Add `scanStatus` to `AdminCompanyFilters`, wire to query param           |
| `ui/console/src/features/companies/hooks/useCompanies.ts`             | Full rewrite — remove all client-side filter logic                       |
| `ui/console/src/features/companies/hooks/normalizeAdminCompany.ts`    | Use `queue_status` from backend; derive status from `consecutive_errors` |
| `ui/console/src/features/companies/components/CompaniesFilterBar.tsx` | Rewrite to match `JobFilters` pattern                                    |
| `ui/console/src/features/companies/components/CompaniesHeader.tsx`    | Remove queue polling trigger; keep auto-refresh toggle                   |
| `ui/console/src/features/companies/constants.ts`                      | Add `SCAN_STATUS_FILTER` enum matching backend values                    |

### `useCompanies` — rewritten state

Remove: `needsClientFilter`, `limit=9999`, separate `/companies/queue` query, `queueMap`, all client-side filter application.

State:

```
search            string         raw input value
debouncedSearch   string         300ms debounced
adapters          string[]       multi-select adapter types
activeFilter      'all'|'active'|'inactive'
scanStatus        'all'|'ok'|'error'|'queued'|'running'
sorting           SortingState
page              number         1-indexed
limit             number
```

On any filter or sort change: reset `page` to 1. All five filter values are sent to the backend as query params. No client-side filtering.

Query key: `['admin-companies', debouncedSearch, adapters, activeFilter, scanStatus, sortBy, sortDir, page, limit]`

Invalidation on mutations: unchanged — `useCompanyActions` already invalidates `'admin-companies'`.

Remove the separate `'scan-queue'` query from this hook entirely. Queue status arrives in each company row from the main list response.

### `normalizeAdminCompany`

- `last_scan_status`: use `queue_status` if present (`'running'` / `'queued'`), else derive from `consecutive_errors > 0` → `'error'`, else `last_scan_at` present → `'ok'`, else `'pending'`
- `scan_history`: pass through `ScanHistory []bool` from the view directly

### `CompaniesFilterBar`

Rewrite to match `JobFilters.tsx` structure:

- **Left:** search input with Search icon
- **Center:** MultiSelectDropdown (adapter types) · SegmentedSwitch (all/active/inactive) · FilterPills for scan status (ok / error / queued / running — single-select, click again to deselect)
- **Right:** "Add Company" button
- **Reset button:** visible only when any filter is non-default; resets all to default

### `CompaniesHeader`

Remove: the separate queue-poll invalidation (no longer needed — queue status is in the main list).
Keep: auto-refresh toggle (30s refetchInterval on the main query), manual refresh button, PipelineStrip, error count badge.

---

## Data flow after rewrite

```
GET /admin/companies?scan_status=error&search=…&active=true&page=1&limit=20
  → queryCompanies (JOIN company_scan_queue) → companies[] + queue_status per row
  → GetScanHistories(companyIDs) → scan_history per row
  → adminListResponse { data, total, total_active, page, limit }

useCompanies (all server params, no client filter)
  → normalizeAdminCompany (queue_status + consecutive_errors → last_scan_status)
  → CompaniesTable / CompaniesFilterBar
```

---

## What this does not change

- Offset-based pagination (intentional — small admin dataset)
- AddCompanyModal, CompanyDetailDrawer, PipelineStrip (no data shape change)
- Scan queue polling in `CompaniesHeader` for real-time pipeline strip stats (separate endpoint, unchanged)
- All mutation endpoints and `useCompanyActions` hook
- Backend company CRUD endpoints

---

## Testing

- Backend: unit test `queryCompanies` with each `scan_status` value (ok/error/queued/running/all); verify scan_history attached
- Frontend: lint must pass (`npm run lint`); verify each filter combination updates the query key and resets page to 1; verify scan history bars render with data
