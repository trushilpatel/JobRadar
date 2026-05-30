# Command Registry Redesign — Design

Date: 2026-05-30
Supersedes: `2026-05-29-ui-copy-shortcut-registry-design.md`

## Problem

The current token system conflates three distinct concerns in one object shape
(`{ copy, shortcut }`) and fails to be the single source of truth it claims to be:

1. **Naming.** `copy` is an awkward field name for on-screen text. It should read
   as the action's name.
2. **Split source of truth.** The keyboard hook
   (`features/jobs/hooks/useJobKeyboard.ts`) hardcodes many bindings as string
   literals that exist in no token: `meta+enter`/`ctrl+enter`, `meta+k`/`ctrl+k`,
   `meta+shift+r`/`ctrl+shift+r`, `shift+slash`, and `down`/`up` aliases baked
   into template strings.
3. **Drifted surfaces.** `JobShortcutSheet.tsx` hand-maintains shortcut lists that
   are now wrong — it still advertises removed multi-select shortcuts
   (`x`, `shift+x`, `cmd+a`, `shift+s/a/d`) and tab shortcuts that do not exist
   (`opt+1`..`opt+6`). `JobCommandPalette.tsx` re-hardcodes `g n`, `cmd+shift+r`,
   `?`. None of these derive from the tokens.
4. **Preferences 500.** `PATCH /me/config` returns
   `{"error":{"code":"INTERNAL"}}` — the keyboard-shortcut preference cannot be
   saved.

## Goals

- One typed **command registry** per feature: the single source for an action's
  id, label (on-screen text), and key bindings.
- Every surface — keyboard engine, command palette, help sheet, action buttons —
  **derives** from the registry. Drift becomes impossible.
- Adding a command is a **one-file, two-adjacent-line** edit; forgetting to wire a
  handler is a **compile error**.
- Cross-platform correct by construction via a `mod` key abstraction.
- Fix the preferences 500 by diagnosis, not guesswork.
- Redesign the Account page to a real settings page whose shortcut reference
  derives from the registry.

## Non-goals

- User-remappable keybindings UI and persistence. The engine is built so the
  remap seam exists at zero extra cost (single key-resolution point), but no
  preference field, persistence, or remap UI ships here.
- i18n / localization of labels.
- Turning sidebar/nav items into palette commands (the generic engine allows it
  later; not done now).

## Core model

A **command** is the unit. Static, declarative half (`CommandDef`) carries the
on-screen text and keys; the runtime half is a handler function. Both halves live
together in one feature file, keyed by a derived id union, so they cannot drift.

### Types — `lib/commands/types.ts`

- `KeySpec` — a `react-hotkeys-hook` key string. Supports `mod+` (expanded to
  `meta` on Mac, `ctrl` elsewhere by the engine), chords (`g>g`), and aliases.
- `CommandCategory` — grouping for palette/help (`'navigation' | 'tabs' |
'status' | 'search'`). Optional; ungrouped commands render under a default
  section.
- `CommandScope` — `'global'` (fires anywhere except form inputs) or `'list'`
  (also suppressed inside popovers/dialogs, e.g. page arrows). Optional, default
  `'global'`.
- `CommandDef` — `{ label: string; keys: KeySpec[]; category?: CommandCategory;
scope?: CommandScope }`. `keys: []` means palette-only (no keyboard trigger).
- `CommandRegistry` — `Record<string, CommandDef>`.
- `CommandHandlers<Id extends string>` — `Record<Id, (e: KeyboardEvent) => void>`.

The `label` field replaces `copy`. Most commands are just `{ label, keys }`.

### Feature registry + handlers — `features/jobs/commands/jobCommands.ts`

One file holds three exports, adjacent:

- `JOB_COMMANDS` — `… as const satisfies CommandRegistry`. The `as const`
  preserves literal keys; `satisfies` shape-checks every entry. Tab-jump labels
  are **inlined** (`label: 'Go to New'`) — no cross-import into copy.
- `JobCommandId` — `keyof typeof JOB_COMMANDS`. **Derived**, never hand-written.
  Adding a registry row grows the union automatically.
- `jobCommandHandlers(cb: JobsCallbacks): CommandHandlers<JobCommandId>` — the
  exhaustive id→handler map. Because the return type is `Record<JobCommandId,
…>`, omitting a handler for any command is a **compile error**. `JobsCallbacks`
  is the typed bag of page callbacks (replaces today's `UseJobKeyboardArgs`
  callback fields).

All bindings currently hardcoded in `useJobKeyboard.ts` (open-url, palette,
reset-filters, help fallback, nav aliases) move into `JOB_COMMANDS` as `keys`
entries (e.g. `keys: ['mod+enter']`, `keys: ['mod+k']`, `keys: ['mod+shift+r']`,
help `keys: ['?', 'shift+slash']`, next `keys: ['j', 'down']`).

### Generic engine — `lib/commands/useCommandKeyboard.ts`

```
useCommandKeyboard<Id>(registry, handlers, opts?: { enabled?: boolean }): void
```

- A pure helper `buildDispatch(registry, overrides = {})` centralizes ALL
  cross-cutting logic in one place: `mod` expansion, chord normalization, alias
  collapsing (e.g. `j` and `down` map to the same id), and construction of a
  `Map<normalizedKey, Id>` plus per-scope combined key strings. The
  `overrides = {}` parameter is the free remap seam — the only key-resolution
  point in the system.
- The hook calls `useHotkeys` a **fixed** number of times (one per scope bucket:
  `global`, `list`), not in a loop — this respects rules-of-hooks. Each
  registration dispatches the fired key through the `Map` to `handlers[id]`.
- Memoized on `registry` so dispatch structures rebuild only when the registry
  changes.

### Dev-time validation — folded into the engine path

`buildDispatch` (dev builds only) throws if two commands claim the same
normalized key within a scope, or a `KeySpec` is malformed. A unit test asserts
the registry is collision-free and that `jobCommandHandlers` covers every
`JobCommandId`. This permanently locks the SSOT — a clashing key fails the build.

### Display formatting — `lib/commands/formatKeys.ts`

`formatKeys(spec: KeySpec): string[]` renders a key spec into display glyphs,
platform-aware: `mod` → `⌘` on Mac, `Ctrl` on Windows/Linux; `enter` → `⏎`,
`shift` → `⇧`, chords split on `>`. Used by palette, help sheet, and button
hints via the existing `Kbd`/`KbdGroup` components.

## Surfaces derive from the registry

- **`useJobKeyboard.ts`** collapses to a thin wrapper:
  `useCommandKeyboard(JOB_COMMANDS, jobCommandHandlers(cb), { enabled })`. The
  double-press-left "jump to first page" quirk moves into the `jobs.page.prev`
  handler (state held in the handler closure / a ref passed via callbacks), not
  the engine. ~200 lines → ~6.
- **`JobCommandPalette.tsx`** iterates `JOB_COMMANDS` grouped by `category`,
  rendering label + `formatKeys`. Recent-jobs section unchanged. No hardcoded
  shortcut strings remain.
- **`JobShortcutSheet.tsx`** iterates `JOB_COMMANDS` grouped by `category`.
  Removes the stale `SELECTION_SHORTCUTS` and non-existent `opt+1..6` rows.
- **Action buttons** (existing `use-action` / button components) read label +
  keys from the registry and set `aria-keyshortcuts` from the binding.

## Copy split — `config/copy/`

Pure display strings that are NOT commands (sidebar/nav labels, tab titles,
search placeholder) move to a `config/copy/` map of `id → string` (Layer 0,
imports nothing). Job commands do NOT live in `config/` — they are feature-local
and live in `features/jobs/commands/`. `NAV_TOKENS` becomes nav copy; the
`config/tokens/` directory and `getActionTokens()` accessor are removed.

### Id naming convention

All command ids and copy ids use **dot-namespaced kebab segments** matching the
existing token style: `jobs.nav.next`, `jobs.tab.go-new`, `jobs.page.prev`. One
convention, enforced across the registry.

## Backend — preferences 500 fix

The Go code (`UserRepositoryService.PatchConfig`) reads correctly. Likely causes,
in order: (1) **migration 075 not applied** in the cluster (migrations doc states
"currently at 073"); (2) the JSONB upsert binds `[]byte` where Postgres wants an
explicit `?::jsonb` cast.

Process: reproduce the 500 against the cluster → verify/apply migration 075 via
`cd backend && go run ./apps/migrate` (per the supabase-local-access note) →
re-test PATCH. If still failing, add the `::jsonb` cast to the upsert SQL and the
INSERT param. Fix is confirmed by observing a successful PATCH and a persisted
row, not by code reading alone.

## Account page redesign — `pages/AccountPage.tsx`

Replace the bare prototype with a proper settings page:

- **Profile** section (existing display-name form, email) — unchanged behavior.
- **Keyboard shortcuts** section: the existing "show shortcut hints" toggle
  (wired to `useUserConfig`/`usePatchUserConfig`), plus a **reference table**
  derived from `JOB_COMMANDS` (label + `formatKeys`), grouped by category. This
  reuses the same registry the help sheet uses — one source.
- **Danger zone** (existing delete-account flow) — unchanged.

Sections stay wrapped in `ErrorBoundary`. Styling via `cn()` + Tailwind per
frontend rules.

## File layout

```
config/
  copy/index.ts                       nav/tab/placeholder copy (id → string)
lib/commands/
  types.ts                            CommandDef, scope, category, registry, handlers
  useCommandKeyboard.ts               generic engine + buildDispatch (+ dev validation)
  formatKeys.ts                       platform-aware display glyphs
features/jobs/
  commands/jobCommands.ts             JOB_COMMANDS + JobCommandId + jobCommandHandlers
  hooks/useJobKeyboard.ts             thin wrapper over the engine
  components/JobCommandPalette.tsx    derives from registry
  components/JobShortcutSheet.tsx     derives from registry
pages/AccountPage.tsx                 settings page w/ registry-derived reference
```

Removed: `config/tokens/` (jobs.tokens.ts, nav.tokens.ts, index.ts,
`getActionTokens`).

## Testing

- `lib/commands`: `buildDispatch` (mod expansion, chord/alias normalization,
  collision throw, malformed-key throw); `formatKeys` (mac vs non-mac glyphs);
  `useCommandKeyboard` (fires correct handler per key, respects `enabled` and
  scope suppression in popovers).
- `features/jobs/commands`: registry collision-free; `jobCommandHandlers` covers
  every `JobCommandId` (compile + runtime assertion); each handler invokes the
  right callback.
- `JobCommandPalette` / `JobShortcutSheet`: render rows from a stub registry;
  assert no stale/hardcoded entries; categories group correctly.
- `useJobKeyboard`: integration — pressing each key invokes the page callback.
- `AccountPage`: reference table renders registry rows; toggle calls
  `patchConfig`; loading/error states; delete flow unchanged.
- Backend: `UserRepositoryService` PatchConfig/GetConfig round-trip (existing
  tests), plus a regression test for the JSONB upsert if the cast is added.
- Coverage: frontend ≥90% per-file (console target); backend per-package ≥80%.

## Edge cases

### Key resolution & dispatch (`buildDispatch`)

- **Duplicate key across commands (same scope):** dev build throws with both ids
  named; prod build last-registered wins but a unit test prevents it ever
  shipping. `mod`-expanded keys count as the same key (`mod+k` vs `meta+k` on Mac
  collide).
- **Same key in different scopes:** allowed (e.g. a `global` `enter` and a `list`
  arrow never coexist in the same listener) — not a collision.
- **Alias keys mapping to one command:** `keys: ['j','down']` must register both
  and dispatch both to the same handler; the reverse-map is keyed per-alias.
- **Empty `keys: []`:** command is palette/help-only — never registered with the
  keyboard engine, still rendered in palette/help (with no key glyph).
- **Malformed `KeySpec`** (empty string, trailing `+`, unknown token): dev throws;
  caught by a format test.
- **Chord (`g>g`, `g>n`) vs single key `g`:** if both a chord prefix and a bare
  prefix key are registered, the bare key must not swallow the chord. Registry
  must not define a bare `g` command; a test asserts no command's single key is
  the prefix of another command's chord.
- **`mod` on non-Mac:** expands to `ctrl`; `meta+k` literal (if ever used)
  stays meta-only. Convention: always use `mod`, never raw `meta`/`ctrl`, enforced
  by a lint-style registry test.
- **Registry identity / memoization:** `JOB_COMMANDS` is a module-level const, so
  `buildDispatch` memo never rebuilds across renders; handlers closure changes do
  not require re-registering keys (handlers read via ref/latest pattern to avoid
  stale closures without re-running `useHotkeys`).

### Engine behavior (`useCommandKeyboard`)

- **`enabled=false`:** no handler fires for any key; toggling back to true
  re-arms without remount. Driven by JobsPage `keyboardEnabled`
  (`!isMobile && !paletteOpen && !helpOpen && !mobileFilterOpen`).
- **Typing in the search input:** `enableOnFormTags:false` keeps `s`,`d`,`j`,`/`
  etc. from firing while typing. `/` focuses search (uses `useKey:true` so it
  works across layouts); pressing `/` while already in search must NOT re-trigger
  (form-tag guard covers this).
- **Escape:** in search input, local `onKeyDown` blurs the field (unchanged);
  global `escape` command closes preview/clears focus. Both must continue to work
  — the search-local handler stops propagation only enough to blur.
- **Focus inside a Radix popover/dialog/select (`scope:'list'`):** page-arrow
  commands (`left`/`right`) suppressed via `isInPopover()` so the popover's own
  arrow nav wins; `global`-scope commands still fire. Preserve existing
  `isInPopover` selector list.
- **`preventDefault`:** kept for `/` (no browser quick-find) and arrows; must not
  break native behavior inside inputs (guarded by form-tag check).
- **Rules-of-hooks:** scope buckets are a fixed compile-time set, so the number
  of `useHotkeys` calls never varies between renders even if a registry were
  empty for a scope (empty combined string → `useHotkeys('')` is a safe no-op).

### Double-press-left "jump to first page"

- Single `left` → prev page; two `left` within 200ms → first page (only if a
  first-page callback exists). State (`lastLeftRef`) lives in the
  `jobs.page.prev` handler closure, not the engine. A third press resets to a new
  prev. Must not fire first-page when already on page 1.

### Display (`formatKeys`)

- Mac vs Windows/Linux glyph differences (`⌘` vs `Ctrl`, `⌥` vs `Alt`).
- Chord rendered as separate `Kbd` groups (`g` then `g`); alias arrays render
  only the **primary** key in buttons (first entry) but **all** in the help sheet.
- Unknown/raw single chars pass through uppercased where appropriate (`g`→`G`?
  decision: keep lowercase letters as typed, modifiers as glyphs).

### Surfaces

- **Palette:** category with zero commands renders no heading. Recent-jobs
  section independent of command groups. Selecting a command closes the palette
  and runs the handler (existing `runAndClose`).
- **Help sheet:** must show ONLY commands that exist in the registry — regression
  test asserts removed multi-select rows (`x`, `⇧X`, `⌘A`, `⇧S/A/D`) and
  non-existent `⌥1..6` rows are absent.
- **Buttons / `aria-keyshortcuts`:** a command with `keys:[]` renders no hint and
  no aria attribute. Hint shows primary key only.

### Backend / preferences

- **New user, no `user_preferences` row:** `GetConfig` returns zero-value config
  (existing behavior) — must not 500.
- **Migration 075 unapplied:** PATCH 500s — root cause to verify first.
- **JSONB merge:** PATCH of one field must not clobber other stored fields
  (`data || EXCLUDED.data`); round-trip test covers partial patch preserving
  siblings.
- **Concurrent PATCH:** upsert `ON CONFLICT` converges; last write wins per field.
- **Malformed body:** invalid JSON / wrong type → 400 (validation), not 500.

### Account page

- Config query loading → toggle disabled, reference table may still render
  (registry is static, not server data). Config error → toggle reflects default
  (false), table still renders. Toggle mutation error → existing toast, no crash.

## Test cases (explicit)

`lib/commands/__tests__/buildDispatch.test.ts`

1. `mod+k` expands to `meta+k` on Mac, `ctrl+k` on non-Mac.
2. Alias `['j','down']` produces two map entries → same id.
3. Duplicate key across two commands (same scope) throws in dev.
4. Same key in two different scopes does NOT throw.
5. Malformed key (`''`, `'mod+'`, `'g>'`) throws.
6. Bare key that is a chord prefix throws (guards `g` vs `g>g`).
7. `keys:[]` produces no map entry and no scope registration.

`lib/commands/__tests__/formatKeys.test.ts` 8. `mod+enter` → `['⌘','⏎']` on Mac, `['Ctrl','⏎']` elsewhere. 9. Chord `g>g` → two groups. 10. `shift+slash` and `?` render expected glyphs.

`lib/commands/__tests__/useCommandKeyboard.test.tsx` 11. Pressing a registered key invokes the matching handler. 12. `enabled:false` → no handler fires; re-enabling re-arms. 13. Typing in an input does not fire single-letter commands. 14. `list`-scope arrow suppressed when focus is in a popover; `global` still fires. 15. Latest handler invoked after callback identity changes (no stale closure).

`features/jobs/commands/__tests__/jobCommands.test.ts` 16. Registry is collision-free (runs `buildDispatch` without throwing). 17. `jobCommandHandlers` returns a handler for every `JobCommandId` (Object.keys
equality with `JOB_COMMANDS`). 18. Each handler invokes the expected callback (table-driven). 19. Every command id matches the dot-kebab naming convention (regex).

`features/jobs/hooks/__tests__/useJobKeyboard.test.tsx` 20. Each key press routes to the right page callback (integration). 21. Double-press `left` within 200ms → first page; single → prev; on page 1 no
first-page call.

`features/jobs/components/__tests__/JobCommandPalette.test.tsx` 22. Renders one row per command grouped by category, with `formatKeys` glyphs. 23. No hardcoded shortcut strings remain (snapshot/derived assertion). 24. Selecting a command runs handler and closes palette.

`features/jobs/components/__tests__/JobShortcutSheet.test.tsx` 25. Rows derive from registry; stale multi-select + `⌥1..6` rows are ABSENT. 26. Categories group correctly; empty category renders no heading.

`pages/__tests__/AccountPage.test.tsx` 27. Reference table renders registry rows. 28. Toggle calls `patchConfig`; loading disables toggle; error shows default. 29. Delete flow unchanged (existing coverage retained).

Backend `service/user/tests/UserRepositoryService_test.go` 30. PatchConfig round-trip persists and GetConfig returns it. 31. Partial patch preserves other JSONB fields. 32. GetConfig on missing row returns zero-value, no error. 33. (if cast added) regression: upsert with `::jsonb` succeeds.

Backend `service/user/tests/UserRestService_test.go` 34. PATCH `/me/config` valid body → 200 + persisted. 35. PATCH malformed body → 400, not 500.

## Migration / rollout

- Migration 075 must be applied before the preferences feature works; verify in
  cluster as part of the backend fix.
- Pure frontend refactor otherwise — no API contract change. `/me/config` shape
  is unchanged.
