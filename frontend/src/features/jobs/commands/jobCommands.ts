import type { CommandHandlers, CommandRegistry } from '@/lib/commands/types'
import type { JobStatus } from '@/types'
import type { TabKey } from '../hooks/useJobs'

type SettableStatus = Exclude<JobStatus, 'dismissed' | 'new' | 'saved'>

/**
 * The single source of truth for every jobs-page command: id → label + key
 * bindings + grouping. `as const satisfies CommandRegistry` preserves literal
 * keys for the derived id union while shape-checking every entry. Tab-jump
 * labels are inlined — no cross-import into copy.
 */
export const JOB_COMMANDS = {
  // ── Navigation ─────────────────────────────────────────────────────────
  'jobs.nav.next': { label: 'Next job', keys: ['j', 'down'], category: 'navigation' },
  'jobs.nav.prev': { label: 'Previous job', keys: ['k', 'up'], category: 'navigation' },
  'jobs.nav.open-preview': { label: 'Open preview', keys: ['enter'], category: 'navigation' },
  'jobs.nav.open-url': {
    label: 'Open URL in new tab',
    keys: ['mod+enter'],
    category: 'navigation',
  },
  'jobs.nav.escape': { label: 'Close / clear', keys: ['escape'], category: 'navigation' },
  'jobs.nav.first-row': { label: 'Jump to top', keys: ['g>g'], category: 'navigation' },
  'jobs.nav.last-row': { label: 'Jump to bottom', keys: ['shift+g'], category: 'navigation' },

  // ── Tabs ───────────────────────────────────────────────────────────────
  'jobs.tab.go-new': { label: 'Go to New', keys: ['g>n'], category: 'tabs' },
  'jobs.tab.go-saved': { label: 'Go to Saved', keys: ['g>s'], category: 'tabs' },
  'jobs.tab.go-applied': { label: 'Go to Applied', keys: ['g>a'], category: 'tabs' },
  'jobs.tab.go-interviewing': { label: 'Go to Interviewing', keys: ['g>i'], category: 'tabs' },
  'jobs.tab.go-offer': { label: 'Go to Offer', keys: ['g>o'], category: 'tabs' },
  'jobs.tab.go-rejected': { label: 'Go to Rejected', keys: ['g>r'], category: 'tabs' },

  // ── Status / save / undo ─────────────────────────────────────────────────
  'jobs.save': { label: 'Save / unsave', keys: ['s'], category: 'status' },
  'jobs.apply': { label: 'Applied', keys: ['a'], category: 'status' },
  'jobs.interviewing': { label: 'Interviewing', keys: ['i'], category: 'status' },
  'jobs.offer': { label: 'Offer', keys: ['o'], category: 'status' },
  'jobs.rejected': { label: 'Rejected', keys: ['r'], category: 'status' },
  'jobs.undo': { label: 'Undo last change', keys: ['u'], category: 'status' },

  // ── Search / palette / help ──────────────────────────────────────────────
  'jobs.nav.focus-search': {
    label: 'Search',
    keys: ['/'],
    category: 'search',
    useKey: true,
  },
  'jobs.nav.open-palette': { label: 'Command palette', keys: ['mod+k'], category: 'search' },
  'jobs.nav.reset-filters': {
    label: 'Reset filters',
    keys: ['mod+shift+r'],
    category: 'search',
  },
  'jobs.nav.open-help': {
    label: 'Keyboard shortcuts',
    keys: ['?', 'shift+slash'],
    category: 'search',
    useKey: true,
  },
} as const satisfies CommandRegistry

/** Derived id union — never hand-written. Adding a row grows this. */
export type JobCommandId = keyof typeof JOB_COMMANDS

/**
 * The typed bag of page callbacks the handlers invoke.
 */
export interface JobsCallbacks {
  onFocusNext: () => void
  onFocusPrev: () => void
  onFocusFirst: () => void
  onFocusLast: () => void
  onOpenPreview: () => void
  onOpenUrl: () => void
  onEscape: () => void
  onGoToTab: (tab: TabKey) => void
  onSetStatus: (status: SettableStatus) => void
  onToggleSave: () => void
  onUndo: () => void
  onFocusSearch: () => void
  onOpenPalette: () => void
  onResetFilters: () => void
  onOpenHelp: () => void
}

/**
 * Builds the exhaustive id→handler map. The `CommandHandlers<JobCommandId>`
 * return type means omitting any command is a compile error.
 */
export function jobCommandHandlers(cb: JobsCallbacks): CommandHandlers<JobCommandId> {
  return {
    'jobs.nav.next': () => cb.onFocusNext(),
    'jobs.nav.prev': () => cb.onFocusPrev(),
    'jobs.nav.open-preview': () => cb.onOpenPreview(),
    'jobs.nav.open-url': () => cb.onOpenUrl(),
    'jobs.nav.escape': () => cb.onEscape(),
    'jobs.nav.first-row': () => cb.onFocusFirst(),
    'jobs.nav.last-row': () => cb.onFocusLast(),
    'jobs.tab.go-new': () => cb.onGoToTab('new'),
    'jobs.tab.go-saved': () => cb.onGoToTab('saved'),
    'jobs.tab.go-applied': () => cb.onGoToTab('applied'),
    'jobs.tab.go-interviewing': () => cb.onGoToTab('interviewing'),
    'jobs.tab.go-offer': () => cb.onGoToTab('offer'),
    'jobs.tab.go-rejected': () => cb.onGoToTab('rejected'),
    'jobs.save': () => cb.onToggleSave(),
    'jobs.apply': () => cb.onSetStatus('applied'),
    'jobs.interviewing': () => cb.onSetStatus('interviewing'),
    'jobs.offer': () => cb.onSetStatus('offer'),
    'jobs.rejected': () => cb.onSetStatus('rejected'),
    'jobs.undo': () => cb.onUndo(),
    'jobs.nav.focus-search': () => cb.onFocusSearch(),
    'jobs.nav.open-palette': () => cb.onOpenPalette(),
    'jobs.nav.reset-filters': () => cb.onResetFilters(),
    'jobs.nav.open-help': () => cb.onOpenHelp(),
  }
}
