import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'

vi.mock('@/lib/hooks/useUserConfig')
vi.mock('@/lib/core/supabase', () => ({
  supabase: {
    auth: {
      getSession: vi.fn(),
      onAuthStateChange: vi.fn(() => ({
        data: { subscription: { unsubscribe: vi.fn() } },
      })),
    },
  },
}))

import * as useUserConfigModule from '@/lib/hooks/useUserConfig'
import { useAction } from '@/components/ui/use-action'

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: 0, gcTime: 0 }, mutations: { retry: 0 } },
  })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children)
}

function mockConfig(show: boolean) {
  vi.mocked(useUserConfigModule.useUserConfig).mockReturnValue({
    data: { show_keyboard_shortcuts: show },
    isLoading: false,
    isError: false,
    isSuccess: true,
  } as ReturnType<typeof useUserConfigModule.useUserConfig>)
}

describe('useAction', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns label from token', () => {
    mockConfig(false)
    const { result } = renderHook(() => useAction('jobs.save'), { wrapper: makeWrapper() })
    expect(result.current.label).toBe('Save')
  })

  it('showKbd is false when show_keyboard_shortcuts is false', () => {
    mockConfig(false)
    const { result } = renderHook(() => useAction('jobs.save'), { wrapper: makeWrapper() })
    expect(result.current.showKbd).toBe(false)
  })

  it('showKbd is true when show_keyboard_shortcuts is true and token has shortcut', () => {
    mockConfig(true)
    const { result } = renderHook(() => useAction('jobs.save'), { wrapper: makeWrapper() })
    expect(result.current.showKbd).toBe(true)
    expect(result.current.shortcut).toBe('s')
  })

  it('kbd is null when showKbd is false', () => {
    mockConfig(false)
    const { result } = renderHook(() => useAction('jobs.save'), { wrapper: makeWrapper() })
    expect(result.current.kbd).toBeNull()
  })

  it('kbd is non-null when show_keyboard_shortcuts is true and token has shortcut', () => {
    mockConfig(true)
    const { result } = renderHook(() => useAction('jobs.save'), { wrapper: makeWrapper() })
    expect(result.current.kbd).not.toBeNull()
  })

  it('showKbd is false for tokens without shortcut regardless of preference', () => {
    mockConfig(true)
    const { result } = renderHook(() => useAction('nav.dashboard'), { wrapper: makeWrapper() })
    expect(result.current.showKbd).toBe(false)
    expect(result.current.shortcut).toBeNull()
  })
})
