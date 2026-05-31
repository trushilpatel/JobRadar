import { renderHook } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { CommandHandlers, CommandRegistry } from '../types'
import { useCommandKeyboard } from '../useCommandKeyboard'

// Mock react-hotkeys-hook to capture each registration (key string, callback,
// options) without wiring real document listeners. Tests then invoke the
// captured callback directly with a synthetic KeyboardEvent + HotkeysEvent,
// exercising the engine's full resolve→dispatch path deterministically.
interface HotkeysEventLike {
  keys?: string[]
  meta?: boolean
  ctrl?: boolean
  alt?: boolean
  shift?: boolean
}
interface Reg {
  keys: string
  callback: (e: KeyboardEvent, hk: HotkeysEventLike) => void
  options?: { enabled?: boolean | (() => boolean); useKey?: boolean }
}
const regs: Reg[] = []

vi.mock('react-hotkeys-hook', () => ({
  useHotkeys: (
    keys: string,
    callback: (e: KeyboardEvent, hk: HotkeysEventLike) => void,
    options?: Reg['options']
  ) => {
    regs.push({ keys, callback, options })
  },
}))

function stubPlatform(mac: boolean) {
  Object.defineProperty(window.navigator, 'platform', {
    value: mac ? 'MacIntel' : 'Win32',
    configurable: true,
  })
}

// The DOM `key` value the engine reconstructs the base from.
const DOM_KEY: Record<string, string> = {
  j: 'j',
  down: 'ArrowDown',
  left: 'ArrowLeft',
  slash: '/',
}

function regFor(registeredKey: string): Reg {
  const reg = regs.find((r) =>
    r.keys
      .split(',')
      .map((s) => s.trim())
      .includes(registeredKey)
  )
  if (!reg)
    throw new Error(
      `no registration for ${registeredKey}; have: ${regs.map((r) => r.keys).join(' | ')}`
    )
  return reg
}

// Fire a synthetic keydown against whichever bucket owns the given key.
function fire(registeredKey: string) {
  const reg = regFor(registeredKey)
  const domKey = DOM_KEY[registeredKey] ?? registeredKey
  reg.callback(new KeyboardEvent('keydown', { key: domKey }), { keys: [registeredKey] })
}

beforeEach(() => {
  regs.length = 0
  stubPlatform(true)
  vi.clearAllMocks()
})

type Id = 'a.next' | 'a.page' | 'a.search'
const registry: CommandRegistry = {
  'a.next': { label: 'Next', keys: ['j', 'down'], scope: 'global' },
  'a.page': { label: 'Page', keys: ['left'], scope: 'list' },
  'a.search': { label: 'Search', keys: ['/'], useKey: true },
}

function makeHandlers(): CommandHandlers<Id> {
  return { 'a.next': vi.fn(), 'a.page': vi.fn(), 'a.search': vi.fn() }
}

describe('useCommandKeyboard', () => {
  it('always registers exactly three useHotkeys buckets', () => {
    renderHook(() => useCommandKeyboard(registry, makeHandlers()))
    expect(regs).toHaveLength(3)
  })

  it('invokes the matching handler when a registered key fires', () => {
    const handlers = makeHandlers()
    renderHook(() => useCommandKeyboard(registry, handlers))
    fire('j')
    expect(handlers['a.next']).toHaveBeenCalledTimes(1)
  })

  it('routes an alias key to the same handler', () => {
    const handlers = makeHandlers()
    renderHook(() => useCommandKeyboard(registry, handlers))
    fire('down')
    expect(handlers['a.next']).toHaveBeenCalledTimes(1)
  })

  it('disables every bucket when enabled is false', () => {
    renderHook(() => useCommandKeyboard(registry, makeHandlers(), { enabled: false }))
    for (const r of regs) {
      const en = r.options?.enabled
      expect(typeof en === 'function' ? en() : en).toBe(false)
    }
  })

  it('re-arms when enabled flips back to true (no remount)', () => {
    const handlers = makeHandlers()
    const { rerender } = renderHook(
      ({ enabled }: { enabled: boolean }) => useCommandKeyboard(registry, handlers, { enabled }),
      { initialProps: { enabled: false } }
    )
    regs.length = 0
    rerender({ enabled: true })
    for (const r of regs) {
      const en = r.options?.enabled
      expect(typeof en === 'function' ? en() : en).toBe(true)
    }
  })

  it('suppresses list-scope keys when focus is in a popover; global stays enabled', () => {
    renderHook(() => useCommandKeyboard(registry, makeHandlers()))

    const popover = document.createElement('div')
    popover.setAttribute('role', 'dialog')
    const inner = document.createElement('button')
    inner.setAttribute('tabindex', '0')
    popover.appendChild(inner)
    document.body.appendChild(popover)
    inner.focus()
    expect(inner.closest('[role="dialog"]')).toBe(popover)

    const listEnabled = regFor('left').options?.enabled
    expect(typeof listEnabled === 'function' ? listEnabled() : listEnabled).toBe(false)
    // Global bucket uses a plain boolean `enabled` (true), unaffected by focus.
    expect(regFor('j').options?.enabled).toBe(true)

    document.body.removeChild(popover)
  })

  it('invokes the latest handler after the handlers identity changes', () => {
    const first = makeHandlers()
    const { rerender } = renderHook(
      ({ h }: { h: CommandHandlers<Id> }) => useCommandKeyboard(registry, h),
      { initialProps: { h: first } }
    )
    const second = makeHandlers()
    rerender({ h: second })
    fire('j')
    expect(first['a.next']).not.toHaveBeenCalled()
    expect(second['a.next']).toHaveBeenCalledTimes(1)
  })

  it('marks the useKey bucket with useKey:true', () => {
    renderHook(() => useCommandKeyboard(registry, makeHandlers()))
    expect(regFor('slash').options?.useKey).toBe(true)
  })

  it('does nothing when an unregistered key fires', () => {
    const handlers = makeHandlers()
    renderHook(() => useCommandKeyboard(registry, handlers))
    regFor('j').callback(new KeyboardEvent('keydown', { key: 'z' }), { keys: ['z'] })
    expect(handlers['a.next']).not.toHaveBeenCalled()
  })
})

describe('useCommandKeyboard chord resolution', () => {
  type ChordId = 'c.top' | 'c.new'
  const chordRegistry: CommandRegistry = {
    'c.top': { label: 'Top', keys: ['g>g'] },
    'c.new': { label: 'New', keys: ['g>n'] },
  }
  function makeChordHandlers(): CommandHandlers<ChordId> {
    return { 'c.top': vi.fn(), 'c.new': vi.fn() }
  }

  it('completes a g>n chord across two keydowns', () => {
    const handlers = makeChordHandlers()
    renderHook(() => useCommandKeyboard(chordRegistry, handlers))
    const reg = regFor('g')
    reg.callback(new KeyboardEvent('keydown', { key: 'g' }), { keys: ['g'] })
    reg.callback(new KeyboardEvent('keydown', { key: 'n' }), { keys: ['n'] })
    expect(handlers['c.new']).toHaveBeenCalledTimes(1)
    expect(handlers['c.top']).not.toHaveBeenCalled()
  })

  it('completes a g>g chord (prefix equals completion key)', () => {
    const handlers = makeChordHandlers()
    renderHook(() => useCommandKeyboard(chordRegistry, handlers))
    const reg = regFor('g')
    reg.callback(new KeyboardEvent('keydown', { key: 'g' }), { keys: ['g'] })
    reg.callback(new KeyboardEvent('keydown', { key: 'g' }), { keys: ['g'] })
    expect(handlers['c.top']).toHaveBeenCalledTimes(1)
  })
})
