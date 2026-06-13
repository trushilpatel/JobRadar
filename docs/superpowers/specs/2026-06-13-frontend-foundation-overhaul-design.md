# Frontend Foundation Overhaul

**Date:** 2026-06-13  
**Branch:** `feat/frontend-foundation`  
**Status:** Complete

## Problem

Three compounding issues were discovered during investigation of a reported pagination bug (users bounced to page 1 when clicking Next repeatedly):

1. **Wrong abstraction for cursor pagination.** `useInfiniteQuery` is designed for infinite scroll — it accumulates all pages. The jobs page uses it for page-by-page navigation with a manual `pageIndex` URL param. The mismatch requires an "eager advance" pattern (`setPageIndex(N+1)` + `fetchNextPage()` fired simultaneously from two different async systems). This creates a race: there is at least one render cycle where `pageIndex` has advanced but `isFetchingNextPage` is still `false`, causing `isLoadingPage` to evaluate `false`, `itemsOnPage = 0`, and the `Pagination` component's H7 auto-reset to fire `onFirst()` → page 1.

2. **Business logic in a UI component.** `pagination.tsx` contains a `useEffect` (H7) that calls `onFirst()` based on `itemsOnPage`, `hasPrev`, and `loading`. A generic display component making navigation decisions is a design smell that makes tests incomplete and bugs hard to locate.

3. **Inconsistent component origins.** `components/ui/` mixes shadcn/ui components, custom Radix wrappers, and fully hand-rolled components (notably `multi-select-dropdown.tsx` — 196 lines, 9 effects, raw `createPortal`). This creates maintenance burden, a11y gaps, and inconsistent patterns for a multi-developer team.

## Architecture Decisions

### Pagination: `useQuery` + cursor history stack

Replace `useInfiniteQuery` with `useQuery`. A `cursorHistory: string[]` array tracks the cursor for each visited page. `pageIndex` in the URL (nuqs) indexes into this array.

```
cursorHistory = ['', 'cursor-abc', 'cursor-xyz']
pageIndex     = 2   (stored in URL)

currentCursor = cursorHistory[pageIndex]  →  'cursor-xyz'
useQuery({ queryKey: [...filters, currentCursor, limit], queryFn: fetchJobs(..., currentCursor) })
```

**Navigation:**

- **Next:** if `cursorHistory[pageIndex + 1]` exists, `setPageIndex(pageIndex + 1)`. Otherwise append `data.next_cursor` to history first, then advance. No fetch+advance race — the URL update and the cursor push are atomic in one `setState` call.
- **Prev:** `setPageIndex(pageIndex - 1)`. Cursor already in history → TanStack Query cache hit.
- **Filter/tab/sort/limit change:** `setCursorHistory([''])` + `setPageIndex(0)`.
- **Refresh:** `cursorHistory` is in `sessionStorage` keyed by a fingerprint of active filters. `pageIndex` comes from the URL. On load: restore history from sessionStorage if fingerprint matches, otherwise start fresh from page 1.

**Why this eliminates the race:**  
There is no eager advance. `setPageIndex` and cursor history are updated together in a single synchronous state update. The query for the new page fires from the new cursor. Loading state is just `query.isLoading || query.isFetching` — no multi-system coordination.

### `Pagination` component: pure display

Remove the H7 `useEffect`. `Pagination` accepts props and renders — no side effects, no internal navigation decisions. All auto-reset and loading-state logic lives in `useJobs`.

### shadcn/ui: formal adoption

`components.json` already exists and shadcn is initialized. The adoption completes what is already started.

**Decision tree for every new UI component (mandatory for all developers):**

1. Does shadcn/ui have it? → `npx shadcn@latest add <name>`, done.
2. Does Radix UI have a headless primitive? → wrap it with Tailwind + `cn()`.
3. Neither? → pure custom component (no `useEffect` with side effects). PR description must explain why shadcn/Radix does not cover the use case.

**Components being replaced in this sprint:**

- `multi-select-dropdown.tsx` → `Popover + Command + Checkbox` (shadcn pattern)
- `status-pill.tsx` → `Badge` variants (shadcn `badge.tsx` already present)
- `segmented-switch.tsx` → `ToggleGroup` (shadcn `toggle-group.tsx` already present)
- `kpi-card.tsx` → `Card` (to be added via CLI)

**Components staying custom (no shadcn equivalent):**

- `pagination.tsx` — cursor pagination; shadcn has no cursor mode
- `data-table.tsx` — TanStack Table integration; stays as-is
- `resize-handle.tsx` — drag handle; no shadcn equivalent
- `dismiss-stripe.tsx` — domain-specific toast stripe; no shadcn equivalent
- `scan-history-bar.tsx` — domain visualisation; no shadcn equivalent

## New Rules for CLAUDE.md

````
### UI component boundary (components/ui/)

Every component in `components/ui/` is either:
  a. shadcn/ui origin — added via `npx shadcn@latest add <name>` and only
     lightly customised for this project's tokens/colours. Never diverge
     structurally from the shadcn source.
  b. Pure custom — wraps a Radix primitive or is hand-built. MUST satisfy:
       - No `useEffect` with side effects (no navigation callbacks, no parent
         mutations, no auto-resets)
       - No business logic (no "if itemsOnPage === 0 call onFirst()")
       - No internal state that affects parent behaviour
       - Data in → render out → event callbacks out. That is all.

Any component that does not satisfy (a) or (b) is a design error. Fix the
seam: move the logic to the hook or service that owns it.

### New component decision tree (mandatory)

1. Does shadcn/ui have it? → `npx shadcn@latest add <name>`. Stop.
2. Does Radix UI have a headless primitive? → thin styled wrapper with
   Tailwind + cn(). Stop.
3. Neither? → write a pure custom component. The PR description MUST include
   one sentence explaining why shadcn and Radix do not cover this case.

### Forbidden patterns

- `createPortal` directly in a component — use Radix `Portal` or shadcn
  `Popover`/`Dialog` which already handle portalling correctly.
- `useInfiniteQuery` for page-by-page cursor navigation — it is designed for
  infinite scroll (append). Use `useQuery` + an explicit cursor history array.
  See `features/jobs/hooks/useJobs.ts` for the canonical pattern.
- Multi-select / combobox / filterable dropdown built from scratch — always
  compose from shadcn `Popover + Command + Checkbox`. A from-scratch dropdown
  will have broken keyboard navigation and missing ARIA roles.
- Business logic (`loading` decisions, page resets, auto-snap) in `components/ui/`
  components. That belongs in the hook (`useJobs`, `useCompanies`, etc.).

### Cursor pagination (canonical pattern)

```ts
// hooks/useXxx.ts
const [cursorHistory, setCursorHistory] = useState<string[]>([''])
const currentCursor = cursorHistory[pageIndex] ?? ''

const query = useQuery({
  queryKey: [...filters, currentCursor, limit],
  queryFn: () => fetchXxx(filters, currentCursor, limit),
})

function goToNextPage() {
  const nextCursor = query.data?.next_cursor
  if (!nextCursor) return
  const newHistory = cursorHistory.slice(0, pageIndex + 1).concat(nextCursor)
  setCursorHistory(newHistory)
  void setPageIndex(pageIndex + 1)
}

function goToPrevPage() {
  if (pageIndex > 0) void setPageIndex(pageIndex - 1)
}

function resetCursor() {
  setCursorHistory([''])
  void setPageIndex(0)
}
````

````

## Implementation Phases

Each phase = one commit on `feat/frontend-foundation`. All gates must pass
before committing: frontend lint, TypeScript check, Vitest coverage ≥ 90%
per changed file.

---

### Phase 1 — CLAUDE.md strict rules ✅

**Files:** `CLAUDE.md`, `frontend/CLAUDE.md`
**Tests:** None (docs only)
**Commit:** `docs: add strict frontend component and pagination rules`

Add the rules from the section above verbatim. This is the contract all
subsequent phases implement against.

---

### Phase 2 — Add missing shadcn primitives ✅

**Action:** `npx shadcn@latest add card input label separator skeleton scroll-area popover checkbox`
**Files:** New files in `src/components/ui/`
**Tests:** Smoke-render test for each added component (1 test each)
**Commit:** `feat(ui): add missing shadcn primitives (card, input, label, separator, skeleton, popover, checkbox)`

---

### Phase 3 — Replace multi-select-dropdown ✅

**Files:**
- DELETE `src/components/ui/multi-select-dropdown.tsx`
- CREATE `src/components/ui/multi-select.tsx` (Popover + Command + Checkbox)
- UPDATE all importers (JobFilters, any admin filters)
- UPDATE `src/components/ui/__tests__/multi-select.test.tsx`

**Must cover in tests:**
- Renders trigger with placeholder when nothing selected
- Renders trigger with count badge when N items selected
- Opens popover on click
- Renders all options
- Clicking an option calls onChange with updated selection
- Clicking a selected option deselects it
- Search filters the option list
- Keyboard: Enter selects, Escape closes
- Empty search state renders "No results"

**Commit:** `feat(ui): replace multi-select-dropdown with shadcn Popover+Command+Checkbox`

---

### Phase 4 — Replace domain display components ✅

**Files:**
- `src/components/ui/status-pill.tsx` → use `Badge` from `badge.tsx`
- `src/components/ui/segmented-switch.tsx` → use `ToggleGroup`
- `src/components/ui/kpi-card.tsx` → use `Card`
- Update all importers (no API surface change — same props where possible)
- Tests for each updated component

**Must cover in tests:**
- Each variant renders correctly
- Interactive variants (ToggleGroup) fire onChange
- Snapshot-free: assert on rendered text and ARIA roles

**Commit:** `feat(ui): replace status-pill, segmented-switch, kpi-card with shadcn equivalents`

---

### Phase 5 — Pure Pagination component ✅

**Files:**
- `src/components/ui/pagination.tsx` — remove H7 `useEffect` entirely
- `src/components/ui/__tests__/pagination.test.tsx` — add tests for previously untested states

**Design:**
- Remove the `useEffect` block from `CursorPagination` (lines 92–96)
- `onFirst` remains a prop — the hook decides when to call it, not the component
- The component becomes: given these props, render these buttons with these disabled states

**Must cover in tests (add to existing file):**
- Does NOT call `onFirst` when `itemsOnPage === 0, hasPrev=true, loading=false` (regression for H7 removal)
- `loading=true` disables both nav buttons
- Range text renders correctly for all states: loading, no results, N–M, N–M of total

**Commit:** `fix(ui): make Pagination a pure display component, remove H7 auto-reset side effect`

---

### Phase 6 — Rewrite useJobs: useQuery + cursor history ✅

**Files:**
- `src/features/jobs/hooks/useJobs.ts` — full rewrite of pagination logic
- `src/features/jobs/hooks/useJobUrlState.ts` — no changes needed (pageIndex stays in URL)
- `src/features/jobs/hooks/__tests__/useJobs.ts` — update all pagination tests
- `src/features/jobs/hooks/__tests__/useJobs.pagination.test.ts` — rewrite for new pattern
- `JobsPage.tsx` — update `loading` prop derivation (remove `isLoadingPage` reference)

**New state in `useJobs`:**
```ts
// cursorHistory in sessionStorage, keyed by filter fingerprint
// pageIndex from URL (nuqs, unchanged)
// currentCursor = cursorHistory[pageIndex] ?? ''
// query = useQuery({ queryKey: [...filters, currentCursor, limit], queryFn: ... })
// goToNextPage: extend history + setPageIndex
// goToPrevPage: setPageIndex(pageIndex - 1)  [cache hit]
// resetCursor: called by setTab, setCompanies, setSort, setLimit, etc.
````

**Removed from public surface:**

- `isFetchingNextPage` (no longer exists)
- `isLoadingPage` (no longer needed — `query.isLoading` is sufficient)

**Must cover in tests:**

- Initial load: isLoading true → false, jobs populated
- goToNextPage: cursor appended to history, pageIndex advances, new query fires
- goToNextPage at last page (no next_cursor): is a no-op
- goToPrevPage: pageIndex decrements, cursor from history used (cache hit scenario)
- goToPrevPage on page 1: is a no-op
- Filter change: cursorHistory resets to [''], pageIndex resets to 0
- Tab change: same as filter change
- Sort change: same as filter change
- Limit change: same as filter change
- sessionStorage restore: matching fingerprint restores cursorHistory
- sessionStorage mismatch: fresh start from page 1
- goToFirstPage: resets to page 1
- itemsBeforePage: sums items from earlier pages in cursorHistory
- **Regression: no auto-snap to page 1 when clicking Next rapidly** (the race that was the original bug)

**Commit:** `feat(jobs): replace useInfiniteQuery with useQuery + cursor history, eliminate pagination race`

---

## Success Criteria

- [x] Pagination race (snap to page 1 on rapid Next) is eliminated
- [x] All tests pass (1126 tests across 95 files)
- [x] New tests added per phase bring per-file coverage to ≥ 90%
- [x] `multi-select-dropdown.tsx` is deleted (0 references)
- [x] `Pagination` component has zero `useEffect` calls
- [x] `useJobs` has zero `useInfiniteQuery` references
- [x] `CLAUDE.md` contains the new rules and every subsequent PR can be judged against them
- [x] Frontend lint clean: `cd frontend && npm run lint`
- [x] TypeScript strict-mode clean: `cd frontend && npx tsc --noEmit`

## Decisions Not Made Here (out of scope)

- E2E tests (Playwright) — deferred; foundational unit/integration layer first
- `data-table.tsx` refactor — works correctly today; not in scope
- `dismiss-stripe.tsx` refactor — domain-specific, no shadcn equivalent
- Admin page component audit — follow-up sprint once this foundation is in
