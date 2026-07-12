import type { JobStatus } from '@/types'
import type { TabKey } from '@/config/constants/jobStatus'
import { useCommandKeyboard } from '@/lib/commands/useCommandKeyboard'
import { JOB_COMMANDS, jobCommandHandlers } from '../commands/jobCommands'

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

  // ── Tabs ───────────────────────────────────────────────────────────────────
  onGoToTab: (tab: TabKey) => void

  // ── Status / save / undo ────────────────────────────────────────────────────
  onSetStatus: (status: SettableStatus) => void
  /** Toggle the orthogonal saved flag on the focused row (bound to `s`). */
  onToggleSave: () => void
  onUndo: () => void

  // ── Search / palette / help ────────────────────────────────────────────────
  onFocusSearch: () => void
  onOpenPalette: () => void
  onResetFilters: () => void
  onOpenHelp: () => void
}

/**
 * Thin wrapper over the generic command engine. All bindings are declared in
 * `JOB_COMMANDS`; this hook just adapts the page callback bag.
 */
export function useJobKeyboard(args: UseJobKeyboardArgs) {
  useCommandKeyboard(JOB_COMMANDS, jobCommandHandlers(args), { enabled: args.enabled ?? true })
}
