# Nil Slice → JSON Null → Frontend Crash Fix

**Date:** 2026-05-11

## What changed

Go nil slices (`var results []T`) serialize to JSON `null`, not `[]`. When GORM finds no rows it leaves the slice nil. The backend was sending `{"data": null}` for empty insights queries. The frontend `unwrap` function in `api-client.ts` detected the `data` key and returned `null`; TanStack Query's `{ data = [] }` destructuring default only fires for `undefined`, not `null`, so components crashed calling `.length` on null.

## Why

The bug class is: **nil-slice JSON null ↔ TanStack Query default mismatch**. It affects any Go endpoint that returns a `var results []T` slice that GORM leaves nil when the query returns zero rows, combined with any frontend component that uses `{ data = [] }` as its null guard.

## Files touched

| File                                                             | Change                                                                                                                      |
| ---------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| `ui/console/src/lib/api-client.ts`                               | `unwrap`: map `obj.data === null` to `undefined` so TanStack Query defaults take effect                                     |
| `backend/service/insights/services/InsightsRepositoryService.go` | All six slice variables changed from `var results []T` to `make([]T, 0)` so they serialize to `[]` not `null` at the source |

## Affected endpoints

`/admin/insights/tech-stack`, `/admin/insights/company-activity`, `/admin/insights/sponsorship`, `/admin/insights/velocity`, `/admin/insights/role-categories`, `/admin/insights/location` (countries + states).
