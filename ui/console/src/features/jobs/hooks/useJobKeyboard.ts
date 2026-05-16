import { useHotkeys } from 'react-hotkeys-hook'
import type { JobStatus } from '@/types'
import type { TabKey } from '@/constants/jobStatus'

type SettableStatus = Exclude<JobStatus, 'dismissed' | 'new'>

export interface UseJobKeyboardArgs {
  /** Master toggle — when false, no shortcuts fire. */
  enabled?: boolean

  // ── Navigation ─────────────────────────────────────────────────────────────
  onFocusNext: () => void
  onFocusPrev: () => void
  onFocusFirst: () => void
  onFocusLast: () => void
  onOpenPreview: () => void
  onOpenUrl: () => void
  onEscape: () => void
  onPrevPage: () => void
  onNextPage: () => void

  // ── Tabs ───────────────────────────────────────────────────────────────────
  onGoToTab: (tab: TabKey) => void

  // ── Status / dismiss / undo ────────────────────────────────────────────────
  /**
   * Note: plain `r` maps to status=rejected; this is intentional Vim-style
   * mnemonic alongside s/a/i/o. To reset filters, use Cmd/Ctrl+Shift+R.
   */
  onSetStatus: (status: SettableStatus) => void
  onDismiss: () => void
  onUndo: () => void

  // ── Selection / bulk ───────────────────────────────────────────────────────
  onToggleSelect: () => void
  onRangeSelect: () => void
  /**
   * Optional — no keyboard binding fires this. It was previously bound to
   * Cmd/Ctrl+A but that hijacked the browser's native "select all text",
   * making it impossible to copy text from the detail panel. Callers that
   * still want a "select all rows" affordance should wire it to a button
   * (the header checkbox already does).
   */
  onSelectAll?: () => void
  onBulkSave: () => void
  onBulkApplied: () => void
  onBulkDismiss: () => void

  // ── Search / palette / help ────────────────────────────────────────────────
  onFocusSearch: () => void
  onOpenPalette: () => void
  onResetFilters: () => void
  onOpenHelp: () => void
}

// Common options applied to every binding. react-hotkeys-hook ignores inputs
// by default (enableOnFormTags defaults to []), so typing in the search field
// won't fire shortcuts. We opt into preventDefault to keep the browser from
// e.g. quick-finding on `/`.
const BASE_OPTS = {
  preventDefault: true,
  enableOnFormTags: false,
  enableOnContentEditable: false,
} as const

/**
 * Returns true when the active element is inside a Radix-style popover
 * (Select listbox, Menu, Combobox, Dialog) or a contenteditable region. We
 * suppress global navigation shortcuts (ArrowLeft / ArrowRight for page nav)
 * in those contexts so the popover's own keyboard navigation isn't hijacked.
 *
 * `enableOnFormTags: false` only filters `<input>` / `<textarea>` / `<select>`,
 * which doesn't cover Radix's `<button>` triggers — hence this extra guard.
 */
function isInPopover(): boolean {
  const el = document.activeElement
  if (!el || el === document.body) return false
  const sel =
    '[role="listbox"], [role="menu"], [role="combobox"], [role="dialog"], [contenteditable], [data-radix-popper-content-wrapper]'
  return !!(el as Element).closest?.(sel)
}

/**
 * Centralised keyboard map for the jobs page.
 *
 * Callbacks are injected so this hook doesn't reach into URL state, the
 * focused-job hook, or mutations. The page owns the wiring; this hook just
 * binds keys.
 *
 * Notes on a few less-obvious decisions:
 * - There's no `Cmd/Ctrl+A` binding any more — the old one prevented the
 *   browser's text select-all everywhere on the page, making it impossible
 *   to copy text from the detail panel. Use `x` + `shift+x` for keyboard
 *   selection.
 * - `/` won't refocus the search input if the user is already typing in it
 *   (`enableOnFormTags: false`). To re-arm the shortcut, press Escape first
 *   to leave the input.
 * - `?` and `shift+/` both open help — both forms exist because `?` on
 *   AZERTY / Windows layouts often requires AltGr; `shift+/` always
 *   produces the same physical key.
 */
export function useJobKeyboard(args: UseJobKeyboardArgs) {
  const enabled = args.enabled ?? true
  const opts = { ...BASE_OPTS, enabled }
  // For bindings that must additionally skip when focus is inside a popover.
  const safeOpts = { ...BASE_OPTS, enabled: () => enabled && !isInPopover() }

  // ── Navigation ───────────────────────────────────────────────────────────
  useHotkeys('j, down', () => args.onFocusNext(), opts, [args.onFocusNext])
  useHotkeys('k, up', () => args.onFocusPrev(), opts, [args.onFocusPrev])
  useHotkeys('enter', () => args.onOpenPreview(), opts, [args.onOpenPreview])
  useHotkeys('meta+enter, ctrl+enter', () => args.onOpenUrl(), opts, [args.onOpenUrl])
  useHotkeys('escape', () => args.onEscape(), opts, [args.onEscape])
  // Page nav is scoped via safeOpts so pressing ← / → inside a Radix Select
  // (status dropdown) does NOT close the popover + advance the page.
  useHotkeys('left', () => args.onPrevPage(), safeOpts, [args.onPrevPage])
  useHotkeys('right', () => args.onNextPage(), safeOpts, [args.onNextPage])

  // ── Chord navigation ─────────────────────────────────────────────────────
  // `g g` jump-to-top, `shift+g` (G) jump-to-bottom.
  useHotkeys('g>g', () => args.onFocusFirst(), opts, [args.onFocusFirst])
  useHotkeys('shift+g', () => args.onFocusLast(), opts, [args.onFocusLast])

  // ── Tab jumps via `g <key>` chord ────────────────────────────────────────
  useHotkeys('g>n', () => args.onGoToTab('new'), opts, [args.onGoToTab])
  useHotkeys('g>s', () => args.onGoToTab('saved'), opts, [args.onGoToTab])
  useHotkeys('g>a', () => args.onGoToTab('applied'), opts, [args.onGoToTab])
  useHotkeys('g>i', () => args.onGoToTab('interviewing'), opts, [args.onGoToTab])
  useHotkeys('g>o', () => args.onGoToTab('offer'), opts, [args.onGoToTab])
  useHotkeys('g>r', () => args.onGoToTab('rejected'), opts, [args.onGoToTab])

  // ── Status keys on focused row ───────────────────────────────────────────
  useHotkeys('s', () => args.onSetStatus('saved'), opts, [args.onSetStatus])
  useHotkeys('a', () => args.onSetStatus('applied'), opts, [args.onSetStatus])
  useHotkeys('i', () => args.onSetStatus('interviewing'), opts, [args.onSetStatus])
  useHotkeys('o', () => args.onSetStatus('offer'), opts, [args.onSetStatus])
  useHotkeys('r', () => args.onSetStatus('rejected'), opts, [args.onSetStatus])
  useHotkeys('d', () => args.onDismiss(), opts, [args.onDismiss])
  useHotkeys('u', () => args.onUndo(), opts, [args.onUndo])

  // ── Selection / bulk ─────────────────────────────────────────────────────
  // Note: no `meta+a, ctrl+a` binding — see hook JSDoc.
  useHotkeys('x', () => args.onToggleSelect(), opts, [args.onToggleSelect])
  useHotkeys('shift+x', () => args.onRangeSelect(), opts, [args.onRangeSelect])
  useHotkeys('shift+s', () => args.onBulkSave(), opts, [args.onBulkSave])
  useHotkeys('shift+a', () => args.onBulkApplied(), opts, [args.onBulkApplied])
  useHotkeys('shift+d', () => args.onBulkDismiss(), opts, [args.onBulkDismiss])

  // ── Search / palette / help ──────────────────────────────────────────────
  // `/` and `?` rely on `useKey: true` so react-hotkeys-hook matches `e.key`
  // directly instead of `e.code` (which for both is "Slash").
  useHotkeys('/', () => args.onFocusSearch(), { ...opts, useKey: true }, [args.onFocusSearch])
  useHotkeys('meta+k, ctrl+k', () => args.onOpenPalette(), opts, [args.onOpenPalette])
  useHotkeys('meta+shift+r, ctrl+shift+r', () => args.onResetFilters(), opts, [args.onResetFilters])
  useHotkeys('?', () => args.onOpenHelp(), { ...opts, useKey: true }, [args.onOpenHelp])
  // Cross-layout fallback — physical Slash + Shift always reaches the help
  // sheet even on layouts where `?` would require AltGr.
  useHotkeys('shift+slash', () => args.onOpenHelp(), opts, [args.onOpenHelp])
}
