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
  onSetStatus: (status: SettableStatus) => void
  onDismiss: () => void
  onUndo: () => void

  // ── Selection / bulk ───────────────────────────────────────────────────────
  onToggleSelect: () => void
  onRangeSelect: () => void
  onSelectAll: () => void
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
 * Centralised keyboard map for the jobs page.
 *
 * Callbacks are injected so this hook doesn't reach into URL state, the
 * focused-job hook, or mutations. The page owns the wiring; this hook just
 * binds keys.
 *
 * Disable the entire hook via `enabled: false` when a modal/palette is open
 * so shortcuts don't conflict with the overlay's own keys.
 */
export function useJobKeyboard(args: UseJobKeyboardArgs) {
  const enabled = args.enabled ?? true
  const opts = { ...BASE_OPTS, enabled }

  // ── Navigation ───────────────────────────────────────────────────────────
  useHotkeys('j, down', () => args.onFocusNext(), opts, [args.onFocusNext])
  useHotkeys('k, up', () => args.onFocusPrev(), opts, [args.onFocusPrev])
  useHotkeys('enter', () => args.onOpenPreview(), opts, [args.onOpenPreview])
  useHotkeys('meta+enter, ctrl+enter', () => args.onOpenUrl(), opts, [args.onOpenUrl])
  useHotkeys('escape', () => args.onEscape(), opts, [args.onEscape])
  useHotkeys('left', () => args.onPrevPage(), opts, [args.onPrevPage])
  useHotkeys('right', () => args.onNextPage(), opts, [args.onNextPage])

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
  useHotkeys('x', () => args.onToggleSelect(), opts, [args.onToggleSelect])
  useHotkeys('shift+x', () => args.onRangeSelect(), opts, [args.onRangeSelect])
  useHotkeys('meta+a, ctrl+a', () => args.onSelectAll(), opts, [args.onSelectAll])
  useHotkeys('shift+s', () => args.onBulkSave(), opts, [args.onBulkSave])
  useHotkeys('shift+a', () => args.onBulkApplied(), opts, [args.onBulkApplied])
  useHotkeys('shift+d', () => args.onBulkDismiss(), opts, [args.onBulkDismiss])

  // ── Search / palette / help ──────────────────────────────────────────────
  // `/` and `?` rely on `useKey: true` so react-hotkeys-hook matches `e.key`
  // directly instead of `e.code` (which for both is "Slash"). This also keeps
  // the help-key binding distinct from the search-focus binding regardless of
  // shift state.
  useHotkeys('/', () => args.onFocusSearch(), { ...opts, useKey: true }, [args.onFocusSearch])
  useHotkeys('meta+k, ctrl+k', () => args.onOpenPalette(), opts, [args.onOpenPalette])
  useHotkeys('meta+shift+r, ctrl+shift+r', () => args.onResetFilters(), opts, [args.onResetFilters])
  useHotkeys('?', () => args.onOpenHelp(), { ...opts, useKey: true }, [args.onOpenHelp])
}
