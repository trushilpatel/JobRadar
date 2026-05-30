import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'

vi.mock('@/lib/api/userConfig')
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))
vi.mock('@/lib/core/supabase', () => ({
  supabase: {
    auth: {
      getSession: vi.fn(),
      onAuthStateChange: vi.fn(() => ({ data: { subscription: { unsubscribe: vi.fn() } } })),
    },
  },
}))

import * as userConfigApi from '@/lib/api/userConfig'
import { toast } from 'sonner'
import { useUserConfig, usePatchUserConfig, USER_CONFIG_QUERY_KEY } from '@/lib/hooks/useUserConfig'

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: 0, gcTime: 0 },
      mutations: { retry: 0 },
    },
  })
  return {
    qc,
    wrapper: ({ children }: { children: React.ReactNode }) =>
      createElement(QueryClientProvider, { client: qc }, children),
  }
}

describe('useUserConfig', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('configures staleTime as 30 minutes', async () => {
    vi.mocked(userConfigApi.getUserConfig).mockResolvedValue({ show_keyboard_shortcuts: false })
    const { wrapper, qc } = makeWrapper()
    const { result } = renderHook(() => useUserConfig(), { wrapper })
    await waitFor(() => result.current.isSuccess)
    const query = qc.getQueryCache().find({ queryKey: USER_CONFIG_QUERY_KEY })
    // staleTime is 30 * 60 * 1000 = 1_800_000. It is merged onto query.options at
    // runtime from the observer, but QueryOptions doesn't declare it in v5's types.
    const options = query?.options as { staleTime?: number } | undefined
    expect(options?.staleTime).toBe(30 * 60 * 1000)
  })

  it('invalidates user-config query on successful patch', async () => {
    vi.mocked(userConfigApi.getUserConfig).mockResolvedValue({ show_keyboard_shortcuts: false })
    vi.mocked(userConfigApi.patchUserConfig).mockResolvedValue({ show_keyboard_shortcuts: true })
    const { wrapper, qc } = makeWrapper()
    const { result } = renderHook(
      () => ({ config: useUserConfig(), patch: usePatchUserConfig() }),
      { wrapper }
    )
    await waitFor(() => result.current.config.isSuccess)

    const invalidateSpy = vi.spyOn(qc, 'invalidateQueries')
    await act(async () => {
      result.current.patch.mutate({ show_keyboard_shortcuts: true })
    })
    await waitFor(() => result.current.patch.isSuccess)
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: USER_CONFIG_QUERY_KEY })
  })

  it('returns default show_keyboard_shortcuts: false when no data', () => {
    vi.mocked(userConfigApi.getUserConfig).mockReturnValue(new Promise(() => {}))
    const { wrapper } = makeWrapper()
    const { result } = renderHook(() => useUserConfig(), { wrapper })
    // While loading, data is undefined — consumer falls back to false
    const value = result.current.data?.show_keyboard_shortcuts ?? false
    expect(value).toBe(false)
  })

  it('shows error toast when patch fails', async () => {
    vi.mocked(userConfigApi.getUserConfig).mockResolvedValue({ show_keyboard_shortcuts: false })
    vi.mocked(userConfigApi.patchUserConfig).mockRejectedValue(new Error('Network error'))
    const { wrapper } = makeWrapper()
    const { result } = renderHook(() => ({ patch: usePatchUserConfig() }), { wrapper })

    await act(async () => {
      result.current.patch.mutate({ show_keyboard_shortcuts: true })
    })
    await waitFor(() => result.current.patch.isError)
    expect(vi.mocked(toast.error)).toHaveBeenCalledWith('Failed to update preferences.')
  })
})
