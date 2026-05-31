# Jobs Page UX Fixes — 2026-05-16

Branch: `investigation/jobs-page-ux-audit`
Trigger: User-reported keyboard bug on `/jobs` ("shortcuts act on the first job, not the active one"). A 7-agent audit followed, finding 23 P0-to-P2 issues plus accessibility gaps. This document is the prevention record — what changed, what to enforce in code review, and what tests to keep green so the same bugs don't recur.

## What changed (file-level only)

### Hooks

- [frontend/src/features/jobs/hooks/useFocusedJob.ts](../../../frontend/src/features/jobs/hooks/useFocusedJob.ts) — refactored to track focus by **job id** (was: array index). Adds `resetKey` option for deliberate context-change resets. Clamps to the prior index when the focused job is removed. Exposes `setFocusedJobId`.
- [frontend/src/features/jobs/hooks/useJobActions.ts](../../../frontend/src/features/jobs/hooks/useJobActions.ts) — fixed the `lookupStatus` query-key predicate (P0: every undo silently reverted to NEW because the predicate `['jobs', 'list']` never matched the writer's `['jobs', filters, limit]`). Added optimistic UI for single mutations, a `runWithLimit` concurrency throttle (5 in flight) for bulk paths, a 20-entry cap on the undo stack, `toast.warning('Nothing to undo')` on empty pop, and `invalidateUserData` that fans out to `['jobs']` + `['dashboard']` + `['insights']`.
- [frontend/src/features/jobs/hooks/useJobKeyboard.ts](../../../frontend/src/features/jobs/hooks/useJobKeyboard.ts) — removed the `Cmd/Ctrl+A` binding (it hijacked the browser's text select-all), scoped `←` / `→` page nav via an `isInPopover()` guard so they no longer close Radix Selects, added `shift+/` as a cross-layout help fallback.
- [frontend/src/features/jobs/hooks/useJobUrlState.ts](../../../frontend/src/features/jobs/hooks/useJobUrlState.ts) — replaced the dead `cursor` URL slot with `page` (integer). Per-setter history policy: `push` for tab/filter/sort/limit changes, `replace` for search debounce / pagination / preview-open.
- [frontend/src/features/jobs/hooks/useJobs.ts](../../../frontend/src/features/jobs/hooks/useJobs.ts) — page now read/written via URL state (replaces local `useState`). Auto-snap effect: when a refetch returns fewer pages than the current `page` index, snap back to the last available page. Now exposes `isError` so the table can render a retry affordance.
- [frontend/src/features/jobs/queryKeys.ts](../../../frontend/src/features/jobs/queryKeys.ts) — added an `isJobListKey()` predicate that all jobs-list cache scans go through, so a key shape drift can't silently break lookup again.

### Components

- [frontend/src/features/jobs/components/JobDetailPanel.tsx](../../../frontend/src/features/jobs/components/JobDetailPanel.tsx) — notes textarea with 800 ms debounce + save-on-blur, `<StatusSelect>` rendered in both desktop and mobile-route modes (so Interviewing / Offer / Rejected are reachable on phone), `pb-[max(0.75rem,env(safe-area-inset-bottom))]` on the sticky action bar, panel container labelled `role="region" aria-label="Job details"`.
- [frontend/src/features/jobs/components/DismissModal.tsx](../../../frontend/src/features/jobs/components/DismissModal.tsx) — `role="radiogroup"` + `role="radio"` semantics with arrow-key + Home/End nav, optional free-text note textarea (max 500 chars, auto-fills from the selected label when empty), `count` prop for bulk mode (title becomes "Dismiss N jobs?"), new `onConfirm({reason, note})` signature.
- [frontend/src/features/jobs/components/JobsTable.tsx](../../../frontend/src/features/jobs/components/JobsTable.tsx) — wired `getRowProps` to make rows tabbable (`tabIndex=0`, `role="button"`, `aria-label`, Enter/Space → open), focused-row indicator switched to a strong violet inset ring, accepts `isError` + `onRetry`.
- [frontend/src/features/jobs/components/jobs-columns.tsx](../../../frontend/src/features/jobs/components/jobs-columns.tsx) — checkbox `aria-label`s ("Select all jobs on this page", "Select {title} at {company}").
- [frontend/src/features/jobs/components/MobileJobDetailRoute.tsx](../../../frontend/src/features/jobs/components/MobileJobDetailRoute.tsx) — `navigate(-1)` falls back to `navigate('/jobs')` when there's no in-app history (deep-link / refresh). Error UI differentiates 404 / 403 / network with a Retry affordance for network errors. Threads `note` through `dismissJob`.

### Primitives

- [frontend/src/components/ui/data-table.tsx](../../../frontend/src/components/ui/data-table.tsx) — `isError` + `onRetry` props with a real error state (no more "looks like an empty list"), skeleton rows during `loading` (replaces the centered spinner), `aria-busy` while loading, `aria-sort` on sortable headers, `getRowProps` callback to let callers inject row-level a11y attrs without leaking domain knowledge into the primitive.

### Page

- [frontend/src/pages/JobsPage.tsx](../../../frontend/src/pages/JobsPage.tsx) — syncs `focus.setFocusedJobId(job.id)` on row click (eliminates the headline bug), prunes `selectedIds` whenever the visible jobs change (no more phantom-id bulk dismiss), routes both single and bulk dismiss through `DismissModal` (bulk no longer silently bypasses the reason capture), bulk action bar gains Save / Mark applied buttons + `role="status"` live region, auto-clears stale `?job=` when the open job is filtered out, scroll restoration via `sessionStorage`, mounts `useJobKeyboard` only on non-mobile, wires `isError` + `refetch` into the table.

### App / root

- [frontend/src/App.tsx](../../../frontend/src/App.tsx) — `h-screen` → `h-dvh` (dynamic viewport height) so the iOS Safari address-bar collapse no longer causes layout jumps.

## Architecture rules — enforce in code review

These are the patterns that, when violated, produced the bugs in this audit.

1. **All jobs cache reads/writes go through `jobQueryKeys` and `isJobListKey`.** The `lookupStatus` bug existed because one caller hand-wrote `['jobs', 'list']` while the writer used `['jobs', filters, limit]` — neither was wrong in isolation, but they didn't match. Rule: never inline `['jobs', ...]` arrays. If you need to scan all list caches, use `predicate: (q) => isJobListKey(q.queryKey)`.

2. **Mutation hooks must capture `priorStatus` (or any other "before" value) inside `onMutate`, not inside `mutationFn`.** `mutationFn` runs after `onMutate`, so by then your optimistic patch has already overwritten the cache. Pass the captured value through the mutation context and read it in `onSuccess`/`onError`.

3. **Track row-level focus by stable id, not by array index.** TanStack Query returns a new array reference on every refetch — index-keyed state silently jumps back to 0. Same rule applies to selection, hover state, and any other "follows a specific row" state.

4. **Selection state (and anything else that references job ids) must be pruned when the visible job list changes.** Use the prev-state-during-render pattern, not `useEffect`. Phantom ids are a silent destructive-action bug waiting to happen.

5. **No `useState` + `useEffect` for derived state.** The React Compiler lint rule `react-hooks/set-state-in-effect` catches this. Use the prev-prop-as-state pattern from the React docs. Two of the lint errors in this audit came from missing this rule.

6. **Global keyboard shortcuts must be scoped against popovers / inputs.** `enableOnFormTags: false` only catches `<input>`/`<textarea>`/`<select>` — Radix triggers are `<button>`, and arrow keys leak through. Use `isInPopover()` or a similar `document.activeElement.closest('[role="..."]')` guard.

7. **Bulk operations must throttle concurrency.** `Promise.allSettled(ids.map(fn))` is the wrong pattern at any non-trivial N; it produces 429 toast spam and uneven backend load. Use `runWithLimit(items, 5, fn)` or a similar small in-house helper.

8. **Mutations that affect user-job state must invalidate jobs + dashboard + insights.** Dashboard tabs aggregate `user_job_state` and stale silently. Use the `invalidateUserData()` helper.

9. **Every interactive element needs a visible focus indicator.** `outline-none` without a `focus-visible:ring-*` replacement is a WCAG failure.

10. **Every `<input type="checkbox">` and every form field needs an accessible name.** Placeholder text is not a label. Wrap with `<label htmlFor>` or set `aria-label`.

## Tests to keep green

The audit fixes are protected by these regression tests. Coverage of these specific assertions IS the spec — if any of them break, the bug is back.

- [src/features/jobs/hooks/**tests**/useFocusedJob.test.ts](../../../frontend/src/features/jobs/hooks/__tests__/useFocusedJob.test.ts)
  - "PRESERVES focus across a TanStack refetch (same ids, new array reference)" — the focus-reset bug.
  - "CLAMPS focus to the same index when the focused job is removed" — the dismiss-cursor-jumps bug.
  - "resetKey change forces focus back to the first item" — deliberate context-change semantics.

- [src/features/jobs/hooks/**tests**/useJobActions.test.ts](../../../frontend/src/features/jobs/hooks/__tests__/useJobActions.test.ts)
  - "REGRESSION: undo reverts to the correct prior status when the cache is populated" — the `lookupStatus` query-key mismatch.
  - "throttles bulk mutations to at most 5 concurrent" — the 429-storm guard.
  - "caps the undo stack at 20 entries" — the unbounded-history guard.
  - "invalidates dashboard + insights along with jobs on mutation" — the stale-dashboard guard.
  - "optimistically patches the list cache before the mutation resolves" — the no-feedback-during-mutation gap.
  - "rolls back optimistic patch when the mutation fails" — the no-rollback gap.
  - "shows a 'Nothing to undo' warning when history is empty" — the silent-no-op gap.

- [src/features/jobs/hooks/**tests**/useJobKeyboard.test.tsx](../../../frontend/src/features/jobs/hooks/__tests__/useJobKeyboard.test.tsx)
  - "Cmd+A no longer hijacks 'select all' — text selection stays native".
  - "ArrowLeft / ArrowRight are suppressed inside a Radix popover".
  - "shift+/ (cross-layout fallback) also opens help".

- [src/features/jobs/components/**tests**/DismissModal.test.tsx](../../../frontend/src/features/jobs/components/__tests__/DismissModal.test.tsx)
  - "wraps options in a role='radiogroup' with role='radio' + aria-checked".
  - "arrow keys move selection across radios".
  - "emits { reason, note } with the auto-filled label when the note is empty".
  - "user-typed note overrides the auto-filled label".
  - bulk-mode title + Dismiss button text reflect the count.

- [src/features/jobs/components/**tests**/JobDetailPanel.test.tsx](../../../frontend/src/features/jobs/components/__tests__/JobDetailPanel.test.tsx)
  - "exposes the panel via role='region' with aria-label 'Job details'".
  - "renders StatusSelect in route mode (so Interviewing / Offer / Rejected are reachable)".
  - "saves on blur via setJobNotes".
  - "applies a safe-area-inset-bottom padding in route mode".

- [src/pages/**tests**/JobsPage.integration.test.tsx](../../../frontend/src/pages/__tests__/JobsPage.integration.test.tsx)
  - "REGRESSION: clicking row 3 then pressing `s` saves row 3, not row 0" — **the headline user-reported bug**.
  - "REGRESSION: clicking bulk dismiss opens the modal (no longer silent)".
  - "REGRESSION: bulk save and mark-applied buttons exist in the bulk bar".
  - "REGRESSION: bulk action bar is a live region (role=status)".
  - "REGRESSION: keyboard rows expose accessible names for screen readers".

## Suggested follow-ups (not in scope of this PR)

- **ESLint rule: `no-inline-job-query-key`** — flag any `useQuery({queryKey: ['jobs', ...]})` or `getQueriesData({queryKey: ['jobs', ...]})` that doesn't come from `jobQueryKeys`. Tooling-level enforcement of rule #1 above.
- **ESLint rule: `mutation-must-invalidate`** — flag any `useMutation` whose `onSuccess` doesn't call `invalidateUserData()` or `queryClient.invalidateQueries`. Catches forgetting #8.
- **Backend migration** to support more distinct dismiss reasons — today we work around by encoding the visible label in the optional `note`. Long-term we should add `bad_location`, `too_junior`, `compensation_low`, `company_fit`, `other` as distinct server values so analytics can group cleanly.
- **Per-job dismiss reasons in bulk path** — `bulkDismissJobs(ids)` accepts only ids; we can't thread `reason` per-job. Switch to per-id calls with the `runWithLimit` throttle so the analytics signal isn't lost on bulk.
- **Tablet (640–1023 px) treatment** — currently the desktop split-panel layout is cramped on portrait iPad.
- **axe-core integration tests** — add an `@axe-core/react` snapshot test on `JobsPage`, `JobDetailPanel`, `DismissModal`. Would catch ARIA regressions automatically.
- **3-persona Playwright suite** — codify Alex / Sam / Jordan walkthroughs from the audit as end-to-end tests against a seeded dev instance.

## Test + lint status

```
$ npx vitest run                # 476 / 476 tests passing
$ npx tsc --noEmit              # 0 errors
$ npm run lint                  # 0 errors, 0 warnings
```
