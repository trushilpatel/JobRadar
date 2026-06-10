# UI/UX Improvement Plan — JobRadar Frontend

**Date:** 2026-06-08  
**Scope:** Full frontend audit, theme system, UI quality fixes, resume feature completion  
**Status:** Active

---

## Context

A full UI/UX and code-quality audit was conducted across all 8 pages and ~40 components of the frontend (Chrome visual audit + full source read). Three workstreams were identified: (1) a theme/token system to enable configurable light/dark mode, (2) cross-cutting UI/code quality fixes, and (3) completing the resume feature end-to-end.

---

## Workstream 1 — Theme System (Light / Dark / System)

### Current state

- Pure dark UI. Every color is a hardcoded Tailwind class or hex string (`#0a0a0f`, `#0d0d14`, `bg-slate-900`, `border-white/[0.07]`).
- `tailwind.config.js` has `darkMode: 'class'` — configured but never activated.
- `index.css` has zero CSS custom properties.
- `UserConfig` has one field: `show_keyboard_shortcuts`. No theme preference.
- Sonner `<Toaster theme="dark">` is hardcoded.

### Implementation plan

#### Step 1 — CSS custom properties (`index.css`)

Define a full token set on `:root` (light) and `.dark`:

```
Tokens: --bg-page, --bg-panel, --bg-subtle, --bg-input,
        --border, --border-strong,
        --text-primary, --text-secondary, --text-muted,
        --sidebar-bg, --sidebar-border,
        --accent, --accent-dim
```

Dark values match existing hardcoded hex palette. Light values provide a clean slate/neutral equivalent.

#### Step 2 — Tailwind wiring (`tailwind.config.js`)

Extend `theme.colors` to map semantic names → CSS variables:

```js
colors: {
  page:    'var(--bg-page)',
  panel:   'var(--bg-panel)',
  subtle:  'var(--bg-subtle)',
  input:   'var(--bg-input)',
  // etc.
}
```

This makes `bg-page`, `text-primary`, etc. valid Tailwind utilities.

#### Step 3 — Anti-flash script (`index.html`)

Inline script before `</head>` that reads the persisted preference from `localStorage` (fallback) and applies `.dark` to `<html>` before React mounts. Eliminates flash of wrong theme (FOWT).

#### Step 4 — Backend: `UserConfig.theme` field

Add `theme: 'dark' | 'light' | 'system'` (default `'dark'`) to:

- Backend `UserConfig` struct + DB migration (JSONB column or separate column depending on current implementation)
- Frontend `UserConfig` type in `types/index.ts`
- `useUserConfig` hook — no changes needed; field is carried transparently

#### Step 5 — `ThemeProvider` context

New context at `src/app/ThemeProvider.tsx`:

- Reads `userConfig.theme` on mount
- Writes `.dark` class to `document.documentElement`
- Exposes `setTheme(theme)` — updates `userConfig` via `updateUserConfig` mutation and applies class synchronously
- Listens to `window.matchMedia('(prefers-color-scheme: dark)')` change events when `theme === 'system'`
- Wraps `ProtectedLayout`

#### Step 6 — Component migration

Systematic find-and-replace across all components. Ordered by impact:

1. `index.css` body background/color → `var(--bg-page)` / `var(--text-primary)`
2. `Sidebar.tsx`
3. All page-level wrapper divs
4. `kpi-card`, `data-table`, `scan-history-bar`
5. Dialog/Sheet/Drawer components (remove `bg-white` defaults)
6. All remaining components

Strategy: per-surface grep-and-replace of the 5 distinct background hex values, then address `text-slate-*` and `border-white/*` patterns.

#### Step 7 — Account page toggle

Add "Appearance" section to `/account` above the Keyboard shortcuts section:

- 3-way segmented control: **Light / Dark / System**
- Calls `setTheme()` from `ThemeProvider` context
- Persists via `/me/config` PATCH

---

## Workstream 2 — UI / Code Quality Fixes

### Critical (breaks functionality or misleads users)

| #   | Issue                                                                                                  | File                                                       | Fix                                                                                                                                            |
| --- | ------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| C1  | `getMockAttachmentForJob` imported in production paths — returns mock data for real users              | `JobsPage.tsx`, `JobResumeSection.tsx`, `jobs-columns.tsx` | Remove mock import; render attachment column only when real attachment API exists                                                              |
| C2  | Double toast on job dismiss (dismiss stripe + undo toast fire simultaneously)                          | `JobsPage.tsx`                                             | Consolidate to single toast with undo action                                                                                                   |
| C3  | `ResumeLibraryPage` upload/new buttons fire `toast.info('mockup')` — users think feature is broken     | `ResumeLibraryPage.tsx`                                    | Disable buttons with tooltip "Coming soon" until backend lands; remove mock toast                                                              |
| C4  | Empty state "Upload first resume" button has no onClick                                                | `ResumeLibraryPage.tsx`                                    | Same as C3 treatment — show disabled state                                                                                                     |
| C5  | `AdminAuditPage` and `AdminUsersPage` show "No results" on API error instead of error state            | `AdminAuditPage.tsx`, `AdminUsersPage.tsx`                 | Add `isError` branch with retry affordance                                                                                                     |
| C6  | `dialog.tsx` defaults include `bg-white border-slate-200` — flashes white before override styles apply | `components/ui/dialog.tsx`                                 | Change defaults to match dark palette                                                                                                          |
| C7  | `pagination.tsx` `<select>` is unstyled native element; renders white background in dark mode          | `components/ui/pagination.tsx`                             | Replace with Radix `Select` primitive                                                                                                          |
| C8  | `AdminUsersPage` native `<select>` for status filter                                                   | `AdminUsersPage.tsx`                                       | Replace with Radix `Select`                                                                                                                    |
| C9  | `JobDetailPanel` notes `queryClient.invalidateQueries({ queryKey: ['jobs'] })` uses bare string        | `JobDetailPanel.tsx`                                       | Use `jobQueryKeys.list(...)` factory                                                                                                           |
| C10 | `AccountPage` calls `supabase.auth.signOut` directly then auth store `signOut` — double call           | `AccountPage.tsx`                                          | Remove direct Supabase call; use store method only                                                                                             |
| C11 | `displayName` input not updated after `refreshIdentity()` succeeds                                     | `AccountPage.tsx`                                          | Re-seed form state from identity after successful save                                                                                         |
| C12 | `JobContextPanel` body is entirely hardcoded strings, not actual job data                              | `JobContextPanel.tsx`                                      | Show real job description fields (from `job.responsibilities` / `job.role_brief`) with a clear "content not yet enriched" placeholder fallback |

### High (degrades UX meaningfully)

| #   | Issue                                                                                  | File                                                             | Fix                                                                                   |
| --- | -------------------------------------------------------------------------------------- | ---------------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| H1  | `JobsTable` re-renders O(n²) per row via `findIndex` in `getRowClassName`              | `JobsTable.tsx`                                                  | Compare `jobs[focusedIndex]?.id === job.id` directly                                  |
| H2  | `JobStatusTabs` `CountPill` doesn't cap at 999+                                        | `JobStatusTabs.tsx`                                              | Add `count > 999 ? '999+' : count` rendering                                          |
| H3  | Skeleton replaces existing rows on next-page fetch (disorienting)                      | `JobsTable.tsx`                                                  | Pass separate `isFetchingNextPage` prop; show bottom spinner instead of full skeleton |
| H4  | `DismissStripe` toast width `w-[360px]` overflows narrow viewports                     | `dismiss-stripe.tsx`                                             | Use `max-w-[360px] w-full`                                                            |
| H5  | `MultiSelectDropdown` does not close on touch outside                                  | `multi-select-dropdown.tsx`                                      | Add `touchstart` to the outside-click listener                                        |
| H6  | `MultiSelectDropdown` dropdown clips inside overflow-hidden parents                    | `multi-select-dropdown.tsx`                                      | Render via Radix Popover or portal                                                    |
| H7  | Pagination "0–0" range when filter returns empty on a non-first page                   | `pagination.tsx`                                                 | Auto-reset to first page or show contextual message                                   |
| H8  | `AdminUsersPage` mounts N `DeleteDialog` instances inside list                         | `AdminUsersPage.tsx`                                             | Hoist single dialog outside list, populate on demand                                  |
| H9  | `VelocityPanel` shows "+0" for companies with no new jobs                              | `VelocityPanel.tsx`                                              | Suppress `+` prefix when value is 0                                                   |
| H10 | `JobDetailPanel` "Saved" notes indicator never auto-clears                             | `JobDetailPanel.tsx`                                             | Clear after 2s using `setTimeout`                                                     |
| H11 | `ApplyQueuePage` filter tabs missing `type="button"`                                   | `ApplyQueuePage.tsx`                                             | Add `type="button"` to all tab buttons                                                |
| H12 | `AdminAuditPage` uses in-render state-set pattern for page reset                       | `AdminAuditPage.tsx`, `AdminUsersPage.tsx`                       | Replace with `useEffect` / reducer                                                    |
| H13 | `JobFilters` and `MobileFilterDrawer` duplicate filter option constants                | Both files                                                       | Extract to shared `src/features/jobs/constants/filterOptions.ts`                      |
| H14 | `EXP_OPTIONS` / `DATE_OPTIONS` / `LOCATION_OPTIONS` duplicated across two filter files | Same as H13                                                      | Same fix                                                                              |
| H15 | `HeatmapPanel` `HEAT_COLORS` hardcoded hex array                                       | `HeatmapPanel.tsx`                                               | Extract to design constants                                                           |
| H16 | `TechTrendsPanel` sparkline colors hardcoded inline                                    | `TechTrendsPanel.tsx`                                            | Same                                                                                  |
| H17 | `InterviewingPanel` `STAGES` dot colors hardcoded hex                                  | `InterviewingPanel.tsx`                                          | Same                                                                                  |
| H18 | `ProgressPanel` "Offer" sub-label always "decision pending" regardless of count        | `ProgressPanel.tsx`                                              | Pluralise based on count                                                              |
| H19 | `ScanHistoryBar` uses native `title` tooltip instead of Radix Tooltip                  | `scan-history-bar.tsx`                                           | Replace with Radix `Tooltip`                                                          |
| H20 | `JobsPage` `HIDDEN_COMPANIES_CATEGORY` magic string defined in 3 places                | `JobsPage.tsx`, `dismiss-stripe.tsx`, `MobileJobDetailRoute.tsx` | Extract to `src/config/constants/`                                                    |

### Medium (accessibility, consistency, polish)

| #   | Issue                                                                                  | Fix                                                       |
| --- | -------------------------------------------------------------------------------------- | --------------------------------------------------------- |
| M1  | `LoginPage` email input has no `aria-label`; error not linked via `aria-describedby`   | Add `<label>` + `aria-describedby`                        |
| M2  | `CompanyAvatar` has no `role="img"` or `aria-label`                                    | Add both                                                  |
| M3  | `ScanHistoryBar` container has no accessible role                                      | Add `role="img"` + `aria-label` summary                   |
| M4  | `HeatmapPanel` color legend squares have no accessible text                            | Add `aria-label` per square                               |
| M5  | Recharts charts have no accessible fallback                                            | Add `role="img"` + `aria-label` on `ResponsiveContainer`  |
| M6  | `AdminAuditPage` filter inputs have no labels                                          | Add `aria-label`                                          |
| M7  | `AdminUsersPage` action icon buttons have no visible label text                        | Add visible label or at minimum `aria-label`              |
| M8  | `MultiSelectDropdown` has no `role="listbox"` / `role="option"`                        | Add ARIA roles                                            |
| M9  | `KbdGroup` typed as `div` but renders as `kbd`; double `dark:dark:` in Kbd styles      | Fix type; fix CSS selector                                |
| M10 | `StatusPill` "Never" label for `pending` state                                         | Change label to "Not scanned"                             |
| M11 | `Insights` listed under admin nav but contains user-level analytics                    | Move to main nav for authenticated users                  |
| M12 | `Button` component defaults (bg-slate-900 / focus ring) don't match app's dark palette | Fix defaults or document the custom usage pattern         |
| M13 | `AdminAuditPage` timestamps use `toLocaleString()` with no locale/timezone option      | Use consistent `Intl.DateTimeFormat` with explicit locale |
| M14 | `KpiCard` `active` variant uses red border — misleading for "selected" state           | Change to violet to match active state convention         |
| M15 | `SegmentedSwitch` `aria-label` prop not enforced                                       | Make prop required or add runtime warning                 |
| M16 | Sidebar collapsed state has no tooltip on user avatar (uses native `title` only)       | Use Radix `Tooltip`                                       |
| M17 | Sidebar `<button>` for collapse has no `type="button"`                                 | Add `type="button"`                                       |
| M18 | Sidebar admin section separator has no `role="separator"`                              | Add role                                                  |
| M19 | `dt`/`dd` order in shortcut reference is reversed semantically                         | Swap to `dt=action, dd=keys`                              |
| M20 | No root-level `ErrorBoundary` wrapping the whole app                                   | Add in `App.tsx`                                          |

---

## Workstream 3 — Resume Feature (Frontend UX)

### Intended end-to-end flow

```
Library (/admin/resumes)
  → Browse cards, upload or create new resume
  ↓
Detail (/admin/resumes/:id)
  → View metadata, version history, attached jobs
  → Select version to preview
  → Click Edit
  ↓
Editor (/admin/resumes/:id/edit?job=:jobId)
  → Edit resume text in left pane
  → View job description in right pane (split view)
  → Save creates new immutable version
  → "Mark as final" → advances attachment state
  ↓
Apply Queue (/admin/apply-queue)
  → Tabbed view: drafts, final, submitted, all
  → Attach/change resume inline
  → Click Submit to record application sent
  ↓
Jobs feed (/jobs → detail panel)
  → JobResumeSection shows attached resume
  → Resume picker inline; editor link
```

### What works today

- Routing (Library → Detail → Editor → Apply Queue) ✓
- Version history browsing in Detail page (local state) ✓
- Split-view layout in Editor (when `?job=` set) ✓
- Mock data filtering in Apply Queue tabs ✓
- Dirty state tracking in content pane ✓

### What is broken / mocked

| Area                                            | Issue                                                         |
| ----------------------------------------------- | ------------------------------------------------------------- |
| Upload / create                                 | `toast.info` stub — no file picker, no API call               |
| Save version                                    | No-op — `onSave` is `() => {}` in editor page                 |
| Mark as final                                   | No PATCH call — only toast + navigate                         |
| Mark submitted                                  | No PATCH call — only toast + invalidate                       |
| Archive / Download                              | No-op stubs                                                   |
| Attach resume to job (picker)                   | Toast fires but no attachment created                         |
| Attach resume to job (modal)                    | Navigates to editor but no attachment created                 |
| `getMockJob` in `AttachedJobsList`              | Returns null for every real job ID                            |
| `getMockAttachmentForJob` in `JobResumeSection` | Returns mock data for real users                              |
| `JobContextPanel` body                          | Entirely hardcoded strings — not actual job content           |
| `ResumePickerInline`                            | N+1: fires `useResume` query per table row                    |
| Loading states                                  | Bare "Loading…" strings — should be skeletons                 |
| Error states                                    | Not handled in hooks or pages                                 |
| Solo editor mode                                | No sidebar: user can't see/switch versions without leaving    |
| Metadata editing                                | Tags, name, label are all read-only — no edit affordance      |
| Data consistency                                | `useResume` excludes `EXTRA_ATTACHMENTS` → count mismatch     |
| Constants                                       | `VERSION_LIMIT_PER_RESUME` hardcoded as `20` in storage panel |
| Dead route params                               | `queue` and `returnTo` in `routes.resumeEdit` never used      |

### Frontend-only fixes (no backend required)

These can ship now regardless of backend readiness:

1. **Skeleton loading states** — Replace "Loading…" strings with skeleton shimmer in Library, Detail, Apply Queue
2. **Error states** — Add `isError` branches across all resume hooks/pages with retry buttons
3. **Disable stub actions clearly** — Upload, New, Save, Submit buttons show `disabled` + tooltip "Backend coming soon" instead of deceptive mock toasts
4. **`JobContextPanel` placeholder** — When job description fields are not populated, show "Job description not yet enriched" placeholder instead of hardcoded fake bullets
5. **Solo editor sidebar** — Add compact version list sidebar to editor when no `?job=` param (so user knows which version they're editing)
6. **Fix data consistency** — `useResume` hook should use `[...MOCK_ATTACHMENTS, ...EXTRA_ATTACHMENTS]` for res-1 lookup
7. **`ResumePickerInline` N+1** — Accept resume name as prop from parent instead of querying per row
8. **`VERSION_LIMIT_PER_RESUME` constant** — Use imported constant in storage panel instead of hardcoded `20`
9. **`AXIS_TONE` deduplication** — Extract shared constant from `ResumeCard` and `ResumeMetadataPanel`
10. **Unknown `?job=` handling** — When `?job=` is set to an unknown ID, show error state instead of silently switching to solo mode
11. **`returnTo` param** — Wire `returnTo` in `routes.resumeEdit` so editor "Back" button goes to the originating location (apply queue vs. detail page)
12. **Consistent empty states** — Library empty state button should clearly communicate it's non-functional (or show a proper coming-soon modal)

### Backend work required (separate plan)

- Migrations: `user_resumes`, `resume_versions`, `resume_job_attachments`
- Go service following decorator chain
- REST endpoints (CRUD for resumes, versions, attachments)
- Supabase Storage integration for file upload/download
- `src/lib/api/resumes.ts` frontend API module
- TanStack `useMutation` hooks for all write operations
- Remove all mock data imports from production components

---

## Cross-cutting design system gaps (no-backend-required)

1. **5 near-identical dark background hex values** across admin pages — should be 1–2 tokens
2. **Inconsistent empty states** — 4 different visual treatments across the app
3. **Inconsistent error states** — most pages don't have them at all
4. **Chart accessibility** — all Recharts components lack ARIA
5. **No "last updated" timestamp** on any data panel despite `refetchInterval`
6. **`Settings` page label** misleads (only contains ATS config, not general settings)

---

## Implementation order

### Phase 1 — Token system foundation (enables everything else)

- CSS custom properties + Tailwind wiring
- Anti-flash script in `index.html`

### Phase 2 — Critical bugs (stop the bleeding)

- C1 (mock data in production)
- C2 (double toast)
- C3/C4 (deceptive resume mock toasts)
- C5 (admin pages swallow errors)
- C6 (dialog white flash)
- C7/C8 (native selects)

### Phase 3 — Theme toggle

- Backend `UserConfig.theme` field
- `ThemeProvider`
- Account page control

### Phase 4 — Component migration to tokens

- Sidebar, layout, cards, table, dialogs

### Phase 5 — Resume UX (frontend-only)

- Skeleton states, error states, stub affordances, solo editor sidebar, data consistency fixes

### Phase 6 — High/Medium issues

- Accessibility sweep (M1–M20)
- UX polish (H1–H20)

### Phase 7 — Resume backend + full feature

- Migrations, Go service, API module, mutations

---

## Files affected (top-level)

```
frontend/
  index.html                         — anti-flash script
  src/index.css                      — CSS custom properties
  tailwind.config.js                 — token wiring
  src/types/index.ts                 — UserConfig.theme field
  src/app/ThemeProvider.tsx          — new file
  src/app/providers.tsx              — wrap with ThemeProvider
  src/app/Sidebar.tsx                — token classes + a11y fixes
  src/pages/AccountPage.tsx          — theme toggle UI
  src/components/ui/dialog.tsx       — fix defaults
  src/components/ui/pagination.tsx   — replace native select
  src/components/ui/multi-select-dropdown.tsx — portal + touch
  src/components/ui/scan-history-bar.tsx      — Radix tooltip
  src/components/ui/dismiss-stripe.tsx        — viewport fix
  src/features/jobs/components/      — mock data removal, fixes
  src/features/resumes/              — skeleton/error states
  src/pages/admin/AdminAuditPage.tsx — error state, labels
  src/pages/admin/AdminUsersPage.tsx — error state, dialog hoist
  src/pages/admin/ResumeLibraryPage.tsx — stub affordances
  src/config/constants/              — shared magic strings
```
