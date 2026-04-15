# Jobs UI Audit — Drift vs Mockup + Primitive Promotion (Phase 1)

Mockup: `Designs/UI/jobs-listing.html`
Code: `frontend/src/features/jobs/` + `frontend/src/pages/JobsPage.tsx`

---

## 1. Drift vs mockup

### 1.1 Header — `pages/JobsPage.tsx:60-74`
- Mockup uses inline header: `flex items-center justify-between px-6 py-3` with `h1 text-lg font-semibold`, side-by-side with a subtitle `{N} jobs · {X} selected filters` (violet accent on the filter count).
- Current code uses `py-4`, `text-xl font-semibold`, and stacks a `<p>` below (`{N} jobs`). No "selected filters" summary chip. **Drift: size + layout + missing filter-count.**

### 1.2 Status tabs — `features/jobs/components/JobStatusTabs.tsx`
- Mockup has 5 tabs: New / Saved / Applied / **Interviewing / Offer** with always-visible count pills.
- Current code has only 3 tabs (`new`, `saved`, `applied`) and only shows a count on the *active* tab (`tab === t.key && total > 0`).
- Mockup tab styling uses `.tab` class (`padding:8px 14px; text-[13px]`), border-bottom `2px` violet-500 when active, light violet-5% bg.
- Current code uses `px-4 py-2 text-sm rounded-t-lg` — different shape (rounded top vs flat). **Drift: missing tabs, always-on counts, rounding.**

### 1.3 Filter bar — `features/jobs/components/JobFilters.tsx`
- Mockup filter bar has a prominent search input **inside** the filter bar (`w-48`, with search icon). Current code has no search input in the filter bar at all — search only exists on CompaniesPage, not JobsPage. **Drift: missing search input.**
- Mockup groups are separated by a vertical `group-sep` divider (`w-px h-[22px] bg-white/[0.08]`) — current `<Divider />` is `h-4` (16px). **Drift: divider height.**
- Mockup Experience pills use `on` (violet) vs `off` plain style; current matches ✓.
- Mockup Date-posted options are `Any / 7d / 30d`; current code has `all / 30d / 7d` (order differs; "all" vs "Any" label). **Drift: order + label.**
- Mockup has Location group: `Remote` (emerald) + `On-site` (off). Current has only `Remote`, no `On-site` pill. **Drift: missing On-site.**
- Current code ships extra filter pills not in the mockup: `Sponsorship`, `No Sponsorship`, `No Citizenship Req.`, `Citizenship Req.`, `Full-time`. Mockup has none of these. **Drift: extra filters (likely intentional but undocumented in mockup).**
- Mockup Reset is a plain text link `text-[11px] text-slate-500`; current code has no Reset button in the filter bar (despite `resetFilters` existing in the hook). **Drift: no Reset in UI.**

### 1.4 Table header — `features/jobs/components/jobs-columns.tsx` + `components/ui/data-table.tsx`
- Mockup header: `text-[11px] uppercase tracking-wider text-slate-500` sticky on `#0d0d14`, padding `py-2`. Column list: checkbox / Company / Title / Location / Salary / Added. Current columns match ✓ (order + content). Sticky styling on `thead` must be verified inside the shared `DataTable` — not visible from `JobsTable.tsx` alone. **Action: verify `thead.sticky top-0 bg-[#0d0d14]` is applied in the shared `DataTable`.**
- Mockup row padding is `px-2 py-3`; current is delegated to `DataTable`. Worth spot-checking visually.

### 1.5 Row cells — `jobs-columns.tsx:70-134`
- Mockup company cell: 28px (`w-7 h-7`) rounded-md avatar with hash color + first letter. Current uses `<CompanyAvatar size="sm">` — verify size prop maps to `w-7 h-7`.
- Mockup location remote pill includes an emoji/SVG globe; current uses `lucide-react Wifi` icon. **Drift: icon choice** (Wifi != Remote glyph semantically). Consider `Globe` icon instead.
- Mockup salary color is `text-amber-300`. Current matches ✓.
- Mockup "Added" is `text-slate-500`. Current matches ✓.

### 1.6 Detail panel — `features/jobs/components/JobDetailPanel.tsx`
- Mockup panel width is `w-[440px]`. Current initial state is 440 ✓ (`JobsPage.tsx:17`).
- Mockup close button is absolutely positioned top-right alongside a Dismiss button at `absolute top-3 right-3`. Current code places close in a `flex justify-between` alongside company info (`JobDetailPanel.tsx:23-38`). **Drift: layout.**
- Mockup puts Dismiss in the **top-right absolute cluster** next to close. Current places Dismiss at `ml-auto` within the action-button row (`JobDetailPanel.tsx:74-82`). **Drift: placement.**
- Mockup action row is: Open / Save / Applied — three compact buttons (`px-2.5 py-1.5`). Current matches roughly but also conditionally hides Save once saved (shows `Saved` state). Reasonable enhancement, not a drift.
- Mockup meta row has emoji prefix badges (globe Remote, salary, full-time, 3–5 yrs). Current uses lucide icons. Stylistic — low priority.
- Mockup body section headings are `text-[10px] font-medium text-slate-500 uppercase tracking-wider`. Current code uses `text-xs` (12px) at `JobDetailPanel.tsx:143, 150, 161, 174`. **Drift: font size 12px vs 10px.**

### 1.7 Drag handle + panel container — `pages/JobsPage.tsx:152-160`
- Mockup: `w-1 bg-white/[0.06] hover:bg-violet-500/50`. Current matches ✓.

### 1.8 Pagination
- Mockup has **no** pagination controls visible (it is a client demo). Current has server pagination — this is correct; preserve it.

### 1.9 Bulk action bar — `pages/JobsPage.tsx:105-123`
- Mockup does not show a bulk action bar in the static example. The current bulk bar (appears when rows are selected) is a reasonable addition and can stay. No drift.

---

## 2. Primitive promotion — Jobs code that should move to `components/ui/`

### 2.1 `kpi-card`
- Jobs page has **no** KPI strip today; nothing to replace. `DashboardPage.tsx` likely has inline KPI cards — out of scope for this audit but worth checking later.

### 2.2 `segmented-switch`
- Jobs currently has **no** segmented switch. The closest analog is `JobStatusTabs` (`features/jobs/components/JobStatusTabs.tsx:1-41`) but that is a tab-strip, not a seg-switch. Keep separate.
- **Action (Phase 2):** consider whether `JobStatusTabs` should instead be a primitive `Tabs` component — but that is separate from `SegmentedSwitch`. Do not merge.

### 2.3 `multi-select-dropdown`
- `features/jobs/components/JobFilters.tsx:14-108` defines `CompanyMultiSelect` inline — a full-featured dropdown with search, selection, checkbox visuals, outside-click, and clear-selection footer.
- This **should be replaced** with `components/ui/multi-select-dropdown.tsx` once the shared primitive exists.
- The shared primitive must support: (a) search, (b) add-new (Companies has it, Jobs does not — but primitive should accept optional `onAdd?` prop), (c) label fn (`'All X' / single / 'N X'`), (d) check-glyph styling.
- **File:line:** `features/jobs/components/JobFilters.tsx:14-108` (whole `CompanyMultiSelect` function, ~95 lines) — delete once shared primitive lands.

### 2.4 `status-pill`
- Jobs uses **inline** status badges throughout:
  - `jobs-columns.tsx:99-109` — Remote/Location pill (inline `<span>` with emerald bg).
  - `jobs-columns.tsx:118-123` — Salary pill (amber).
  - `JobDetailPanel.tsx:90-114` — 5 separate inline badge spans (Location, Remote, Salary, Employ type, Experience).
  - `JobDetailPanel.tsx:119-136` — Sponsorship/Citizenship/Security pills.
- Also: `components/ui/badge.tsx` already exists with `cva` variants (success/error/warning/info/violet/ghost/mono) and covers most of these cases. **Current `Badge` component is underused.**
- **Recommendation:** Companies' new `StatusPill` primitive should extend (or replace) the existing `Badge` — do not create a parallel primitive. If `StatusPill` needs a leading dot (per Companies mockup), add a `dot?: 'ok' | 'err' | 'warn' | 'idle' | 'run'` prop to `Badge`.
- **File:line:** `jobs-columns.tsx:99-109`, `JobDetailPanel.tsx:89-137` — refactor to use `Badge` once dot prop exists.

### 2.5 `scan-history-bar`
- No Jobs equivalent. Nothing to promote from Jobs.

### 2.6 Other inline patterns worth promoting
- **Drag handle:** `JobsPage.tsx:153-156` and `CompaniesPage.tsx:296-299` are verbatim duplicates. Extract as `components/ui/resize-handle.tsx` with `{ width, min, max, onResize }` props.
- **Filter pill:** `JobFilters.tsx:112-136` (`FilterPill`) is local to Jobs. It overlaps conceptually with the Companies `filter-pill` in the mockup. Consider promoting to `components/ui/filter-pill.tsx` with color variants (violet / emerald / red / amber).
- **Side drawer shell:** The wrapper used in both pages (`<div style={{ width }} className="shrink-0 flex flex-col min-h-0 border-l border-white/[0.07] overflow-hidden">`) is duplicated. Extract as `SideDrawer` primitive wrapping drag handle + panel.

---

## 3. File size / structure concerns

All Jobs component files are currently well under 250 lines:

| File | Lines |
|---|---|
| `JobFilters.tsx` | 250 |
| `JobDetailPanel.tsx` | 181 |
| `jobs-columns.tsx` | 136 |
| `JobsTable.tsx` | 68 |
| `DismissModal.tsx` | 67 |
| `JobStatusTabs.tsx` | 40 |

- `JobFilters.tsx` is exactly at the 250-line threshold. Splitting out `CompanyMultiSelect` into `components/ui/multi-select-dropdown.tsx` (Phase 2, item 2.3) drops it to ~140 lines and eliminates the concern.
- `JobDetailPanel.tsx` mixes header + actions + body + all sections in one file. Still under 200 lines, so acceptable, but the sections (Meta badges, Flag indicators, Summary, Responsibilities, Tech stack, Notes) could be split into sub-components once Companies' drawer establishes a shared pattern.
- Logic in hooks — `useJobs` and `useJobActions` are already well-structured; no logic needs to move out of components.
- `JobsPage.tsx` is 181 lines, acceptable. The drag-resize handler (lines 19-32) is duplicated in CompaniesPage — extract when the `ResizeHandle` primitive is built.

---

## 4. Recommendation — ordered Jobs changes for Phase 2

Do these in order AFTER Companies migration lands, so the new primitives exist:

1. **Replace `CompanyMultiSelect`** (`JobFilters.tsx:14-108`) with the shared `components/ui/multi-select-dropdown.tsx` primitive. Verify label-fn behavior still reads "All companies" / single name / "N companies".
2. **Add the missing search input** to the Jobs filter bar (matches Companies pattern; wire to a new `search` field in `useJobs`).
3. **Add Reset button** to the Jobs filter bar — hook `resetFilters` is already exported from `useJobs`, just expose in UI.
4. **Add `On-site`** location pill next to Remote.
5. **Fix tab set + always-on counts** in `JobStatusTabs` — add `interviewing` and `offer` tabs (will need backend support for per-tab counts) and show counts on all tabs, not just active one.
6. **Inline header drift fix** — `py-3` + `text-lg`, move subtitle inline, add "N selected filters" violet chip when any filter is non-default.
7. **Refactor all inline badges** in `jobs-columns.tsx` and `JobDetailPanel.tsx` to use the shared `Badge` component (extended with a `dot` prop if Companies adds one).
8. **Tighten drawer heading typography** — change `text-xs` to `text-[10px]` for section titles in `JobDetailPanel.tsx:143, 150, 161, 174`.
9. **Rework drawer action cluster** — move Dismiss and Close into an absolute top-right cluster; keep Open / Save / Apply in the primary action row.
10. **Extract `ResizeHandle`** primitive and use in both `JobsPage` and `CompaniesPage` (deletes 14 duplicate lines per page).
11. **Swap `Wifi` icon for `Globe`** on the Remote pill in `jobs-columns.tsx:101`.
12. **Verify** `DataTable`'s `<thead>` is sticky on `#0d0d14` (check `components/ui/data-table.tsx`).

Do NOT introduce these changes before Companies migration — they depend on primitives being created first.
