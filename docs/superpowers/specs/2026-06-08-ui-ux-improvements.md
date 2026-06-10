# UI/UX Improvements — Implementation Spec

**Date:** 2026-06-08  
**Plan doc:** `docs/design/2026-06-08-ui-ux-improvement-plan.md`

---

## What was implemented

### Phase 1 — CSS custom property token system

**`frontend/src/index.css`**
Full CSS custom property contract replacing hardcoded body colors. Two selector blocks: `:root` (light) and `.dark` (dark). Covers 13 semantic tokens: `--bg-page`, `--bg-panel`, `--bg-subtle`, `--bg-input`, `--bg-hover`, `--border`, `--border-strong`, `--text-primary`, `--text-secondary`, `--text-muted`, `--text-placeholder`, and sidebar/accent/scrollbar variants. Body and `#root` now use `var()` references.

**`frontend/tailwind.config.js`**
Extended `theme.colors` with semantic Tailwind class names that consume CSS variables: `page`, `panel`, `subtle`, `input-bg`, `hover`, `theme-border`, `border-strong`, `primary`, `secondary`, `muted`, `placeholder`, `accent`, `accent-dim`, `accent-text`, `sidebar`, `sidebar-border`.

**`frontend/index.html`**
Anti-flash inline `<script>` before `</head>`. Reads `localStorage('jobradar-theme')`, resolves `system` via `prefers-color-scheme`, applies `.dark` class synchronously before React mounts. Falls back to dark on any error.

**`frontend/src/app/Sidebar.tsx`**
Migrated from hardcoded hex/opacity classes to semantic token classes (`bg-sidebar`, `border-[var(--sidebar-border)]`, `hover:bg-[var(--sidebar-hover)]`, `bg-[var(--sidebar-active)]`, `text-muted`, `text-primary`). Added `type="button"` to collapse toggle. Added `role="separator"` to admin section divider.

**`frontend/src/components/ui/dialog.tsx`**
Fixed default className: `bg-white` → `bg-[#141420]`, `border-slate-200` → `border-white/[0.08]`. Eliminates white flash during dialog open animation.

**`frontend/src/components/ui/status-pill.tsx`**
`pending` label: "Never" → "Not scanned".

**`frontend/src/components/ui/kpi-card.tsx`**
`active` variant: red border/bg → violet (`border-violet-500/40 bg-violet-500/[0.05]`). Corrects misleading "selected = red" semantics.

---

### Phase 2 — Critical bug fixes

**`frontend/src/pages/JobsPage.tsx`** + **`frontend/src/features/jobs/components/jobs-columns.tsx`** + **`frontend/src/pages/jobs/JobResumeSection.tsx`**
Removed `getMockAttachmentForJob` import from all three production paths. Attachment state always returns null until a real API exists. Eliminates mock data appearing for real users.

**`frontend/src/pages/admin/AdminAuditPage.tsx`** + **`frontend/src/pages/admin/AdminUsersPage.tsx`**
Added `isError` + `refetch` to query destructuring. Added error branch (message + retry button) before table render. API failures no longer silently show "No results".

**`frontend/src/pages/AccountPage.tsx`**
Removed direct `supabase.auth.signOut()` call from `handleDelete` (was double-firing). Changed `displayName` to `useState('')` + `useEffect` sync from `identity?.display_name` to stay fresh after profile save.

**`frontend/src/features/jobs/components/JobDetailPanel.tsx`**
Save state auto-clears after 2s via `saveTimerRef`. Added cleanup on unmount. Fixed `saveTimerRef` type to `ReturnType<typeof setTimeout> | null`.

**`frontend/src/components/ui/dismiss-stripe.tsx`**
`w-[360px]` → `max-w-[360px] w-full` on toast container. Prevents overflow on narrow viewports.

**`frontend/src/pages/admin/ApplyQueuePage.tsx`**
Added `type="button"` to all filter tab buttons.

**`frontend/src/features/jobs/components/JobStatusTabs.tsx`**
`CountPill`: count > 999 renders `999+`.

**`frontend/src/features/dashboard/components/VelocityPanel.tsx`**
`+{n}` suppressed for 0: renders `—` instead of `+0`.

**`frontend/src/App.tsx`**
Root-level `ErrorBoundary` wraps entire `BrowserRouter` tree. Unhandled crashes now show fallback instead of blank screen.

**`frontend/src/features/dashboard/components/ProgressPanel.tsx`**
Offer sub-label: dynamic pluralisation ("decision pending" / "N decisions pending" / "no offers yet").

---

### Phase 3 — Theme toggle

**`frontend/src/types/index.ts`**
Added `ThemePreference = 'dark' | 'light' | 'system'` type alias. Added `theme?: ThemePreference` to `UserConfig`.

**`frontend/src/lib/api/userConfig.ts`**
Extended Zod `UserConfigSchema` with `theme: z.enum(['dark','light','system']).optional()`.

**`frontend/src/app/ThemeContext.ts`** _(new)_
Pure `.ts` module: `ThemeContext`, `ThemeContextValue`, `useTheme()` hook, `resolveSystemTheme()` helper. Separated from provider to satisfy `react-refresh/only-export-components`.

**`frontend/src/app/ThemeProvider.tsx`** _(new)_
`ThemeProvider` component: applies `.dark` class + `colorScheme` style on preference change, listens for OS `prefers-color-scheme` when preference is `system`, writes to `localStorage('jobradar-theme')`.

**`frontend/src/app/providers.tsx`**
`ThemeSync` inner component reads `useUserConfig()` + `usePatchUserConfig()`, wraps children with `ThemeProvider`. `ThemeAwareToaster` reads resolved theme from context and passes to Sonner (replaces hardcoded `theme="dark"`). Both sit inside `QueryClientProvider`.

**`frontend/src/pages/AccountPage.tsx`**
`ThemeToggle` component: 3-button segmented control (Light / Dark / System) using `useTheme()`. "Appearance" section added between Profile and Keyboard shortcuts sections.

---

### Phase 4 — Resume feature UX

**`frontend/src/pages/admin/ResumeLibraryPage.tsx`**
Loading: animated 6-card skeleton grid. Error: retry block. Upload/New/empty-state buttons: disabled + tooltip "Coming soon — file upload is in progress". Removed mock toast calls.

**`frontend/src/pages/admin/ResumeDetailPage.tsx`**
Loading: 3-section animated skeleton. Error: retry block. Replaced hardcoded `/ 20` with `VERSION_LIMIT_PER_RESUME` constant.

**`frontend/src/pages/admin/ApplyQueuePage.tsx`**
Loading: 5-row skeleton. Error: retry block.

**`frontend/src/features/resumes/components/ApplyQueueTable.tsx`**
Loading: 5-row skeleton.

**`frontend/src/features/resumes/components/JobContextPanel.tsx`**
Removed all hardcoded bullet strings. Renders real `job.responsibilities` / `job.requirements` / `job.tech_stack` / `job.role_brief` when available. Falls back to an honest placeholder: "Job description not yet enriched."

**`frontend/src/features/resumes/types.ts`**
Added optional `responsibilities`, `requirements`, `role_brief`, `tech_stack` fields to `ApplyQueueItem.job`.

**`frontend/src/features/resumes/hooks/useResume.ts`**
Uses `getAllMockAttachments()` to merge `MOCK_ATTACHMENTS` + `EXTRA_ATTACHMENTS`. Fixes count mismatch between metadata panel and attached jobs list.

**`frontend/src/features/resumes/mockData.ts`**
Added `getAllMockAttachments()` helper export.

**`frontend/src/features/resumes/constants.ts`** _(new)_
`AXIS_TONE` (compact) and `AXIS_TONE_WITH_RING` (detail) shared constants — deduplicates definition from `ResumeCard` and `ResumeMetadataPanel`.

**`frontend/src/features/resumes/components/ResumeCard.tsx`** + **`ResumeMetadataPanel.tsx`**
Both import from `../constants` instead of defining `AXIS_TONE` locally.

**`frontend/src/features/resumes/index.ts`**
Added `VERSION_LIMIT_PER_RESUME` to barrel export.

**`frontend/src/pages/admin/ResumeEditorPage.tsx`**
Loading: skeleton. Unknown `?job=` guard: error state instead of silent solo-mode fallback. Solo-edit info bar: shows resume name + version number + change summary when no job context is active.

---

## What's left in the plan

See `docs/design/2026-06-08-ui-ux-improvement-plan.md` for the full backlog. Remaining items:

- **Phase 4 (partial):** H-priority UX issues (filter wrap, reset button opacity, `MultiSelectDropdown` portal, admin `DeleteDialog` hoisting, accessibility sweep M1–M20)
- **Phase 5:** Resume backend (migrations, Go service, REST endpoints, Supabase Storage, `src/lib/api/resumes.ts`, TanStack mutations, remove all mock data exports)
- **Phase 7 continuation:** Backend `UserConfig` migration for `theme` column (frontend already typed; API already schema-validated)

## To preview changes

Port 30300 serves the k3d production build. Changes require a rebuild:

```bash
# Option A — dev server (immediate HMR)
make dev-console         # starts Vite at localhost:5173

# Option B — rebuild k3d (matches production)
make k8s-build-console   # builds Docker image with new frontend
make k8s-up              # helm upgrade
```
