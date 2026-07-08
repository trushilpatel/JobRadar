import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, render, fireEvent, act, cleanup } from '@testing-library/react'
import { useJobKeyboard, type UseJobKeyboardArgs } from '@/features/jobs/hooks/useJobKeyboard'

// Integration test: NO mock of react-hotkeys-hook. We fire real DOM keydown
// events and assert the page callbacks fire, exercising the full path
// JOB_COMMANDS → buildDispatch → useCommandKeyboard → react-hotkeys-hook →
// resolve → handler.

// Pin the platform to Mac so `mod` resolves to `meta` — the meta-combo tests
// below fire with metaKey. (Without this, a prior test file leaving
// navigator.platform on Win32 would make `mod+k` register as ctrl+k.)
beforeEach(() => {
  Object.defineProperty(window.navigator, 'platform', { value: 'MacIntel', configurable: true })
})

afterEach(() => {
  cleanup()
})

function makeArgs(): UseJobKeyboardArgs {
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
    onUndo: vi.fn(),
    onFocusSearch: vi.fn(),
    onOpenPalette: vi.fn(),
    onResetFilters: vi.fn(),
    onOpenHelp: vi.fn(),
  }
}

// react-hotkeys-hook inspects `e.code` for chord/modifier tracking; supply a
// plausible code so the browser-equivalent path is taken.
function codeFor(key: string): string {
  if (key.length === 1) {
    const upper = key.toUpperCase()
    if (/[A-Z]/.test(upper)) return `Key${upper}`
    if (/[0-9]/.test(upper)) return `Digit${upper}`
    if (key === '/' || key === '?') return 'Slash'
  }
  if (key === 'ArrowDown') return 'ArrowDown'
  if (key === 'ArrowUp') return 'ArrowUp'
  if (key === 'ArrowLeft') return 'ArrowLeft'
  if (key === 'ArrowRight') return 'ArrowRight'
  if (key === 'Enter') return 'Enter'
  if (key === 'Escape') return 'Escape'
  return ''
}

function fireKey(init: KeyboardEventInit) {
  const code = codeFor(init.key ?? '')
  act(() => {
    fireEvent.keyDown(document.body, { code, ...init })
    // Release so react-hotkeys-hook's pressed-keys tracker does not retain a
    // "held" key into the next press or test.
    fireEvent.keyUp(document.body, { code, ...init })
  })
}

describe('useJobKeyboard', () => {
  let args: UseJobKeyboardArgs

  beforeEach(() => {
    args = makeArgs()
  })

  it('j and ArrowDown both call onFocusNext', () => {
    renderHook(() => useJobKeyboard(args))
    fireKey({ key: 'j' })
    fireKey({ key: 'ArrowDown' })
    expect(args.onFocusNext).toHaveBeenCalledTimes(2)
  })

  it('k and ArrowUp both call onFocusPrev', () => {
    renderHook(() => useJobKeyboard(args))
    fireKey({ key: 'k' })
    fireKey({ key: 'ArrowUp' })
    expect(args.onFocusPrev).toHaveBeenCalledTimes(2)
  })

  it('Enter calls onOpenPreview; Cmd+Enter calls onOpenUrl', () => {
    renderHook(() => useJobKeyboard(args))
    fireKey({ key: 'Enter' })
    expect(args.onOpenPreview).toHaveBeenCalledTimes(1)
    fireKey({ key: 'Enter', metaKey: true })
    expect(args.onOpenUrl).toHaveBeenCalledTimes(1)
  })

  it('Escape calls onEscape', () => {
    renderHook(() => useJobKeyboard(args))
    fireKey({ key: 'Escape' })
    expect(args.onEscape).toHaveBeenCalledTimes(1)
  })

  it.each([
    ['a', 'applied'],
    ['i', 'interviewing'],
    ['o', 'offer'],
    ['r', 'rejected'],
  ] as const)('"%s" sets workflow status to %s', (key, status) => {
    renderHook(() => useJobKeyboard(args))
    fireKey({ key })
    expect(args.onSetStatus).toHaveBeenCalledWith(status)
  })

  it('"s" toggles save — never sets workflow status', () => {
    renderHook(() => useJobKeyboard(args))
    fireKey({ key: 's' })
    expect(args.onToggleSave).toHaveBeenCalledTimes(1)
    expect(args.onSetStatus).not.toHaveBeenCalled()
  })

  it('u calls onUndo — d is unbound (dismiss feature removed)', () => {
    renderHook(() => useJobKeyboard(args))
    fireKey({ key: 'd' })
    fireKey({ key: 'u' })
    expect(args.onUndo).toHaveBeenCalledTimes(1)
  })

  it('"/" focuses search; Cmd+K opens palette; ? opens help', () => {
    renderHook(() => useJobKeyboard(args))
    fireKey({ key: '/' })
    fireKey({ key: 'k', metaKey: true })
    fireKey({ key: '?', shiftKey: true })
    expect(args.onFocusSearch).toHaveBeenCalledTimes(1)
    expect(args.onOpenPalette).toHaveBeenCalledTimes(1)
    expect(args.onOpenHelp).toHaveBeenCalled()
  })

  it('Cmd+Shift+R resets filters', () => {
    renderHook(() => useJobKeyboard(args))
    fireKey({ key: 'r', metaKey: true, shiftKey: true })
    expect(args.onResetFilters).toHaveBeenCalledTimes(1)
  })

  it('g g calls onFocusFirst', () => {
    renderHook(() => useJobKeyboard(args))
    fireKey({ key: 'g' })
    fireKey({ key: 'g' })
    expect(args.onFocusFirst).toHaveBeenCalledTimes(1)
  })

  it('Shift+G calls onFocusLast', () => {
    renderHook(() => useJobKeyboard(args))
    fireKey({ key: 'G', shiftKey: true })
    expect(args.onFocusLast).toHaveBeenCalledTimes(1)
  })

  it.each([
    ['n', 'new'],
    ['s', 'saved'],
    ['a', 'applied'],
    ['i', 'interviewing'],
    ['o', 'offer'],
    ['r', 'rejected'],
  ] as const)('g %s jumps to the %s tab', (key, tab) => {
    renderHook(() => useJobKeyboard(args))
    fireKey({ key: 'g' })
    fireKey({ key })
    // `toHaveBeenCalledWith` (not Last) keeps this resilient if the second key
    // also has a standalone binding that fires alongside the chord.
    expect(args.onGoToTab).toHaveBeenCalledWith(tab)
  })

  it('does not fire any callbacks when enabled is false', () => {
    renderHook(() => useJobKeyboard({ ...args, enabled: false }))
    fireKey({ key: 'j' })
    fireKey({ key: 's' })
    fireKey({ key: 'Escape' })
    expect(args.onFocusNext).not.toHaveBeenCalled()
    expect(args.onSetStatus).not.toHaveBeenCalled()
    expect(args.onEscape).not.toHaveBeenCalled()
  })

  it('shortcuts do not fire when the keydown originates from an INPUT', () => {
    function Harness() {
      useJobKeyboard(args)
      return <input data-testid="search" />
    }
    const { getByTestId } = render(<Harness />)
    const input = getByTestId('search') as HTMLInputElement
    input.focus()
    fireEvent.keyDown(input, { key: 'j', code: 'KeyJ' })
    fireEvent.keyDown(input, { key: 's', code: 'KeyS' })
    expect(args.onFocusNext).not.toHaveBeenCalled()
    expect(args.onSetStatus).not.toHaveBeenCalled()
  })
})
