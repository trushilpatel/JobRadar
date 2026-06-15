import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'

vi.mock('@/lib/api/userConfig')
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))
vi.mock('@clerk/react', () => ({ useAuth: vi.fn() }))

import * as userConfigApi from '@/lib/api/userConfig'
import { useAuth } from '@clerk/react'
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
    // Default: Clerk is loaded and user is signed in — tests that need a
    // different state override this individually.
    vi.mocked(useAuth).mockReturnValue({ isLoaded: true, isSignedIn: true } as never)
  })

  it('configures staleTime as 30 minutes', async () => {
    vi.mocked(userConfigApi.getUserConfig).mockResolvedValue({ show_keyboard_shortcuts: false })
    const { wrapper, qc } = makeWrapper()
    const { result } = renderHook(() => useUserConfig(), { wrapper })
    await waitFor(() => result.current.isSuccess)
    const query = qc.getQueryCache().find({ queryKey: USER_CONFIG_QUERY_KEY })
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

  // ── enabled guard (auth race-condition fix) ─────────────────────────────────

  it('does not fire the query while Clerk is still loading', () => {
    vi.mocked(useAuth).mockReturnValue({ isLoaded: false, isSignedIn: false } as never)
    const { wrapper } = makeWrapper()
    const { result } = renderHook(() => useUserConfig(), { wrapper })
    // fetchStatus='idle' means the query is disabled, not just waiting for data
    expect(result.current.fetchStatus).toBe('idle')
    expect(vi.mocked(userConfigApi.getUserConfig)).not.toHaveBeenCalled()
  })

  it('does not fire the query when Clerk is loaded but user is not signed in', () => {
    vi.mocked(useAuth).mockReturnValue({ isLoaded: true, isSignedIn: false } as never)
    const { wrapper } = makeWrapper()
    const { result } = renderHook(() => useUserConfig(), { wrapper })
    expect(result.current.fetchStatus).toBe('idle')
    expect(vi.mocked(userConfigApi.getUserConfig)).not.toHaveBeenCalled()
  })

  it('fires the query once Clerk is loaded and user is signed in', async () => {
    vi.mocked(userConfigApi.getUserConfig).mockResolvedValue({ show_keyboard_shortcuts: false })
    const { wrapper } = makeWrapper()
    const { result } = renderHook(() => useUserConfig(), { wrapper })
    await waitFor(() => result.current.isSuccess)
    expect(vi.mocked(userConfigApi.getUserConfig)).toHaveBeenCalledTimes(1)
  })
})
