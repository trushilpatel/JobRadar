import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useUndoToast } from '@/lib/use-undo-toast'

vi.mock('sonner', () => ({
  toast: vi.fn(),
}))

// Pull the mocked toast after vi.mock has registered it.
import { toast } from 'sonner'

describe('useUndoToast', () => {
  beforeEach(() => {
    vi.mocked(toast).mockClear()
  })

  it('returns a stable callback across renders', () => {
    const { result, rerender } = renderHook(() => useUndoToast())
    const first = result.current
    rerender()
    expect(result.current).toBe(first)
  })

  it('calls toast with message, undo action, and the default 5s duration', () => {
    const { result } = renderHook(() => useUndoToast())
    const undo = vi.fn()
    act(() => {
      result.current('Moved to Saved', undo)
    })
    expect(toast).toHaveBeenCalledTimes(1)
    const [message, options] = vi.mocked(toast).mock.calls[0]
    expect(message).toBe('Moved to Saved')
    expect(options).toMatchObject({
      duration: 5000,
      action: { label: 'Undo' },
    })
    // Trigger the action callback and ensure it wires through to the supplied undo fn.
    const action = (options as { action: { onClick: () => void } }).action
    action.onClick()
    expect(undo).toHaveBeenCalledTimes(1)
  })

  it('respects a custom duration when provided', () => {
    const { result } = renderHook(() => useUndoToast())
    act(() => {
      result.current('Dismissed', () => {}, { duration: 8000 })
    })
    const [, options] = vi.mocked(toast).mock.calls[0]
    expect(options).toMatchObject({ duration: 8000 })
  })
})
