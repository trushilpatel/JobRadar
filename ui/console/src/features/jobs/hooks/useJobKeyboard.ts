import { useCallback, useRef } from 'react'
import type { JobStatus } from '@/types'
import type { TabKey } from '@/config/constants/jobStatus'
import { useCommandKeyboard } from '@/lib/commands/useCommandKeyboard'
import { JOB_COMMANDS, jobCommandHandlers, DOUBLE_PRESS_MS } from '../commands/jobCommands'

type SettableStatus = Exclude<JobStatus, 'dismissed' | 'new' | 'saved'>

/**
 * Page callbacks consumed by the jobs keyboard engine. Kept structurally
 * identical to the previous contract so the JobsPage call site is untouched —
 * the bindings themselves now live in `JOB_COMMANDS`.
 */
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
   * `onPrevPage`.
   */
  onFirstPage?: () => void

  // ── Tabs ───────────────────────────────────────────────────────────────────
  onGoToTab: (tab: TabKey) => void

  // ── Status / save / dismiss / undo ─────────────────────────────────────────
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

/**
 * Thin wrapper over the generic command engine. All bindings are declared in
 * `JOB_COMMANDS`; this hook only adapts the page callback bag and owns the
 * `lastLeftRef` the double-press-left quirk needs.
 */
export function useJobKeyboard(args: UseJobKeyboardArgs) {
  // Ref holding the last `←` timestamp — only read/written inside the callback
  // below (a user-event handler), never during render.
  const lastLeftRef = useRef(0)

  // Wraps onPrevPage + onFirstPage with the double-press-left timing quirk so
  // the logic stays in this hook and the ref never crosses render boundaries.
  const onPrevPageOrFirst = useCallback(() => {
    const now = performance.now()
    const elapsed = now - lastLeftRef.current
    if (elapsed < DOUBLE_PRESS_MS && args.onFirstPage) {
      args.onFirstPage()
      lastLeftRef.current = 0
    } else {
      args.onPrevPage()
      lastLeftRef.current = now
    }
  }, [args.onPrevPage, args.onFirstPage]) // eslint-disable-line react-hooks/exhaustive-deps

  useCommandKeyboard(
    JOB_COMMANDS,
    // eslint-disable-next-line react-hooks/refs -- onPrevPageOrFirst only reads lastLeftRef.current inside an event handler, never during render
    jobCommandHandlers({ ...args, onPrevPage: onPrevPageOrFirst }),
    { enabled: args.enabled ?? true }
  )
}
