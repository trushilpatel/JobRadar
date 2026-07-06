import { describe, it, expect, vi } from 'vitest'
import {
  JOB_COMMANDS,
  jobCommandHandlers,
  type JobCommandId,
  type JobsCallbacks,
} from '../jobCommands'
import { buildDispatch } from '@/lib/commands/buildDispatch'

function makeCallbacks(): JobsCallbacks {
  return {
    onFocusNext: vi.fn(),
    onFocusPrev: vi.fn(),
    onFocusFirst: vi.fn(),
    onFocusLast: vi.fn(),
    onOpenPreview: vi.fn(),
    onOpenUrl: vi.fn(),
    onEscape: vi.fn(),
    onGoToTab: vi.fn(),
    onSetStatus: vi.fn(),
    onToggleSave: vi.fn(),
    onDismiss: vi.fn(),
    onUndo: vi.fn(),
    onFocusSearch: vi.fn(),
    onOpenPalette: vi.fn(),
    onResetFilters: vi.fn(),
    onOpenHelp: vi.fn(),
  }
}

const evt = () => new KeyboardEvent('keydown')

describe('JOB_COMMANDS registry', () => {
  it('is collision-free (buildDispatch does not throw)', () => {
    expect(() => buildDispatch(JOB_COMMANDS)).not.toThrow()
  })

  it('contains exactly 24 commands', () => {
    expect(Object.keys(JOB_COMMANDS)).toHaveLength(24)
  })

  it('every command id matches the dot-kebab naming convention', () => {
    const re = /^[a-z]+(\.[a-z]+(-[a-z]+)*)+$/
    for (const id of Object.keys(JOB_COMMANDS)) {
      expect(re.test(id), `id "${id}" violates dot-kebab convention`).toBe(true)
    }
  })

  it('every command has a non-empty label', () => {
    for (const id of Object.keys(JOB_COMMANDS) as JobCommandId[]) {
      expect(JOB_COMMANDS[id].label.length).toBeGreaterThan(0)
    }
  })

  it('uses mod (never raw meta/ctrl) for cross-platform combos', () => {
    for (const id of Object.keys(JOB_COMMANDS) as JobCommandId[]) {
      for (const spec of JOB_COMMANDS[id].keys) {
        expect(spec.includes('meta+'), `${id} uses raw meta+`).toBe(false)
        expect(spec.includes('ctrl+'), `${id} uses raw ctrl+`).toBe(false)
      }
    }
  })
})

describe('jobCommandHandlers', () => {
  it('returns a handler for every JobCommandId', () => {
    const handlers = jobCommandHandlers(makeCallbacks())
    expect(Object.keys(handlers).sort()).toEqual(Object.keys(JOB_COMMANDS).sort())
  })

  const cases: { id: JobCommandId; assert: (cb: JobsCallbacks) => void }[] = [
    { id: 'jobs.nav.next', assert: (cb) => expect(cb.onFocusNext).toHaveBeenCalled() },
    { id: 'jobs.nav.prev', assert: (cb) => expect(cb.onFocusPrev).toHaveBeenCalled() },
    { id: 'jobs.nav.first-row', assert: (cb) => expect(cb.onFocusFirst).toHaveBeenCalled() },
    { id: 'jobs.nav.last-row', assert: (cb) => expect(cb.onFocusLast).toHaveBeenCalled() },
    { id: 'jobs.nav.open-preview', assert: (cb) => expect(cb.onOpenPreview).toHaveBeenCalled() },
    { id: 'jobs.nav.open-url', assert: (cb) => expect(cb.onOpenUrl).toHaveBeenCalled() },
    { id: 'jobs.nav.escape', assert: (cb) => expect(cb.onEscape).toHaveBeenCalled() },
    { id: 'jobs.tab.go-new', assert: (cb) => expect(cb.onGoToTab).toHaveBeenCalledWith('new') },
    { id: 'jobs.tab.go-saved', assert: (cb) => expect(cb.onGoToTab).toHaveBeenCalledWith('saved') },
    {
      id: 'jobs.tab.go-applied',
      assert: (cb) => expect(cb.onGoToTab).toHaveBeenCalledWith('applied'),
    },
    {
      id: 'jobs.tab.go-interviewing',
      assert: (cb) => expect(cb.onGoToTab).toHaveBeenCalledWith('interviewing'),
    },
    { id: 'jobs.tab.go-offer', assert: (cb) => expect(cb.onGoToTab).toHaveBeenCalledWith('offer') },
    {
      id: 'jobs.tab.go-rejected',
      assert: (cb) => expect(cb.onGoToTab).toHaveBeenCalledWith('rejected'),
    },
    { id: 'jobs.save', assert: (cb) => expect(cb.onToggleSave).toHaveBeenCalled() },
    { id: 'jobs.apply', assert: (cb) => expect(cb.onSetStatus).toHaveBeenCalledWith('applied') },
    {
      id: 'jobs.interviewing',
      assert: (cb) => expect(cb.onSetStatus).toHaveBeenCalledWith('interviewing'),
    },
    { id: 'jobs.offer', assert: (cb) => expect(cb.onSetStatus).toHaveBeenCalledWith('offer') },
    {
      id: 'jobs.rejected',
      assert: (cb) => expect(cb.onSetStatus).toHaveBeenCalledWith('rejected'),
    },
    { id: 'jobs.dismiss', assert: (cb) => expect(cb.onDismiss).toHaveBeenCalled() },
    { id: 'jobs.undo', assert: (cb) => expect(cb.onUndo).toHaveBeenCalled() },
    { id: 'jobs.nav.focus-search', assert: (cb) => expect(cb.onFocusSearch).toHaveBeenCalled() },
    { id: 'jobs.nav.open-palette', assert: (cb) => expect(cb.onOpenPalette).toHaveBeenCalled() },
    {
      id: 'jobs.nav.reset-filters',
      assert: (cb) => expect(cb.onResetFilters).toHaveBeenCalled(),
    },
    { id: 'jobs.nav.open-help', assert: (cb) => expect(cb.onOpenHelp).toHaveBeenCalled() },
  ]

  for (const { id, assert } of cases) {
    it(`${id} invokes its callback`, () => {
      const cb = makeCallbacks()
      jobCommandHandlers(cb)[id](evt())
      assert(cb)
    })
  }
})
