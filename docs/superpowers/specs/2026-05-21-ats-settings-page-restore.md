# ATS Settings Page Restoration + Auto-Discovery

**Date:** 2026-05-21
**Branch:** feat/ats-settings-page-restore
**Status:** Approved

---

## Problem

The `/settings` page (ATS adapter configuration) was accidentally dropped from the sidebar nav during the multi-user refactor (`d00b4af`). The page, route, and all components still exist and work — but there is no way to reach the page from the UI. Additionally:

- The route is not admin-gated despite controlling infrastructure-level config
- The `ADAPTER_COLORS` map in `AdapterConfigsTable` is hardcoded — every new adapter requires a manual frontend code change
- No queryKeys factory (inline string `['adapter-configs']` spread across files)
- No empty state when the DB returns zero rows

---

## Scope

Frontend-only. No backend changes, no DB migrations, no API changes. The backend already returns adapter configs dynamically from the DB; this work makes the frontend fully leverage that.

---

## Design

### 1. Route Guard — `ui/console/src/App.tsx`

Move the `/settings` route inside the existing `<Route element={<RequireAdmin />}>` block. Adapter config is infrastructure-level; it belongs with Companies, Users, and Audit Log.

**Before:**

```
<Route path="/settings" element={<SettingsPage />} />
<Route element={<RequireAdmin />}>
  <Route path="/admin/companies" ... />
  ...
</Route>
```

**After:**

```
<Route element={<RequireAdmin />}>
  <Route path="/settings" element={<SettingsPage />} />
  <Route path="/admin/companies" ... />
  ...
</Route>
```

### 2. Sidebar Nav — `ui/console/src/components/Sidebar.tsx`

Add a Settings entry to `adminNav`:

```ts
{ to: '/settings', label: 'Settings', icon: <Settings size={16} /> }
```

Import `Settings` from lucide-react. The item appears in the Admin section for admins only, consistent with all other admin nav entries.

### 3. Auto-Color — `ui/console/src/lib/adapterColor.ts`

New pure utility module. Exports:

```ts
function getAdapterDotColor(adapterType: string): string
```

Algorithm:

1. Compute a djb2-style hash of the `adapterType` string
2. `index = Math.abs(hash) % PALETTE.length`
3. Return `PALETTE[index]` — a Tailwind `bg-*` color class string

Palette (12 entries, reuses the current set of adapter colors):
`bg-emerald-400`, `bg-blue-400`, `bg-violet-400`, `bg-red-400`, `bg-slate-300`,
`bg-orange-400`, `bg-indigo-400`, `bg-amber-400`, `bg-sky-400`, `bg-rose-400`,
`bg-teal-400`, `bg-cyan-400`

**Properties:**

- Deterministic: same input always returns the same color class
- No code change needed when a new adapter is added to the DB
- Testable in isolation as a pure function

Remove the hardcoded `ADAPTER_COLORS` constant from `AdapterConfigsTable.tsx`. Replace usage with `getAdapterDotColor(cfg.adapter_type)`.

### 4. QueryKeys Factory — `ui/console/src/features/settings/queryKeys.ts`

```ts
export const settingsQueryKeys = {
  adapterConfigs: () => ['adapter-configs'] as const,
}
```

Update `useAdapterConfigs` and `useAdapterConfigActions` to import from this factory. Remove all inline `['adapter-configs']` string literals from both hooks.

Export `settingsQueryKeys` from `src/features/settings/index.ts`.

### 5. Empty State — `AdapterConfigsTable.tsx`

When `rows.length === 0` (after loading succeeds), render an empty state inside the table body:

> **No adapter configurations found.**
> Adapters are seeded by database migrations.

Single row spanning all columns, centered, muted text. Consistent with the empty-state pattern used elsewhere in the codebase.

---

## Files Changed

| File                                                                  | Change                                                             |
| --------------------------------------------------------------------- | ------------------------------------------------------------------ |
| `ui/console/src/App.tsx`                                              | Move `/settings` under `RequireAdmin`                              |
| `ui/console/src/components/Sidebar.tsx`                               | Add Settings to `adminNav`                                         |
| `ui/console/src/lib/adapterColor.ts`                                  | New — deterministic color utility                                  |
| `ui/console/src/features/settings/queryKeys.ts`                       | New — queryKeys factory                                            |
| `ui/console/src/features/settings/components/AdapterConfigsTable.tsx` | Remove `ADAPTER_COLORS`, use `getAdapterDotColor`, add empty state |
| `ui/console/src/features/settings/hooks/useAdapterConfigs.ts`         | Use queryKeys factory                                              |
| `ui/console/src/features/settings/hooks/useAdapterConfigActions.ts`   | Use queryKeys factory                                              |
| `ui/console/src/features/settings/index.ts`                           | Export `settingsQueryKeys`                                         |

---

## Tests

### New

| File                                                           | What it covers                                                                                                                                                                                                                    |
| -------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `src/lib/adapterColor.test.ts`                                 | Determinism (same input → same output), full palette coverage (all 12 slots reachable), empty string input, symbols/spaces, non-ASCII                                                                                             |
| `src/features/settings/__tests__/AdapterConfigsTable.test.tsx` | Loading skeleton (5 rows), populated state (rows rendered, colors applied), empty state message, error state + retry button triggers refetch, confirmation modal: open on save, cancel restores state, confirm calls updateConfig |

### Updated

| File                                                                    | What changes                                                      |
| ----------------------------------------------------------------------- | ----------------------------------------------------------------- |
| `src/features/settings/__tests__/useAdapterConfigs.test.ts`             | Use `settingsQueryKeys.adapterConfigs()` instead of inline string |
| `src/features/settings/hooks/__tests__/useAdapterConfigActions.test.ts` | Use `settingsQueryKeys.adapterConfigs()` instead of inline string |

---

## Acceptance Criteria

- [ ] Admins see a Settings entry in the sidebar under the Admin section
- [ ] Non-admins are redirected away from `/settings` (same behavior as `/admin/companies`)
- [ ] All registered adapters display in the table with a colored dot (no manual color wiring)
- [ ] A new adapter added to the DB appears in the table on next load with a consistent dot color, zero frontend code changes required
- [ ] Empty state renders when DB returns no rows
- [ ] Loading skeleton renders while query is pending
- [ ] Error state renders with a retry button when the API call fails
- [ ] Confirmation modal works: cancel reverts, confirm saves and shows toast
- [ ] `npm run lint` passes with zero errors
- [ ] `npx tsc --noEmit` passes with zero errors
- [ ] All new and updated tests pass

---

## Out of Scope

- Backend changes, DB migrations, API shape changes
- Adding a read-only mode for non-admin users
- Serving colors from the backend
- Changes to `AdapterConfigCell` or `ScanIntervalOverrideModal`
- Changes to `CompanyDetailDrawer` scan interval override section
