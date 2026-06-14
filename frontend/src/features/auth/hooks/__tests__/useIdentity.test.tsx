import { describe, it, expect, vi, afterEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createElement } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

// Mock @clerk/react hooks
vi.mock('@clerk/react', () => ({
  useAuth: vi.fn(),
  useUser: vi.fn(),
}))

// Mock fetchMe API call
vi.mock('@/features/auth/api/me.api', () => ({
  fetchMe: vi.fn().mockResolvedValue({
    user_id: 'user_abc',
    email: 'alice@example.com',
    display_name: 'Alice',
    status: 'active' as const,
    roles: ['admin'],
  }),
}))

function wrapper({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return createElement(QueryClientProvider, { client: qc }, children)
}

describe('useIdentity', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('returns isAdmin=true for admin publicMetadata', async () => {
    const { useAuth, useUser } = await import('@clerk/react')
    vi.mocked(useAuth).mockReturnValue({ isSignedIn: true, isLoaded: true } as never)
    vi.mocked(useUser).mockReturnValue({
      isLoaded: true,
      user: { publicMetadata: { role: 'admin' } },
    } as never)

    const { useIdentity } = await import('../useIdentity')
    const { result } = renderHook(() => useIdentity(), { wrapper })

    await waitFor(() => expect(result.current.isLoaded).toBe(true))
    expect(result.current.isAdmin).toBe(true)
    expect(result.current.isSignedIn).toBe(true)
  })

  it('returns isAdmin=false when role is "user"', async () => {
    const { useAuth, useUser } = await import('@clerk/react')
    vi.mocked(useAuth).mockReturnValue({ isSignedIn: true, isLoaded: true } as never)
    vi.mocked(useUser).mockReturnValue({
      isLoaded: true,
      user: { publicMetadata: { role: 'user' } },
    } as never)

    const { useIdentity } = await import('../useIdentity')
    const { result } = renderHook(() => useIdentity(), { wrapper })

    await waitFor(() => expect(result.current.isLoaded).toBe(true))
    expect(result.current.isAdmin).toBe(false)
  })

  it('returns isAdmin=false when publicMetadata has no role', async () => {
    const { useAuth, useUser } = await import('@clerk/react')
    vi.mocked(useAuth).mockReturnValue({ isSignedIn: true, isLoaded: true } as never)
    vi.mocked(useUser).mockReturnValue({
      isLoaded: true,
      user: { publicMetadata: {} },
    } as never)

    const { useIdentity } = await import('../useIdentity')
    const { result } = renderHook(() => useIdentity(), { wrapper })

    await waitFor(() => expect(result.current.isLoaded).toBe(true))
    expect(result.current.isAdmin).toBe(false)
  })

  it('returns isSignedIn=false and does not call fetchMe when not authenticated', async () => {
    const { useAuth, useUser } = await import('@clerk/react')
    vi.mocked(useAuth).mockReturnValue({ isSignedIn: false, isLoaded: true } as never)
    vi.mocked(useUser).mockReturnValue({ isLoaded: true, user: null } as never)

    const { fetchMe } = await import('@/features/auth/api/me.api')
    const { useIdentity } = await import('../useIdentity')
    const { result } = renderHook(() => useIdentity(), { wrapper })

    await waitFor(() => expect(result.current.isLoaded).toBe(true))
    expect(result.current.isSignedIn).toBe(false)
    expect(result.current.isAdmin).toBe(false)
    expect(fetchMe).not.toHaveBeenCalled()
  })

  it('returns isLoaded=false while Clerk is initializing', async () => {
    const { useAuth, useUser } = await import('@clerk/react')
    vi.mocked(useAuth).mockReturnValue({ isSignedIn: false, isLoaded: false } as never)
    vi.mocked(useUser).mockReturnValue({ isLoaded: false, user: null } as never)

    const { useIdentity } = await import('../useIdentity')
    const { result } = renderHook(() => useIdentity(), { wrapper })

    expect(result.current.isLoaded).toBe(false)
  })

  it('returns isLoaded=false when only useAuth is not loaded', async () => {
    const { useAuth, useUser } = await import('@clerk/react')
    vi.mocked(useAuth).mockReturnValue({ isSignedIn: false, isLoaded: false } as never)
    vi.mocked(useUser).mockReturnValue({
      isLoaded: true,
      user: { publicMetadata: {} },
    } as never)

    const { useIdentity } = await import('../useIdentity')
    const { result } = renderHook(() => useIdentity(), { wrapper })

    expect(result.current.isLoaded).toBe(false)
  })

  it('returns isLoaded=false when only useUser is not loaded', async () => {
    const { useAuth, useUser } = await import('@clerk/react')
    vi.mocked(useAuth).mockReturnValue({ isSignedIn: true, isLoaded: true } as never)
    vi.mocked(useUser).mockReturnValue({ isLoaded: false, user: null } as never)

    const { useIdentity } = await import('../useIdentity')
    const { result } = renderHook(() => useIdentity(), { wrapper })

    expect(result.current.isLoaded).toBe(false)
  })
})
