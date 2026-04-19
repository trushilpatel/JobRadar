# Bulk Toggle All Companies

**Date:** 2026-04-12  
**Status:** Approved

## Summary

Add an "Activate All / Deactivate All" button to the companies listing page filter bar, backed by a single bulk API endpoint. The button is context-aware: it shows "Activate All" when any company is inactive, and "Deactivate All" when all companies are active.

## Backend

### Store

Add `SetAllCompaniesActive(active bool) error` to `CompanyWriter` in `backend/internal/store/company.go`:

```go
SetAllCompaniesActive(active bool) error
```

Implementation: single `UPDATE companies SET active = ?` — no per-row loop.

### Handler

Add `SetAllActive` method to `CompanyHandler` in `backend/internal/api/handler/companies.go`:

- Decodes `{ "active": bool }` from request body
- Calls `h.companies.SetAllCompaniesActive(active)`
- Returns `{ "active": active }` on success

### Route

Register in `backend/internal/api/router.go`:

```
PATCH /companies/active
```

**Must be registered before** `PATCH /companies/{id}` so chi does not interpret the literal path segment `active` as an `{id}` param.

## Frontend

### API

Add `setAllCompaniesActive(active: boolean): Promise<void>` to `frontend/src/api/companies.ts`:

```ts
PATCH /api/companies/active
Body: { active: boolean }
```

### UI — CompaniesPage.tsx

Add a smart toggle button in the filter bar, placed immediately to the left of the existing `RefreshCw` button.

**State derivation** (from `companies`, not `filtered`):
- If every company has `active === true` → button shows `<Power size={14} /> Deactivate All`
- Otherwise → button shows `<Power size={14} /> Activate All`

**Interaction:**
- On click: sets `bulkToggling = true`, calls `setAllCompaniesActive(next)`, then calls `load()`
- Button shows a spinner (replaces icon) and is disabled while `bulkToggling === true`
- On success: `toast.success('All companies activated')` or `toast.success('All companies deactivated')`
- On error: `toast.error('Failed to update companies')`

**Styling:** matches the existing refresh button — `flex items-center gap-1.5 p-2 text-slate-400 hover:text-white bg-white/5 border border-white/10 rounded-lg transition-colors text-sm`

## What Does Not Change

- Per-row toggle in `companies-columns.tsx` — unchanged
- No new migrations
- No changes to the scan queue, scheduler, or other handlers
