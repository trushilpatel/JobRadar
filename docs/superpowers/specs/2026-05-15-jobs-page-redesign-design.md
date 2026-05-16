# Jobs Page — Keyboard-First Power List + Responsive Layout

**Date:** 2026-05-15
**Status:** Design — implementation plan to follow

## Problem

The current jobs page is a generic admin table with limited keyboard support and no mobile story.

1. **Keyboard shortcuts only fire after a mouse click.** [`useJobShortcuts`](../../../ui/console/src/features/jobs/hooks/useJobShortcuts.ts) requires `activeJob` (set by clicking a row in [`JobsPage.tsx`](../../../ui/console/src/pages/JobsPage.tsx#L185)). Pure-keyboard navigation is impossible. The shortcut tooltip (`JobsPage.tsx:116`) literally instructs the user to "click a row first."
2. **Listed shortcut `↑/↓ Move selection` is not implemented.** [`useJobShortcuts.ts:21-22`](../../../ui/console/src/features/jobs/hooks/useJobShortcuts.ts#L21-L22) advertises it; the handler at `:49-65` only handles `s/a/i/o/r/d`.
3. **No URL state.** Tab, active job, filters, and pagination all live in `useState`. Refresh wipes context; share-links don't work; back-button does the wrong thing.
4. **Limited bulk actions.** Only "bulk dismiss" exists ([`JobsPage.tsx:39`](../../../ui/console/src/pages/JobsPage.tsx#L39)). No bulk save, no bulk applied.
5. **No discoverability.** Shortcut help is buried in a tooltip on a small icon. No command palette. No `?` overlay.
6. **Not mobile-usable.** The split list+preview is desktop-only; tabs and filter row both overflow `<640px`.
7. **No undo.** Destructive actions (`d`, bulk dismiss) commit immediately with no soft-undo window.

## Goals

- Pure keyboard navigation across the full feed without ever touching the mouse.
- Discoverable shortcuts: command palette + `?` shortcut sheet.
- Responsive layout that's at least readable on mobile (tap to read, tap to open).
- Foundation that supports a v2 mobile-triage experience (swipe gestures) without architectural rework.
- Zero functional regressions — all current actions, filters, and tabs preserved.

## Non-goals (v1)

- Mobile swipe-to-save / swipe-to-dismiss gestures (v2; framer-motion + per-row hook).
- Long-press selection mode on touch (v2).
- Kanban / pipeline board view (deferred).
- One-at-a-time triage mode (deferred).

---

## Workflow model

**Power list + persistent preview** (Linear-style). Dense list on the left, resizable preview pane on the right (current layout, refined). Every interaction gets a key.

Same model maps cleanly to mobile by collapsing the split into a list-only view; tapping a row pushes the detail as a new route.

## Responsive strategy

**Single codebase, breakpoint-conditional.** No separate mobile build, no device detection — Tailwind `md:` (≥768px) and `lg:` (≥1024px) breakpoints + a couple of media-query hooks where layout _behavior_ differs (preview pane vs. push-route).

| Breakpoint   | Layout                                                                                                 |
| ------------ | ------------------------------------------------------------------------------------------------------ |
| `<640px`     | Single pane (list). Tap row → push `/jobs/:id`. Tabs collapse to dropdown. Filters open in a `Drawer`. |
| `640–1023px` | Split view, narrower preview. Tabs scroll horizontally. Filter row inline.                             |
| `≥1024px`    | Full split view as designed (current).                                                                 |

## Library decisions

| Library                   | Status                                                                                                            | Purpose                                                                                                                  |
| ------------------------- | ----------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| `shadcn/ui`               | **adopt formally** — run `npx shadcn@latest init`, generate `components.json`, point at existing `components/ui/` | Foundation for `Drawer`, `Sheet`, `Command`, `Tabs`, `Dialog`, `AlertDialog`, `ToggleGroup`, `Kbd`                       |
| `nuqs`                    | **add**                                                                                                           | URL state for `tab`, active `job`, all filters, pagination                                                               |
| `react-hotkeys-hook`      | **add**                                                                                                           | Replaces manual `useEffect`+`keydown` in `useJobShortcuts`; supports scoped shortcuts and chord sequences (`g g`, `g n`) |
| `cmdk`                    | **add via shadcn `command` component**                                                                            | Powers the `⌘K` palette                                                                                                  |
| `vaul`                    | **add via shadcn `drawer` component**                                                                             | Mobile bottom-sheet for filters/tabs                                                                                     |
| `sonner`                  | already installed — currently unused                                                                              | Undo toasts for destructive actions                                                                                      |
| `@tanstack/react-table`   | already installed                                                                                                 | Existing table; preserved                                                                                                |
| `@tanstack/react-virtual` | **defer**                                                                                                         | Only needed once a single page exceeds ~200 rows; pagination handles current scale                                       |
| `framer-motion`           | **defer to v2**                                                                                                   | Only needed for mobile swipe gestures                                                                                    |

No CSS framework change. No replacement of existing primitives unless touched by this redesign.

---

## URL state schema (`nuqs`)

All state that affects what the user sees moves into the URL. Local-only state (e.g. preview-pane width, transient UI hover) stays in `useState`.

| Param       | Type                                                             | Default | Notes                                                        |
| ----------- | ---------------------------------------------------------------- | ------- | ------------------------------------------------------------ |
| `tab`       | enum (`new`/`saved`/`applied`/`interviewing`/`offer`/`rejected`) | `new`   | One source of truth; replaces `JobsPage` local state         |
| `job`       | string (job ID)                                                  | `null`  | Drives preview pane on desktop; pushes `/jobs/:id` on mobile |
| `q`         | string                                                           | `''`    | Search query                                                 |
| `companies` | string[]                                                         | `[]`    | Multi-select                                                 |
| `loc`       | enum (`any`/`remote`/`onsite`)                                   | `any`   |                                                              |
| `exp`       | string[] (`0-2`/`2-5`/`5+`)                                      | `[]`    |                                                              |
| `since`     | enum (`all`/`24h`/`7d`/`30d`)                                    | `all`   |                                                              |
| `sort`      | string + `dir`                                                   | none    | Existing TanStack Table sorting state                        |
| `cursor`    | string                                                           | `''`    | Existing cursor pagination                                   |
| `limit`     | number                                                           | `20`    |                                                              |

Selection (`selectedIds`) and focused-row index stay in local state — both are transient and shouldn't survive refresh. The "focused row" (driven by `j`/`k`) is distinct from the "open preview job" (URL `job` param): focus moves with arrow/`j`/`k` and is what status keys act on; pressing `⏎` promotes the focused row into the open preview.

---

## Layout

### Desktop (≥1024px)

```
┌─ Topbar ──────────────────────────────────────────────┐
│ Jobs    [Tabs with counts]    filter-active   ⌘K  ?  │
├─ Filter row ──────────────────────────────────────────┤
│ 🔍 search    Companies ▾  | 0-2 2-5 5+ | 24h 7d 30d… │
├─ Body ────────────────────────────┬───────────────────┤
│  □ Stripe · Sr Eng    Remote  2d  │  Anthropic        │
│  ▶ Anthropic · Eng    SF      1d  │  Engineer, Inf.   │
│  ☑ Linear · PM        Remote  3d  │  [Save] [Applied] │
│  ☑ Vercel · DX        NYC     4d  │  Summary…         │
│    Figma · Eng        Remote  5d  │  Tech stack…      │
├─ Footer ──────────────────────────┴───────────────────┤
│ 2 selected · ⇧D bulk dismiss      1–7 of 142  ← →    │
└───────────────────────────────────────────────────────┘
```

### Mobile (<640px)

- Topbar collapses to title + tab dropdown (`Select`)
- Filter row → `Drawer` (vaul); trigger is a single `⚙` button with active-filter count badge
- Body is list-only; tap row → push `/jobs/:id`
- Detail route has sticky bottom action bar (Save / Applied / Dismiss in thumb zone)

---

## Full keyboard map

Implemented via `react-hotkeys-hook` with two scopes: **list** (default) and **detail** (when preview is focused / detail route is active).

### Navigation

| Key       | Action                                                   |
| --------- | -------------------------------------------------------- |
| `j` / `↓` | Move focus to next row                                   |
| `k` / `↑` | Move focus to previous row                               |
| `⏎`       | Open preview (desktop) / push `/jobs/:id` route (mobile) |
| `⌘⏎`      | Open posting URL in a new tab                            |
| `esc`     | Close preview / clear selection / exit search            |
| `←` / `→` | Previous / next page                                     |
| `g g`     | Jump to top of list                                      |
| `⇧G`      | Jump to bottom of list                                   |

### Tabs / views

| Key         | Action             |
| ----------- | ------------------ |
| `g n`       | Go to New          |
| `g s`       | Go to Saved        |
| `g a`       | Go to Applied      |
| `g i`       | Go to Interviewing |
| `g o`       | Go to Offer        |
| `g r`       | Go to Rejected     |
| `⌥1` … `⌥6` | Tab by index       |

### Status actions (on focused row)

| Key | Action                                               |
| --- | ---------------------------------------------------- |
| `s` | Save                                                 |
| `a` | Applied                                              |
| `i` | Interviewing                                         |
| `o` | Offer                                                |
| `r` | Rejected                                             |
| `d` | Dismiss (opens reason modal)                         |
| `u` | Undo last status change (also a button on the toast) |

### Selection / bulk

| Key  | Action                                 |
| ---- | -------------------------------------- |
| `x`  | Toggle row selection                   |
| `⇧X` | Range select (last selected → focused) |
| `⌘A` | Select all on page                     |
| `⇧S` | Bulk save                              |
| `⇧A` | Bulk applied                           |
| `⇧D` | Bulk dismiss                           |

### Search / palette / help

| Key   | Action                                                              |
| ----- | ------------------------------------------------------------------- |
| `/`   | Focus search input                                                  |
| `⌘K`  | Open command palette (any action by name; jump to job; jump to tab) |
| `⌘⇧R` | Reset filters                                                       |
| `?`   | Open shortcut help sheet (shadcn `Sheet`)                           |

Shortcuts are disabled when typing in inputs (existing `isTypingTarget` check) and when modals are open (existing `enabled` flag, plus scoped shortcuts via `react-hotkeys-hook`).

---

## Undo

Every destructive or status-changing action surfaces a `sonner` toast with an "Undo" button (5-second window). Pressing `u` invokes the same handler. Undo replays the prior status; for `dismiss` it restores the row and clears the dismissal reason.

---

## Files touched

### New / added

| File                                                             | Purpose                                                                           |
| ---------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| `ui/console/components.json`                                     | shadcn config                                                                     |
| `ui/console/src/components/ui/drawer.tsx`                        | shadcn `Drawer` (vaul)                                                            |
| `ui/console/src/components/ui/sheet.tsx`                         | shadcn `Sheet` (Radix Dialog variant)                                             |
| `ui/console/src/components/ui/command.tsx`                       | shadcn `Command` (cmdk)                                                           |
| `ui/console/src/components/ui/tabs.tsx`                          | shadcn `Tabs` (Radix) — replaces `JobStatusTabs` internals                        |
| `ui/console/src/components/ui/dialog.tsx`                        | shadcn `Dialog` — used by `DismissModal` rewrite                                  |
| `ui/console/src/components/ui/alert-dialog.tsx`                  | shadcn `AlertDialog`                                                              |
| `ui/console/src/components/ui/toggle-group.tsx`                  | shadcn `ToggleGroup` — exp / date / location pills                                |
| `ui/console/src/components/ui/kbd.tsx`                           | Keyboard key display                                                              |
| `ui/console/src/features/jobs/components/JobCommandPalette.tsx`  | `⌘K` palette (commands + recent jobs)                                             |
| `ui/console/src/features/jobs/components/JobShortcutSheet.tsx`   | `?` help overlay                                                                  |
| `ui/console/src/features/jobs/components/MobileFilterDrawer.tsx` | Mobile-only filter sheet                                                          |
| `ui/console/src/features/jobs/hooks/useJobUrlState.ts`           | nuqs-backed URL state, replaces local state in `useJobs`                          |
| `ui/console/src/features/jobs/hooks/useJobKeyboard.ts`           | All shortcut bindings via `react-hotkeys-hook`; replaces `useJobShortcuts`        |
| `ui/console/src/features/jobs/hooks/useFocusedJob.ts`            | Focused-row tracking (independent of `activeJob`/preview-open)                    |
| `ui/console/src/lib/use-media-query.ts`                          | Tiny hook for breakpoint-conditional behavior (e.g., preview pane vs. route push) |
| `ui/console/src/lib/use-undo-toast.ts`                           | Wraps `sonner` for status undo                                                    |

### Refactored

| File                                                         | Change                                                                                                                                                                 |
| ------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ui/console/src/pages/JobsPage.tsx`                          | Driven by `useJobUrlState` + `useFocusedJob`; renders preview via shadcn `Sheet` (mobile) or inline (desktop)                                                          |
| `ui/console/src/features/jobs/hooks/useJobs.ts`              | Reads filters from URL state via `useJobUrlState`; no longer owns filter `useState`                                                                                    |
| `ui/console/src/features/jobs/components/JobsTable.tsx`      | Adds `data-row-index` + ref for focused-row scroll-into-view; row click toggles open instead of merely "active"                                                        |
| `ui/console/src/features/jobs/components/JobDetailPanel.tsx` | Receives `mode: 'pane' \| 'route'` for layout differences; mobile route variant adds sticky action bar                                                                 |
| `ui/console/src/features/jobs/components/JobStatusTabs.tsx`  | Internals re-implemented on shadcn `Tabs`; mobile breakpoint swaps to `Select` dropdown                                                                                |
| `ui/console/src/features/jobs/components/JobFilters.tsx`     | Above `md:` — inline pill bar (current). Below `md:` — single trigger button + `MobileFilterDrawer`                                                                    |
| `ui/console/src/features/jobs/components/DismissModal.tsx`   | Re-built on shadcn `Dialog` (current is bespoke)                                                                                                                       |
| `ui/console/src/features/jobs/hooks/useJobActions.ts`        | Wraps mutations with `useUndoToast`; exposes `undoLast`                                                                                                                |
| `ui/console/src/App.tsx`                                     | New route `/jobs/:id` for mobile detail (same component as preview, `mode='route'`)                                                                                    |
| `ui/console/src/main.tsx`                                    | Wrap with `<NuqsAdapter>`                                                                                                                                              |
| `ui/console/package.json`                                    | Add `nuqs`, `react-hotkeys-hook`. shadcn-added components bring `cmdk`, `vaul`, `@radix-ui/react-tabs`, `@radix-ui/react-toggle-group`, `@radix-ui/react-alert-dialog` |

### Deleted

| File                                                    | Replaced by         |
| ------------------------------------------------------- | ------------------- |
| `ui/console/src/features/jobs/hooks/useJobShortcuts.ts` | `useJobKeyboard.ts` |

---

## Out of scope (explicit)

- **Swipe gestures on mobile** — v2; only adds `framer-motion` + a wrapper around `JobsTable` rows, no architecture change.
- **Long-press selection on mobile** — v2; tiny custom hook.
- **Kanban / board view** — separate feature.
- **One-at-a-time triage mode** — separate feature; would live as a `?triage=1` mode on the same page.
- **Backend changes** — none. All API contracts unchanged.

## Risks

1. **URL state migration is the riskiest change.** Filters/tab/job all moving to nuqs means anywhere code reads them must be updated atomically. Mitigated by keeping `useJobs` as the single read point and changing only its internals.
2. **shadcn `Tabs` re-implementation** must preserve the exact visual styling of `JobStatusTabs` (counts, active state colors). Snapshot test or visual diff before merging.
3. **`g g`, `g n` chord sequences** require careful scope handling so they don't fire while typing in the search input. `react-hotkeys-hook`'s `enableOnFormTags: false` (default) handles this, but verify.
4. **Mobile detail route** must restore scroll position when navigating back. React Router 7's default scroll restoration covers this; verify on real device.

## Acceptance criteria

- All keyboard shortcuts in the map work without ever touching the mouse, including from a fresh page load (no "click a row first" precondition).
- Refreshing the page preserves tab, filters, active job, and pagination.
- A direct link to `/jobs?tab=saved&job=abc-123` opens the saved tab with job `abc-123` in the preview.
- Mobile (`<640px`) renders one pane; tapping a row pushes a detail route; back returns to the list at the same scroll position.
- `?` opens a discoverable shortcut sheet listing every key.
- `⌘K` opens a palette that can run any action by name and jump to any tab.
- Every status change and dismiss surfaces an undo toast for 5s; pressing `u` undoes.
- Bulk save (`⇧S`), bulk applied (`⇧A`), and bulk dismiss (`⇧D`) all work on the current selection.
- No regressions in the existing test suite for `features/jobs/`.
