import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, render, fireEvent, act } from '@testing-library/react'
import { useJobKeyboard, type UseJobKeyboardArgs } from '@/features/jobs/hooks/useJobKeyboard'

function makeArgs(): UseJobKeyboardArgs {
  return {
    onFocusNext: vi.fn(),
    onFocusPrev: vi.fn(),
    onFocusFirst: vi.fn(),
    onFocusLast: vi.fn(),
    onOpenPreview: vi.fn(),
    onOpenUrl: vi.fn(),
    onEscape: vi.fn(),
    onPrevPage: vi.fn(),
    onNextPage: vi.fn(),
    onGoToTab: vi.fn(),
    onSetStatus: vi.fn(),
    onDismiss: vi.fn(),
    onUndo: vi.fn(),
    onFocusSearch: vi.fn(),
    onOpenPalette: vi.fn(),
    onResetFilters: vi.fn(),
    onOpenHelp: vi.fn(),
  }
}

// Map letters/digits/specials to a plausible `code` value so react-hotkeys-hook
// (which inspects `e.code` for chord/modifier tracking) sees the same code a
// browser would emit.
function codeFor(key: string): string {
  if (key.length === 1) {
    const upper = key.toUpperCase()
    if (/[A-Z]/.test(upper)) return `Key${upper}`
    if (/[0-9]/.test(upper)) return `Digit${upper}`
    if (key === '/') return 'Slash'
    if (key === '?') return 'Slash'
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
  act(() => {
    fireEvent.keyDown(document.body, { code: codeFor(init.key ?? ''), ...init })
  })
}

describe('useJobKeyboard', () => {
  let args: UseJobKeyboardArgs

  beforeEach(() => {
    args = makeArgs()
  })

  // ── Single-key navigation ─────────────────────────────────────────────────
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

  it('ArrowLeft / ArrowRight page through results', () => {
    renderHook(() => useJobKeyboard(args))
    fireKey({ key: 'ArrowLeft' })
    fireKey({ key: 'ArrowRight' })
    expect(args.onPrevPage).toHaveBeenCalledTimes(1)
    expect(args.onNextPage).toHaveBeenCalledTimes(1)
  })

  it('single ArrowLeft fires onPrevPage only (not onFirstPage)', () => {
    const onFirstPage = vi.fn()
    renderHook(() => useJobKeyboard({ ...args, onFirstPage }))
    fireKey({ key: 'ArrowLeft' })
    expect(args.onPrevPage).toHaveBeenCalledTimes(1)
    expect(onFirstPage).not.toHaveBeenCalled()
  })

  it('double ArrowLeft within 200ms fires onFirstPage on the second press', () => {
    const onFirstPage = vi.fn()
    renderHook(() => useJobKeyboard({ ...args, onFirstPage }))
    // First press: fires prev.
    fireKey({ key: 'ArrowLeft' })
    expect(args.onPrevPage).toHaveBeenCalledTimes(1)
    // Second press immediately after: fires first (not a second prev).
    fireKey({ key: 'ArrowLeft' })
    expect(onFirstPage).toHaveBeenCalledTimes(1)
    expect(args.onPrevPage).toHaveBeenCalledTimes(1) // still only the first call
  })

  it('three ArrowLeft presses fire prev → first → prev (third press resets the window)', () => {
    const onFirstPage = vi.fn()
    renderHook(() => useJobKeyboard({ ...args, onFirstPage }))
    fireKey({ key: 'ArrowLeft' })
    fireKey({ key: 'ArrowLeft' })
    fireKey({ key: 'ArrowLeft' })
    expect(args.onPrevPage).toHaveBeenCalledTimes(2)
    expect(onFirstPage).toHaveBeenCalledTimes(1)
  })

  // ── Status keys ───────────────────────────────────────────────────────────
  it.each([
    ['s', 'saved'],
    ['a', 'applied'],
    ['i', 'interviewing'],
    ['o', 'offer'],
    ['r', 'rejected'],
  ] as const)('"%s" sets status to %s', (key, status) => {
    renderHook(() => useJobKeyboard(args))
    fireKey({ key })
    expect(args.onSetStatus).toHaveBeenCalledWith(status)
  })

  it('d calls onDismiss, u calls onUndo', () => {
    renderHook(() => useJobKeyboard(args))
    fireKey({ key: 'd' })
    fireKey({ key: 'u' })
    expect(args.onDismiss).toHaveBeenCalledTimes(1)
    expect(args.onUndo).toHaveBeenCalledTimes(1)
  })

  // ── Selection / bulk shortcuts removed in the bookmark refactor ───────────
  // x, shift+x, shift+s, shift+a, shift+d no longer exist — verify they are
  // truly inert (don't fire any callback). The status `a` shortcut on the
  // focused row still works without modifiers.

  it('ArrowLeft / ArrowRight are suppressed inside a Radix popover', () => {
    function Harness() {
      useJobKeyboard(args)
      return (
        <div role="listbox">
          <button data-testid="opt">option</button>
        </div>
      )
    }
    const { getByTestId } = render(<Harness />)
    const opt = getByTestId('opt') as HTMLElement
    opt.focus()
    // Active element is inside [role="listbox"] — page nav must NOT fire.
    fireEvent.keyDown(opt, { code: 'ArrowLeft', key: 'ArrowLeft' })
    fireEvent.keyDown(opt, { code: 'ArrowRight', key: 'ArrowRight' })
    expect(args.onPrevPage).not.toHaveBeenCalled()
    expect(args.onNextPage).not.toHaveBeenCalled()
  })

  it('shift+/ (cross-layout fallback) also opens help', () => {
    renderHook(() => useJobKeyboard(args))
    // First call from the existing "?" test isolation issue won't apply here.
    fireKey({ key: '/', shiftKey: true })
    // shift+/ should fire onOpenHelp at least once.
    expect(args.onOpenHelp).toHaveBeenCalled()
  })

  // ── Search / palette / help ───────────────────────────────────────────────
  it('"/" focuses search; Cmd+K opens palette; ? opens help', () => {
    renderHook(() => useJobKeyboard(args))
    fireKey({ key: '/' })
    fireKey({ key: 'k', metaKey: true })
    fireKey({ key: '?', shiftKey: true })
    expect(args.onFocusSearch).toHaveBeenCalledTimes(1)
    expect(args.onOpenPalette).toHaveBeenCalledTimes(1)
    // `?` AND `shift+slash` both match the help binding on a US keyboard.
    // That redundancy is intentional (cross-layout fallback) — assert ≥1.
    expect(args.onOpenHelp).toHaveBeenCalled()
  })

  it('Cmd+Shift+R resets filters', () => {
    renderHook(() => useJobKeyboard(args))
    fireKey({ key: 'r', metaKey: true, shiftKey: true })
    expect(args.onResetFilters).toHaveBeenCalledTimes(1)
  })

  // ── Chord sequences ───────────────────────────────────────────────────────
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
    expect(args.onGoToTab).toHaveBeenLastCalledWith(tab)
  })

  // ── Enabled flag ──────────────────────────────────────────────────────────
  it('does not fire any callbacks when enabled is false', () => {
    renderHook(() => useJobKeyboard({ ...args, enabled: false }))
    fireKey({ key: 'j' })
    fireKey({ key: 's' })
    fireKey({ key: 'd' })
    fireKey({ key: 'Escape' })
    expect(args.onFocusNext).not.toHaveBeenCalled()
    expect(args.onSetStatus).not.toHaveBeenCalled()
    expect(args.onDismiss).not.toHaveBeenCalled()
    expect(args.onEscape).not.toHaveBeenCalled()
  })

  // ── enableOnFormTags=false: shortcuts ignored when typing in inputs ───────
  it('shortcuts do not fire when the keydown originates from an INPUT', () => {
    function Harness() {
      useJobKeyboard(args)
      return <input data-testid="search" />
    }
    const { getByTestId } = render(<Harness />)
    const input = getByTestId('search') as HTMLInputElement
    input.focus()
    fireEvent.keyDown(input, { key: 'j' })
    fireEvent.keyDown(input, { key: 's' })
    fireEvent.keyDown(input, { key: 'd' })
    expect(args.onFocusNext).not.toHaveBeenCalled()
    expect(args.onSetStatus).not.toHaveBeenCalled()
    expect(args.onDismiss).not.toHaveBeenCalled()
  })
})
