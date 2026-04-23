import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const getSession = vi.fn()
const refreshSession = vi.fn()
const signOut = vi.fn()

vi.mock('@/lib/supabase', () => ({
  supabase: {
    auth: {
      getSession: (...args: unknown[]) => getSession(...args),
      refreshSession: (...args: unknown[]) => refreshSession(...args),
      signOut: (...args: unknown[]) => signOut(...args),
    },
  },
}))

const originalFetch = globalThis.fetch
const originalLocation = window.location

function mockLocation() {
  const assign = vi.fn()
  Object.defineProperty(window, 'location', {
    configurable: true,
    value: {
      ...originalLocation,
      assign,
      pathname: '/dashboard',
      origin: 'http://localhost',
    },
  })
  return assign
}

function restoreLocation() {
  Object.defineProperty(window, 'location', {
    configurable: true,
    value: originalLocation,
  })
}

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

beforeEach(() => {
  vi.resetModules()
  getSession.mockReset()
  refreshSession.mockReset()
  signOut.mockReset()
})

afterEach(() => {
  restoreLocation()
  globalThis.fetch = originalFetch
})

describe('apiClient', () => {
  it('attaches bearer header when session present', async () => {
    getSession.mockResolvedValue({ data: { session: { access_token: 'tok-123' } } })
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { data: { hello: 'world' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const { apiClient } = await import('@/lib/api-client')
    const result = await apiClient.get<{ hello: string }>('/ping')

    expect(result).toEqual({ hello: 'world' })
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const init = fetchMock.mock.calls[0][1] as RequestInit
    const headers = init.headers as Headers
    expect(headers.get('Authorization')).toBe('Bearer tok-123')
  })

  it('retries once on 401 after refreshing session', async () => {
    getSession
      .mockResolvedValueOnce({ data: { session: { access_token: 'old-token' } } })
      .mockResolvedValueOnce({ data: { session: { access_token: 'new-token' } } })
    refreshSession.mockResolvedValue({ data: { session: { access_token: 'new-token' } }, error: null })

    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { error: { message: 'expired' } }))
      .mockResolvedValueOnce(jsonResponse(200, { data: { ok: true } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const { apiClient } = await import('@/lib/api-client')
    const result = await apiClient.get<{ ok: boolean }>('/secure')

    expect(result).toEqual({ ok: true })
    expect(refreshSession).toHaveBeenCalledTimes(1)
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('dedupes concurrent refresh calls', async () => {
    // First 3 calls (before refresh) return stale token; subsequent calls return fresh.
    let getSessionCount = 0
    getSession.mockImplementation(() => {
      getSessionCount += 1
      const token = getSessionCount <= 3 ? 'stale' : 'fresh'
      return Promise.resolve({ data: { session: { access_token: token } } })
    })

    const refreshHandle: { resolve: (() => void) | null } = { resolve: null }
    refreshSession.mockImplementation(
      () =>
        new Promise((resolve) => {
          refreshHandle.resolve = () =>
            resolve({ data: { session: { access_token: 'fresh' } }, error: null })
        })
    )

    const fetchMock = vi.fn().mockImplementation((_url: string, init: RequestInit) => {
      const auth = (init.headers as Headers).get('Authorization')
      if (auth === 'Bearer stale') {
        return Promise.resolve(jsonResponse(401, { error: { message: 'expired' } }))
      }
      return Promise.resolve(jsonResponse(200, { data: { ok: true } }))
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const { apiClient, __testing } = await import('@/lib/api-client')
    __testing.resetPendingRefresh()

    const p1 = apiClient.get<{ ok: boolean }>('/a')
    const p2 = apiClient.get<{ ok: boolean }>('/b')
    const p3 = apiClient.get<{ ok: boolean }>('/c')

    // Wait until all three requests have triggered a refresh attempt.
    await vi.waitFor(() => {
      expect(refreshSession).toHaveBeenCalled()
      expect(refreshHandle.resolve).not.toBeNull()
    })
    refreshHandle.resolve?.()
    const [r1, r2, r3] = await Promise.all([p1, p2, p3])

    expect(r1).toEqual({ ok: true })
    expect(r2).toEqual({ ok: true })
    expect(r3).toEqual({ ok: true })
    expect(refreshSession).toHaveBeenCalledTimes(1)
  })

  it('signs out and redirects when refresh fails', async () => {
    const assign = mockLocation()
    getSession.mockResolvedValue({ data: { session: { access_token: 'stale' } } })
    refreshSession.mockResolvedValue({ data: { session: null }, error: { message: 'nope' } })
    signOut.mockResolvedValue({ error: null })

    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(401, { error: { message: 'expired' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const { apiClient, __testing } = await import('@/lib/api-client')
    __testing.resetPendingRefresh()

    await expect(apiClient.get('/secure')).rejects.toThrow()
    expect(signOut).toHaveBeenCalledWith({ scope: 'global' })
    expect(assign).toHaveBeenCalledWith('/login')
  })

  it('does NOT trigger refresh on 403', async () => {
    getSession.mockResolvedValue({ data: { session: { access_token: 't' } } })
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(403, { error: { message: 'forbidden' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const { apiClient, __testing } = await import('@/lib/api-client')
    __testing.resetPendingRefresh()

    await expect(apiClient.get('/secure')).rejects.toThrow('forbidden')
    expect(refreshSession).not.toHaveBeenCalled()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})
