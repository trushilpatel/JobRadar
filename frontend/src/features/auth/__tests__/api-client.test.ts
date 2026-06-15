import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const clerkSignOut = vi.fn()
const toastWarning = vi.fn()

vi.mock('sonner', () => ({ toast: { warning: (...args: unknown[]) => toastWarning(...args) } }))

const originalFetch = globalThis.fetch
const originalLocation = window.location

function mockLocation(pathname = '/dashboard') {
  const assign = vi.fn()
  Object.defineProperty(window, 'location', {
    configurable: true,
    value: {
      ...originalLocation,
      assign,
      pathname,
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

function setClerk(token: string | null) {
  if (token === null) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    delete (window as any).Clerk
  } else {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ;(window as any).Clerk = {
      session: { getToken: vi.fn().mockResolvedValue(token) },
      signOut: clerkSignOut,
    }
  }
}

beforeEach(() => {
  vi.resetModules()
  clerkSignOut.mockReset()
  clerkSignOut.mockResolvedValue(undefined)
  toastWarning.mockReset()
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  delete (window as any).Clerk
})

afterEach(() => {
  restoreLocation()
  globalThis.fetch = originalFetch
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  delete (window as any).Clerk
})

describe('apiClient — Clerk token attachment', () => {
  it('attaches Bearer token from window.Clerk session', async () => {
    setClerk('clerk-session-token-xyz')
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { data: { hello: 'world' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const { apiClient } = await import('@/lib/core/api-client')
    const result = await apiClient.get<{ hello: string }>('/ping')

    expect(result).toEqual({ hello: 'world' })
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const init = fetchMock.mock.calls[0]![1] as RequestInit
    const headers = init.headers as Headers
    expect(headers.get('Authorization')).toBe('Bearer clerk-session-token-xyz')
  })

  it('sends no Authorization header when Clerk session is absent', async () => {
    setClerk(null)
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { data: {} }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const { apiClient } = await import('@/lib/core/api-client')
    await apiClient.get('/test')

    const init = fetchMock.mock.calls[0]![1] as RequestInit
    const headers = init.headers as Headers
    expect(headers.get('Authorization')).toBeNull()
  })

  it('sends no Authorization header when Clerk session token is null', async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ;(window as any).Clerk = {
      session: { getToken: vi.fn().mockResolvedValue(null) },
      signOut: clerkSignOut,
    }
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { data: {} }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const { apiClient } = await import('@/lib/core/api-client')
    await apiClient.get('/test')

    const init = fetchMock.mock.calls[0]![1] as RequestInit
    const headers = init.headers as Headers
    expect(headers.get('Authorization')).toBeNull()
  })
})

describe('apiClient — 401 handling', () => {
  it('calls Clerk.signOut and redirects to /login on 401', async () => {
    const assign = mockLocation()
    setClerk('valid-token')

    const fetchMock = vi
      .fn()
      .mockResolvedValue(jsonResponse(401, { error: { message: 'expired' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const { apiClient } = await import('@/lib/core/api-client')
    await expect(apiClient.get('/secure')).rejects.toThrow('Unauthorized')

    expect(clerkSignOut).toHaveBeenCalledOnce()
    expect(assign).toHaveBeenCalledWith('/login')
  })

  it('does not redirect when already on /login', async () => {
    const assign = mockLocation('/login')
    setClerk('valid-token')

    const fetchMock = vi
      .fn()
      .mockResolvedValue(jsonResponse(401, { error: { message: 'expired' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const { apiClient } = await import('@/lib/core/api-client')
    await expect(apiClient.get('/secure')).rejects.toThrow('Unauthorized')

    expect(assign).not.toHaveBeenCalled()
  })

  it('does NOT retry on 401 — signs out immediately', async () => {
    mockLocation()
    setClerk('valid-token')

    const fetchMock = vi
      .fn()
      .mockResolvedValue(jsonResponse(401, { error: { message: 'expired' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const { apiClient } = await import('@/lib/core/api-client')
    await expect(apiClient.get('/secure')).rejects.toThrow()

    // Only one fetch call — no retry
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})

describe('apiClient — 403 handling', () => {
  it('does NOT trigger signOut on 403', async () => {
    setClerk('valid-token')
    const fetchMock = vi
      .fn()
      .mockResolvedValue(jsonResponse(403, { error: { message: 'forbidden' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const { apiClient } = await import('@/lib/core/api-client')
    await expect(apiClient.get('/secure')).rejects.toThrow('forbidden')

    expect(clerkSignOut).not.toHaveBeenCalled()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})

describe('apiClient — 429 handling', () => {
  it('shows a warning toast on 429 with Retry-After header', async () => {
    setClerk('valid-token')
    const res = new Response(
      JSON.stringify({ error: { code: 'RATE_LIMITED', message: 'slow down' } }),
      {
        status: 429,
        headers: { 'Content-Type': 'application/json', 'Retry-After': '30' },
      }
    )
    const fetchMock = vi.fn().mockResolvedValue(res)
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const { apiClient } = await import('@/lib/core/api-client')
    await expect(apiClient.get('/fast-endpoint')).rejects.toThrow()

    expect(toastWarning).toHaveBeenCalledWith('Rate limit exceeded. Try again in 30s.')
  })
})
