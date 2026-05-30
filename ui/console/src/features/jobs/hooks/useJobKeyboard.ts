import { useRef } from 'react'
import { useHotkeys } from 'react-hotkeys-hook'
import type { JobStatus } from '@/types'
import type { TabKey } from '@/config/constants/jobStatus'
import { JOBS_TOKENS } from '@/config/tokens/jobs.tokens'

type SettableStatus = Exclude<JobStatus, 'dismissed' | 'new' | 'saved'>

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
  /**
   * Optional — when provided, a fast double-press of `←` (within 200ms)
   * fires this instead of a second `onPrevPage`. Single press always fires
   * `onPrevPage`. See implementation note in the hook body.
   */
  onFirstPage?: () => void

  // ── Tabs ───────────────────────────────────────────────────────────────────
  onGoToTab: (tab: TabKey) => void

  // ── Status / save / dismiss / undo ─────────────────────────────────────────
  /**
   * Workflow-status keys (a/i/o/r). To reset filters, use Cmd/Ctrl+Shift+R.
   */
  onSetStatus: (status: SettableStatus) => void
  /** Toggle the orthogonal saved flag on the focused row (bound to `s`). */
  onToggleSave: () => void
  onDismiss: () => void
  onUndo: () => void

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
 * Multi-select shortcuts (x, shift+x, shift+s, shift+a, shift+d) were removed
 * along with the multi-select / bulk-action UI. To act on multiple jobs,
 * users now operate on the focused row one at a time.
 */
// Window during which two consecutive `←` presses are treated as
// "jump to first page" instead of two consecutive `prev` actions.
const DOUBLE_PRESS_MS = 200

export function useJobKeyboard(args: UseJobKeyboardArgs) {
  const enabled = args.enabled ?? true
  const opts = { ...BASE_OPTS, enabled }
  // For bindings that must additionally skip when focus is inside a popover.
  const safeOpts = { ...BASE_OPTS, enabled: () => enabled && !isInPopover() }

  // ── Navigation ───────────────────────────────────────────────────────────
  // Primary key from token + secondary alias (down/up) for accessibility
  useHotkeys(`${JOBS_TOKENS['jobs.nav.next'].shortcut}, down`, () => args.onFocusNext(), opts, [
    args.onFocusNext,
  ])
  useHotkeys(`${JOBS_TOKENS['jobs.nav.prev'].shortcut}, up`, () => args.onFocusPrev(), opts, [
    args.onFocusPrev,
  ])
  useHotkeys(JOBS_TOKENS['jobs.nav.open-preview'].shortcut, () => args.onOpenPreview(), opts, [
    args.onOpenPreview,
  ])
  useHotkeys('meta+enter, ctrl+enter', () => args.onOpenUrl(), opts, [args.onOpenUrl])
  useHotkeys(JOBS_TOKENS['jobs.nav.escape'].shortcut, () => args.onEscape(), opts, [args.onEscape])

  // `←` — single press: prev page. Double press within 200ms: jump to first
  // page (if `onFirstPage` is wired). No debounce — single press fires
  // instantly. The intermediate state (page N-1) is replaced by page 1 fast
  // enough that the user sees only the final state.
  const lastLeftRef = useRef(0)
  useHotkeys(
    JOBS_TOKENS['jobs.nav.prev-page'].shortcut,
    () => {
      const now = performance.now()
      const elapsed = now - lastLeftRef.current
      if (elapsed < DOUBLE_PRESS_MS && args.onFirstPage) {
        args.onFirstPage()
        lastLeftRef.current = 0 // reset so a 3rd press is treated as new prev
      } else {
        args.onPrevPage()
        lastLeftRef.current = now
      }
    },
    safeOpts,
    [args.onPrevPage, args.onFirstPage]
  )
  useHotkeys(JOBS_TOKENS['jobs.nav.next-page'].shortcut, () => args.onNextPage(), safeOpts, [
    args.onNextPage,
  ])

  // ── Chord navigation ─────────────────────────────────────────────────────
  useHotkeys(JOBS_TOKENS['jobs.nav.first-row'].shortcut, () => args.onFocusFirst(), opts, [
    args.onFocusFirst,
  ])
  useHotkeys(JOBS_TOKENS['jobs.nav.last-row'].shortcut, () => args.onFocusLast(), opts, [
    args.onFocusLast,
  ])

  // ── Tab jumps via `g <key>` chord ────────────────────────────────────────
  useHotkeys(JOBS_TOKENS['jobs.tab.go-new'].shortcut, () => args.onGoToTab('new'), opts, [
    args.onGoToTab,
  ])
  useHotkeys(JOBS_TOKENS['jobs.tab.go-saved'].shortcut, () => args.onGoToTab('saved'), opts, [
    args.onGoToTab,
  ])
  useHotkeys(JOBS_TOKENS['jobs.tab.go-applied'].shortcut, () => args.onGoToTab('applied'), opts, [
    args.onGoToTab,
  ])
  useHotkeys(
    JOBS_TOKENS['jobs.tab.go-interviewing'].shortcut,
    () => args.onGoToTab('interviewing'),
    opts,
    [args.onGoToTab]
  )
  useHotkeys(JOBS_TOKENS['jobs.tab.go-offer'].shortcut, () => args.onGoToTab('offer'), opts, [
    args.onGoToTab,
  ])
  useHotkeys(JOBS_TOKENS['jobs.tab.go-rejected'].shortcut, () => args.onGoToTab('rejected'), opts, [
    args.onGoToTab,
  ])

  // ── Status keys on focused row ───────────────────────────────────────────
  // `s` is special: it toggles the orthogonal saved flag, not a workflow status.
  useHotkeys(JOBS_TOKENS['jobs.save'].shortcut, () => args.onToggleSave(), opts, [
    args.onToggleSave,
  ])
  useHotkeys(JOBS_TOKENS['jobs.apply'].shortcut, () => args.onSetStatus('applied'), opts, [
    args.onSetStatus,
  ])
  useHotkeys(
    JOBS_TOKENS['jobs.interviewing'].shortcut,
    () => args.onSetStatus('interviewing'),
    opts,
    [args.onSetStatus]
  )
  useHotkeys(JOBS_TOKENS['jobs.offer'].shortcut, () => args.onSetStatus('offer'), opts, [
    args.onSetStatus,
  ])
  useHotkeys(JOBS_TOKENS['jobs.rejected'].shortcut, () => args.onSetStatus('rejected'), opts, [
    args.onSetStatus,
  ])
  useHotkeys(JOBS_TOKENS['jobs.dismiss'].shortcut, () => args.onDismiss(), opts, [args.onDismiss])
  useHotkeys(JOBS_TOKENS['jobs.undo'].shortcut, () => args.onUndo(), opts, [args.onUndo])

  // ── Search / palette / help ──────────────────────────────────────────────
  useHotkeys(
    JOBS_TOKENS['jobs.nav.focus-search'].shortcut,
    () => args.onFocusSearch(),
    { ...opts, useKey: true },
    [args.onFocusSearch]
  )
  useHotkeys('meta+k, ctrl+k', () => args.onOpenPalette(), opts, [args.onOpenPalette])
  useHotkeys('meta+shift+r, ctrl+shift+r', () => args.onResetFilters(), opts, [args.onResetFilters])
  useHotkeys(
    JOBS_TOKENS['jobs.nav.open-help'].shortcut,
    () => args.onOpenHelp(),
    { ...opts, useKey: true },
    [args.onOpenHelp]
  )
  // Cross-layout fallback — physical Slash + Shift always reaches the help
  // sheet even on layouts where `?` would require AltGr.
  useHotkeys('shift+slash', () => args.onOpenHelp(), opts, [args.onOpenHelp])
}
