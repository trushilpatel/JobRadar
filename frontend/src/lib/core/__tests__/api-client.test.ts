import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// ── Mocks ────────────────────────────────────────────────────────────────────
// supabase + sessionRef are module-level dependencies of api-client. We mock
// them so we can drive the 401-refresh path and the auth header without a real
// Supabase client.
const refreshSessionMock = vi.fn()
const signOutMock = vi.fn()

vi.mock('@/lib/core/supabase', () => ({
  supabase: {
    auth: {
      refreshSession: () => refreshSessionMock(),
      signOut: (opts: unknown) => signOutMock(opts),
    },
  },
}))

const sessionGetMock = vi.fn<() => { access_token: string } | null>()
vi.mock('@/lib/core/session-ref', () => ({
  sessionRef: {
    get: () => sessionGetMock(),
    set: vi.fn(),
  },
}))

vi.mock('sonner', () => ({ toast: { warning: vi.fn(), error: vi.fn(), success: vi.fn() } }))

import { apiClient, __testing } from '@/lib/core/api-client'
import { ApiValidationError } from '@/lib/core/api-error'
import { toast } from 'sonner'

// Helpers to build Response objects.
function jsonResponse(
  status: number,
  body: unknown,
  headers: Record<string, string> = {}
): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json', ...headers },
  })
}
function emptyResponse(status: number): Response {
  return new Response('', { status })
}
function textResponse(status: number, text: string): Response {
  return new Response(text, { status, headers: { 'Content-Type': 'text/plain' } })
}

const fetchMock = vi.fn()

beforeEach(() => {
  vi.clearAllMocks()
  __testing.resetPendingRefresh()
  sessionGetMock.mockReturnValue({ access_token: 'tok-123' })
  refreshSessionMock.mockResolvedValue({ data: { session: { access_token: 'new' } }, error: null })
  signOutMock.mockResolvedValue(undefined)
  global.fetch = fetchMock as unknown as typeof fetch
  // window.location.assign for signOutAndRedirect
  Object.defineProperty(window, 'location', {
    value: { pathname: '/dashboard', assign: vi.fn() },
    writable: true,
  })
})

afterEach(() => {
  vi.useRealTimers()
})

// ── Envelope unwrapping ──────────────────────────────────────────────────────
describe('apiClient — envelope unwrapping', () => {
  it('unwraps {data} envelope', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { data: { id: 'x' } }))
    const res = await apiClient.get<{ id: string }>('/thing')
    expect(res).toEqual({ id: 'x' })
  })

  it('returns full body for paged envelope {data,total,page,limit}', async () => {
    const body = { data: [1, 2], total: 2, page: 1, limit: 20 }
    fetchMock.mockResolvedValue(jsonResponse(200, body))
    const res = await apiClient.get<typeof body>('/list')
    expect(res).toEqual(body)
  })

  it('returns raw body when there is no data key', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { foo: 'bar' }))
    const res = await apiClient.get<{ foo: string }>('/raw')
    expect(res).toEqual({ foo: 'bar' })
  })

  it('returns undefined for an empty body', async () => {
    fetchMock.mockResolvedValue(emptyResponse(200))
    const res = await apiClient.get<unknown>('/empty')
    expect(res).toBeUndefined()
  })

  it('returns the raw text when body is not JSON', async () => {
    fetchMock.mockResolvedValue(textResponse(200, 'plain text'))
    const res = await apiClient.get<string>('/text')
    expect(res).toBe('plain text')
  })

  it('returns a primitive JSON body unchanged (not an object)', async () => {
    fetchMock.mockResolvedValue(new Response('42', { status: 200 }))
    const res = await apiClient.get<number>('/num')
    expect(res).toBe(42)
  })
})

// ── URL building + headers ───────────────────────────────────────────────────
describe('apiClient — URL building & headers', () => {
  it('prefixes a leading-slash path with /api', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { data: null }))
    await apiClient.get('/dashboard/market')
    expect(fetchMock).toHaveBeenCalledWith('/api/dashboard/market', expect.anything())
  })

  it('prefixes a non-slash path with /api/', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { data: null }))
    await apiClient.get('thing')
    expect(fetchMock).toHaveBeenCalledWith('/api/thing', expect.anything())
  })

  it('passes through absolute http(s) URLs untouched', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { data: null }))
    await apiClient.get('https://example.com/x')
    expect(fetchMock).toHaveBeenCalledWith('https://example.com/x', expect.anything())
  })

  it('sets Authorization header from sessionRef', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { data: null }))
    await apiClient.get('/me')
    const init = fetchMock.mock.calls[0]![1] as RequestInit
    expect((init.headers as Headers).get('Authorization')).toBe('Bearer tok-123')
  })

  it('omits Authorization header when no session token', async () => {
    sessionGetMock.mockReturnValue(null)
    fetchMock.mockResolvedValue(jsonResponse(200, { data: null }))
    await apiClient.get('/me')
    const init = fetchMock.mock.calls[0]![1] as RequestInit
    expect((init.headers as Headers).has('Authorization')).toBe(false)
  })

  it('sets Content-Type when a body is present', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { data: null }))
    await apiClient.post('/thing', { a: 1 })
    const init = fetchMock.mock.calls[0]![1] as RequestInit
    expect((init.headers as Headers).get('Content-Type')).toBe('application/json')
    expect(init.body).toBe(JSON.stringify({ a: 1 }))
  })
})

// ── HTTP verbs ───────────────────────────────────────────────────────────────
describe('apiClient — verbs', () => {
  it.each([
    ['post', 'POST'],
    ['put', 'PUT'],
    ['patch', 'PATCH'],
    ['delete', 'DELETE'],
  ] as const)('%s sends method %s and serialises body', async (verb, method) => {
    fetchMock.mockResolvedValue(jsonResponse(200, { data: { ok: true } }))
    const fn = apiClient[verb] as (p: string, b?: unknown) => Promise<unknown>
    const res = await fn('/thing', { x: 1 })
    expect(res).toEqual({ ok: true })
    const init = fetchMock.mock.calls[0]![1] as RequestInit
    expect(init.method).toBe(method)
    expect(init.body).toBe(JSON.stringify({ x: 1 }))
  })

  it('sends undefined body when none provided (no Content-Type)', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { data: null }))
    await apiClient.post('/thing')
    const init = fetchMock.mock.calls[0]![1] as RequestInit
    expect(init.body).toBeUndefined()
    expect((init.headers as Headers).has('Content-Type')).toBe(false)
  })

  it('get sends GET method', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { data: null }))
    await apiClient.get('/thing')
    const init = fetchMock.mock.calls[0]![1] as RequestInit
    expect(init.method).toBe('GET')
  })
})

// ── Error parsing ────────────────────────────────────────────────────────────
describe('apiClient — error handling', () => {
  it('throws ApiValidationError on a 422 with errors array', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(422, { errors: [{ field: 'email', code: 'required', message: 'Required' }] })
    )
    await expect(apiClient.post('/x', {})).rejects.toBeInstanceOf(ApiValidationError)
  })

  it('throws a plain Error with the status fallback on a 500', async () => {
    fetchMock.mockResolvedValue(jsonResponse(500, { oops: true }))
    await expect(apiClient.get('/x')).rejects.toThrow('Request failed: 500')
  })

  it('wraps a thrown non-Error from fetch as Network error', async () => {
    fetchMock.mockRejectedValue('boom')
    await expect(apiClient.get('/x')).rejects.toThrow('Network error')
  })

  it('re-throws an Error from fetch unchanged', async () => {
    fetchMock.mockRejectedValue(new TypeError('Failed to fetch'))
    await expect(apiClient.get('/x')).rejects.toThrow('Failed to fetch')
  })
})

// ── 429 rate limit ───────────────────────────────────────────────────────────
describe('apiClient — 429 rate limiting', () => {
  it('toasts with Retry-After seconds and throws', async () => {
    fetchMock.mockResolvedValue(jsonResponse(429, {}, { 'Retry-After': '30' }))
    await expect(apiClient.get('/x')).rejects.toThrow()
    expect(vi.mocked(toast.warning)).toHaveBeenCalledWith('Rate limit exceeded. Try again in 30s.')
  })

  it('toasts a generic message when there is no Retry-After header', async () => {
    fetchMock.mockResolvedValue(jsonResponse(429, {}))
    await expect(apiClient.get('/x')).rejects.toThrow()
    expect(vi.mocked(toast.warning)).toHaveBeenCalledWith('Rate limit exceeded. Please slow down.')
  })
})

// ── 401 refresh + retry ──────────────────────────────────────────────────────
describe('apiClient — 401 refresh & retry', () => {
  it('refreshes the session and retries once on 401, then succeeds', async () => {
    fetchMock
      .mockResolvedValueOnce(emptyResponse(401))
      .mockResolvedValueOnce(jsonResponse(200, { data: { ok: true } }))
    const res = await apiClient.get<{ ok: boolean }>('/secure')
    expect(res).toEqual({ ok: true })
    expect(refreshSessionMock).toHaveBeenCalledTimes(1)
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('signs out and throws Unauthorized when refresh fails', async () => {
    fetchMock.mockResolvedValue(emptyResponse(401))
    refreshSessionMock.mockResolvedValue({ data: { session: null }, error: { message: 'nope' } })
    await expect(apiClient.get('/secure')).rejects.toThrow('Unauthorized')
    expect(signOutMock).toHaveBeenCalledWith({ scope: 'global' })
    expect(
      (window.location as unknown as { assign: ReturnType<typeof vi.fn> }).assign
    ).toHaveBeenCalledWith('/login')
  })

  it('returns false from refresh when refreshSession throws', async () => {
    fetchMock.mockResolvedValue(emptyResponse(401))
    refreshSessionMock.mockRejectedValue(new Error('lock'))
    await expect(apiClient.get('/secure')).rejects.toThrow('Unauthorized')
    expect(signOutMock).toHaveBeenCalled()
  })

  it('signs out and throws when the retried request is also 401', async () => {
    fetchMock.mockResolvedValueOnce(emptyResponse(401)).mockResolvedValueOnce(emptyResponse(401))
    await expect(apiClient.get('/secure')).rejects.toThrow('Unauthorized')
    expect(signOutMock).toHaveBeenCalled()
  })

  it('wraps a non-Error thrown by the retried fetch as Network error', async () => {
    fetchMock.mockResolvedValueOnce(emptyResponse(401)).mockRejectedValueOnce('boom')
    await expect(apiClient.get('/secure')).rejects.toThrow('Network error')
  })

  it('does not redirect when already on /login', async () => {
    Object.defineProperty(window, 'location', {
      value: { pathname: '/login', assign: vi.fn() },
      writable: true,
    })
    fetchMock.mockResolvedValue(emptyResponse(401))
    refreshSessionMock.mockResolvedValue({ data: { session: null }, error: { message: 'no' } })
    await expect(apiClient.get('/secure')).rejects.toThrow('Unauthorized')
    expect(
      (window.location as unknown as { assign: ReturnType<typeof vi.fn> }).assign
    ).not.toHaveBeenCalled()
  })

  it('dedupes concurrent refreshes into a single refreshSession call', async () => {
    // Both requests get a 401 first, then succeed; a single in-flight refresh
    // should be shared between them. Each call returns a FRESH response so the
    // body is never read twice.
    let n = 0
    fetchMock.mockImplementation(() => {
      n += 1
      return Promise.resolve(
        n <= 2 ? emptyResponse(401) : jsonResponse(200, { data: { ok: true } })
      )
    })
    let resolveRefresh!: (v: unknown) => void
    refreshSessionMock.mockReturnValue(
      new Promise((r) => {
        resolveRefresh = r
      })
    )
    const p1 = apiClient.get('/a')
    const p2 = apiClient.get('/b')
    await Promise.resolve()
    resolveRefresh({ data: { session: { access_token: 'new' } }, error: null })
    await Promise.all([p1, p2])
    expect(refreshSessionMock).toHaveBeenCalledTimes(1)
  })
})

// ── Timeout / abort ──────────────────────────────────────────────────────────
describe('apiClient — timeout & abort', () => {
  it('throws "Request timed out" when the internal timer fires', async () => {
    vi.useFakeTimers()
    fetchMock.mockImplementation(
      (_url, init: RequestInit) =>
        new Promise((_resolve, reject) => {
          init.signal?.addEventListener('abort', () => {
            reject(new DOMException('Aborted', 'AbortError'))
          })
        })
    )
    const p = apiClient.get('/slow')
    const expectation = expect(p).rejects.toThrow('Request timed out')
    await vi.advanceTimersByTimeAsync(15_000)
    await expectation
  })

  it('aborts immediately (without calling fetch) when the caller signal is already aborted', async () => {
    const ctrl = new AbortController()
    ctrl.abort()
    // rawFetch throws a DOMException('AbortError'); request() wraps any
    // non-Error as 'Network error'. In this runtime DOMException is not an
    // Error instance, so the surfaced message is 'Network error'. The key
    // assertion is that fetch was never invoked.
    await expect(apiClient.get('/x', { signal: ctrl.signal })).rejects.toThrow()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('rejects on a caller-triggered abort (not a timeout)', async () => {
    const ctrl = new AbortController()
    fetchMock.mockImplementation(
      (_url: string, init: RequestInit) =>
        new Promise((_resolve, reject) => {
          init.signal?.addEventListener('abort', () => {
            reject(new DOMException('Aborted', 'AbortError'))
          })
        })
    )
    const p = apiClient.get('/x', { signal: ctrl.signal })
    // Caller-triggered aborts are re-thrown (not the "Request timed out"
    // message reserved for the internal timer).
    const expectation = expect(p).rejects.not.toThrow('Request timed out')
    ctrl.abort()
    await expectation
  })
})
