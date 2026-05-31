import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useAction } from '../use-action'
import * as cfg from '@/lib/hooks/useUserConfig'

vi.mock('@/lib/hooks/useUserConfig')

beforeEach(() => {
  vi.clearAllMocks()
  Object.defineProperty(window.navigator, 'platform', { value: 'MacIntel', configurable: true })
})

describe('useAction', () => {
  it('returns the label and a kbd hint when shortcuts are enabled', () => {
    vi.mocked(cfg.useUserConfig).mockReturnValue({
      data: { show_keyboard_shortcuts: true },
    } as ReturnType<typeof cfg.useUserConfig>)
    const { result } = renderHook(() => useAction('Save / unsave', ['s']))
    expect(result.current.label).toBe('Save / unsave')
    expect(result.current.showKbd).toBe(true)
    expect(result.current.kbd).not.toBeNull()
    expect(result.current.ariaKeyshortcuts).toBe('s')
  })

  it('hides the kbd hint when shortcuts are disabled but keeps aria-keyshortcuts', () => {
    vi.mocked(cfg.useUserConfig).mockReturnValue({
      data: { show_keyboard_shortcuts: false },
    } as ReturnType<typeof cfg.useUserConfig>)
    const { result } = renderHook(() => useAction('Save / unsave', ['s']))
    expect(result.current.showKbd).toBe(false)
    expect(result.current.kbd).toBeNull()
    expect(result.current.ariaKeyshortcuts).toBe('s')
  })

  it('uses the primary (first) key when multiple aliases exist', () => {
    vi.mocked(cfg.useUserConfig).mockReturnValue({
      data: { show_keyboard_shortcuts: true },
    } as ReturnType<typeof cfg.useUserConfig>)
    const { result } = renderHook(() => useAction('Next job', ['j', 'down']))
    expect(result.current.shortcut).toBe('j')
    expect(result.current.ariaKeyshortcuts).toBe('j')
  })

  it('renders no hint and no aria attribute when there is no binding', () => {
    vi.mocked(cfg.useUserConfig).mockReturnValue({
      data: { show_keyboard_shortcuts: true },
    } as ReturnType<typeof cfg.useUserConfig>)
    const { result } = renderHook(() => useAction('Palette only', []))
    expect(result.current.shortcut).toBeNull()
    expect(result.current.ariaKeyshortcuts).toBeUndefined()
    expect(result.current.kbd).toBeNull()
    expect(result.current.showKbd).toBe(false)
  })
})
