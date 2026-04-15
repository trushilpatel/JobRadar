# Companies Revamp — QA Acceptance Checklist (Phase 1)

Source of truth: `Designs/UI/companies-redesign-mockup.html`
Target structure: `features/companies/` + new shared primitives in `components/ui/`.

Each item is a single pass/fail assertion. Stubs for missing backend fields
(`tech_trend`, `new24h`, `open_jobs_total`, `recent_runs`) are acceptable if they
are visually present and annotated with `TODO(backend):` comments.

---

## 1. Pixel parity vs mockup

1.1 Page `<body>` (or root app container) background is `#0a0a10`.
1.2 Table `<thead>` has `position: sticky; top: 0` with background `#0d0d14` and bottom border `border-white/[0.07]`.
1.3 Table header text is `text-[11px] uppercase tracking-wider text-slate-500`.
1.4 Primary accent color is violet-600 (`bg-violet-600` / `rgba(124,58,237,*)`) — no indigo or purple-500.
1.5 Running status dot uses `pulse` keyframe animation (1.5s ease-in-out infinite) with blue-500 glow.
1.6 `scan-history` bar renders exactly 10 cells, each `width:6px; height:16px; border-radius:2px` with 2px gap.
1.7 History cell colors: ok=`bg-emerald-500/70`, err=`bg-red-500/80`, run=`bg-blue-500 animate-pulse`, idle=`bg-white/10`.
1.8 KPI card padding is `px-3.5 py-3` (12/14px), `border-radius: 10px`, border `#ffffff10`, background `#ffffff05`.
1.9 KPI value is `text-[22px] font-semibold leading-[1.1]`; KPI label is `text-[10px] uppercase tracking-[.08em] text-slate-500`.
1.10 Errors KPI card is clickable — hover border violet-600/35; active state border `rgba(239,68,68,.4)` with red-500/5 bg.
1.11 Segmented switch container: `bg-white/[0.03]`, `border-white/[0.08]`, `rounded-lg`, `padding: 2px`.
1.12 Segmented switch selected button variants: violet for "all", emerald for "active", red for "error", blue for "running", amber for "queued".
1.13 Segmented switch un-selected button: `text-slate-400` with `hover:text-slate-200`, no background.
1.14 Segment button internal count pill: `text-[10px] bg-black/25 rounded-full px-1.5 py-0.5`.
1.15 Multi-select adapter dropdown button matches `dropdown-btn` style — width/padding `px-2.5 py-1.5`, `text-[11px]`, violet-tinted when any selected.
1.16 Dropdown menu: `min-width:240px`, `bg-[#13131e]`, 10px radius, shadow `0 8px 24px rgba(0,0,0,.4)`.
1.17 Dropdown item selected check: 14px square with `bg-violet-600` and white check glyph.
1.18 Dropdown has inline "Add new adapter" input + violet button at bottom (border-top separator).
1.19 Drawer default width is 420px; panel background `#0d0d14`; left border `border-white/[0.07]`.
1.20 Drag handle is exactly `w-1` (4px), `cursor-col-resize`, idle bg `bg-white/[0.06]`, hover `bg-violet-500/50`.
1.21 Drawer tech-trend bar: `h-1.5` track `bg-white/[0.04]`, fill `bg-violet-500/70`, label column `w-20 truncate`, count column `w-8 text-right`.
1.22 Row hover shows action icons (scan/edit/delete) via `opacity 0 → 1` transition on `tr:hover .actions`.
1.23 Active toggle is `w-9 h-5` pill, violet-600 when on, `bg-white/15` when off; knob `w-4 h-4` translate-x-4 on.
1.24 Status pills include inline dot (7px) + label in `text-[11px]` with tinted `10%` bg and `20%` border.
1.25 Inline header shows `N tracked · {active} active · {errors} errors` where "errors" is red-400 and clickable.

## 2. Structure

2.1 `frontend/src/features/companies/` directory exists.
2.2 `features/companies/components/CompaniesHeader.tsx` exists.
2.3 `features/companies/components/CompaniesKpiStrip.tsx` exists.
2.4 `features/companies/components/CompaniesFilterBar.tsx` exists.
2.5 `features/companies/components/CompaniesTable.tsx` exists.
2.6 `features/companies/components/companies-columns.tsx` exists.
2.7 `features/companies/components/CompanyDetailDrawer.tsx` exists.
2.8 `features/companies/components/AddCompanyModal.tsx` exists (moved in from `components/companies/`).
2.9 `features/companies/hooks/useCompanies.ts` exists.
2.10 `features/companies/hooks/useCompanyActions.ts` exists.
2.11 `features/companies/index.ts` exports all public symbols (mirrors `features/jobs/index.ts` pattern).
2.12 New shared primitive `components/ui/kpi-card.tsx` exists.
2.13 New shared primitive `components/ui/segmented-switch.tsx` exists.
2.14 New shared primitive `components/ui/multi-select-dropdown.tsx` exists.
2.15 New shared primitive `components/ui/status-pill.tsx` exists.
2.16 New shared primitive `components/ui/scan-history-bar.tsx` exists.
2.17 Each primitive accepts generic props (no Company/Job-specific fields; e.g. `SegmentedSwitch` takes `options: { value, label, count?, color? }[]`).
2.18 `pages/CompaniesPage.tsx` is <= 80 lines and only wires hook outputs into feature components.
2.19 Orphan `pages/companies-columns.tsx` has been deleted.
2.20 Orphan `components/companies/ScanStatsDrawer.tsx` has been deleted (replaced by `CompanyDetailDrawer`).
2.21 Orphan `components/companies/AddCompanyModal.tsx` path is removed (or re-exported from new location) — no dead imports.
2.22 No business logic (fetch, mutate, debounce) lives in any file under `components/`.

## 3. Behavior

3.1 Clicking the Errors KPI card sets status filter to `error`.
3.2 Clicking the Errors KPI card again (now active) toggles status filter back to `none`.
3.3 Clicking the inline "N errors" text in the header toggles status filter the same way.
3.4 When status filter === `error`, the Errors KPI card renders its `.active` styling (red tint).
3.5 When status filter === `error`, the "Errors" segmented-switch button renders its `.on.red` styling and syncs with the card.
3.6 Segmented switches within a group are mutually exclusive (selecting one deselects the others).
3.7 Active-state switch options are exactly: `all` / `active` / `inactive`; counts reflect server totals.
3.8 Status switch options are exactly: `any` / `error` / `running` / `queued`.
3.9 Adapter multi-select supports search (filters list in place, case-insensitive, by substring).
3.10 Adapter multi-select supports inline "Add" — new adapter appears in list and is auto-selected.
3.11 Adapter multi-select closes on outside click.
3.12 Adapter multi-select selected items show a violet check icon.
3.13 Dropdown button label reads "All adapters" when empty, the adapter name when exactly one is selected, "N adapters" otherwise.
3.14 Clicking a table row opens the detail drawer for that company.
3.15 Clicking the Active toggle in a row calls `event.stopPropagation()` so the drawer does NOT open.
3.16 Clicking any icon in the row `.actions` cluster (scan/edit/delete) also stops propagation.
3.17 Drawer close button closes the drawer; ESC key also closes the drawer.
3.18 "Reset" button clears search, both segmented switches, and the adapter selection in one click.
3.19 "Activate all" button toggles every visible company to active (and flips to "Deactivate all" when all are already active).
3.20 "Scan now" action respects backend cooldown (button disabled or shows spinner while a request is in flight).
3.21 Drag handle on drawer edge resizes panel between ~280px and ~680px (persists during drag).
3.22 Drag handle hidden when drawer is closed.
3.23 Drawer shows Last-10-scans bar + success-rate text, Activity cards (new24h + open jobs total), Tech-trend list, Recent runs list.
3.24 Unavailable backend fields render visible skeleton/placeholder text with a `TODO(backend):` comment in JSX/code.

## 4. Wiring (Phase 2 gate)

4.1 `useCompanies` calls real API via `fetchCompanies` — no hardcoded arrays.
4.2 `useCompanies` loading state drives a skeleton/spinner in the table area.
4.3 `useCompanies` error state drives a visible error banner (not a silent `catch {}`).
4.4 Successful mutations (scan, delete, toggle, bulk-toggle, add, edit) fire `sonner` toast.
4.5 Failed mutations fire error toast with a meaningful message (not just "Error").
4.6 Toggling `active` is optimistic — UI updates immediately and rolls back on failure (preserved from current page).
4.7 Pagination controls are present, bound to `serverPagination` on `DataTable`, and preserve existing `page`/`limit` query behavior.
4.8 Filter changes reset to page 1 (preserved from current page).
4.9 Queue-status polling (`fetchQueue`) still runs on a 15s interval; company refresh on 30s interval.
4.10 `ScanStatsDrawer` API fetch logic is ported into `CompanyDetailDrawer` (or a child hook) — recent-runs wires to real data, not mocks.

## 5. Accessibility

5.1 All icon-only buttons have a `title` AND either `aria-label` or visually-hidden text.
5.2 Segmented-switch buttons are focusable and navigable via Tab / arrow keys.
5.3 Multi-select dropdown opens on Enter/Space when button is focused.
5.4 Multi-select dropdown items are navigable via arrow keys; Space toggles; Escape closes.
5.5 Drawer close button has `aria-label="Close"` and is in the tab order.
5.6 Pressing ESC while drawer is focused closes the drawer.
5.7 Every interactive element has a visible `focus-visible` ring (violet-500).
5.8 Table rows have `role="button"` (or are `<tr>` with `tabIndex=0` + keydown Enter/Space handler) so keyboard users can open the drawer.

## 6. Code quality

6.1 No `any` types anywhere in `features/companies/`.
6.2 All domain types (`Company`, `ScanRun`, `ScanStats`, `QueueEntry`) imported from `@/types` — no locally redeclared shapes.
6.3 All class-name concatenation goes through `cn()` from `@/lib/utils`.
6.4 Repeated Tailwind class strings (pill base, dropdown item, seg button) are extracted via `cva` or a shared helper — no copy-paste.
6.5 Imports ordered: external packages, then `@/` absolute, then relative.
6.6 No `console.log` / `console.error` left in shipped code (use toast or error boundary).
6.7 Each component file <= 250 lines; drawer split into sub-sections if it grows past that.
6.8 No default exports for components (matches existing `features/jobs/` convention).
6.9 Event handlers that stop propagation do so explicitly via `event.stopPropagation()` (not by returning false).
6.10 Primitives live under `components/ui/` and have no domain imports (`@/types`, `@/api`, `@/services`).
