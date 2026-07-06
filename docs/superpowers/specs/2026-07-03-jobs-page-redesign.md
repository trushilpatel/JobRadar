# Jobs Page Redesign — Implementation Spec

**Date:** 2026-07-03
**Plan:** `docs/superpowers/plans/2026-07-03-jobs-page-redesign.md`

## What shipped

Rebuilt the Jobs page's filters, status tabs, list, and detail panel to match the approved mockup (`design/jobs-redesign/jobs.html`), while keeping every feature the current app already had beyond the mockup (command palette, keyboard registry, undo stack, backend-persisted notes, resume attachment).

### Filters (Task 1)

- New `frontend/src/features/jobs/components/FilterPopover.tsx` and `FilterChip.tsx` — Popover+Command+Checkbox facet pattern, active-filter chip row.
- Visa/sponsorship filter added end-to-end: `frontend/src/features/jobs/constants/filterOptions.ts` (`VISA_OPTIONS`), `hooks/useJobUrlState.ts` (`visa` param), `hooks/useJobs.ts`, `api/jobs.api.ts` (`sponsorship` param) — wired to the pre-existing backend `?sponsorship=` filter, no backend change needed.
- `components/JobFilters.tsx` and `MobileFilterDrawer.tsx` reworked to use the new popovers.

### Status tabs / Tracking (Task 3)

- New `frontend/src/features/jobs/components/SegmentedTabs.tsx` — generic pill-segmented control.
- `components/JobStatusTabs.tsx` rewritten: New / Saved / Tracking (3 top-level segments); Tracking opens a checkbox sub-menu for the 4 workflow stages with a combined count.
- Backend: `backend/service/job/dto/JobFilter.go` gained `Statuses []string`; `JobRestService.go` parses a repeated `statuses` query param; `UserJobStateRepositoryService.go`'s `GetJobsByStatus` branches on `len(Statuses) > 0` using `IN (...)`. Also touched `JobImplService.go` (dispatch condition) and `JobValidationService.go` (whitelist check).
- `frontend/src/config/constants/jobStatus.ts`: `TAB_KEYS` gained `'tracking'`; the 4 individual workflow statuses stay in `TAB_KEYS` only so a legacy `?tab=applied`-style deep link still parses — `useJobUrlState.ts` redirects it into `tracking` with that one stage pre-checked.
- Scope note: the approved mockup actually has a 4th top-level tab ("All", showing every non-dismissed job) that was not implemented — it would need a new backend query capability beyond the multi-status change above. Flagged as a follow-up, not built.

### Infinite scroll + Apply action (Task 4)

- `hooks/useJobs.ts`: hand-rolled page accumulation (`accumulatedJobs` state + `loadMore()`) on top of the existing `useQuery` + cursor-history pattern — explicitly not `useInfiniteQuery` (forbidden pattern in this file). Classic Prev/Next pagination UI removed from the Jobs page.
- `components/JobsTable.tsx`: `IntersectionObserver`-based sentinel row (300px root margin, matching the mockup), replaces the `Pagination` component for this page (component itself untouched — still used by Companies/Users/Audit pages).
- Row-level Apply action: `components/jobs-columns.tsx` (always-visible icon column) + `hooks/useJobActions.ts` (`applyJob` mutation — opens the posting URL and auto-sets status to `applied` unless already applied/interviewing/offer/rejected).
- Removed dead pagination-keyboard machinery: `jobs.nav.prev-page`/`jobs.nav.next-page` commands and the double-press-left "jump to first page" quirk in `commands/jobCommands.ts` / `hooks/useJobKeyboard.ts` — structurally incompatible with infinite scroll.

### Detail drawer (Task 5)

- `pages/JobsPage.tsx`: the resizable inline side panel (`ResizeHandle`, `panelWidth` state) is replaced by a **non-modal** `Sheet` overlay (`components/ui/sheet.tsx`, new `showOverlay` prop added) — fixed width `min(460px, 94vw)`, slide-in from the right.
- Deliberately non-modal with no dimming overlay: this app's detail panel auto-opens whenever a row is focused (Gmail/Superhuman "always-open reading pane" pattern, pre-existing), so a true modal dialog would make the underlying table `aria-hidden`/non-interactive almost all the time on desktop. `modal={false}` + `showOverlay={false}` + suppressed outside-interaction dismissal keep the table fully clickable and keyboard-navigable while the drawer is open.
- `components/JobDetailPanel.tsx`: `StatusSelect` (Radix `Select` dropdown) replaced by a new inline `StatusSegmentedControl` (5 evenly-distributed segments, `role="radiogroup"`) matching the mockup's `.statusseg`. `StatusSelect.tsx` deleted (no other consumers).

### Cross-cutting fix

Found and fixed during visual QA: `tailwind.config.js` defines brand colors (`violet`, `cyan`, `magenta`, `lime`, etc.) as flat single values, which silently overrides Tailwind's built-in numbered shade scale for that key — `bg-violet-600`-style classes generate no CSS at all, anywhere in the app. Documented in `frontend/CLAUDE.md` §10 and fixed in every file this redesign touched (`checkbox.tsx`, `FilterPopover`, `FilterChip`, `SegmentedTabs`, `JobStatusTabs`, `JobFilters`, `MobileFilterDrawer`, `jobs-columns.tsx`) using the bare token or `color-mix()` pattern. Not fixed app-wide (Companies/Users/Pipeline pages, etc.) — separate follow-up.

Also added: `IntersectionObserver` mock in `frontend/src/test/setup.ts` (jsdom has no native implementation; needed once the infinite-scroll sentinel existed).

## Known follow-ups (not built)

- "All" top-level tab from the mockup (needs a new backend union-query capability).
- App-wide sweep for the numbered-shade Tailwind bug outside the files this redesign touched.
- Pre-existing coverage gaps in `useJobActions.ts` (~87%) and `pages/JobsPage.tsx` (~94%) — neither introduced by this work, not backfilled (out of scope).
