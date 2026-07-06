import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

vi.mock('sonner', () => ({ toast: { warning: vi.fn(), error: vi.fn(), success: vi.fn() } }))

import { apiClient } from '@/lib/core/api-client'
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
const clerkSignOut = vi.fn().mockResolvedValue(undefined)

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
  vi.clearAllMocks()
  setClerk('tok-123')
  clerkSignOut.mockResolvedValue(undefined)
  global.fetch = fetchMock as unknown as typeof fetch
  // window.location.assign for signOutAndRedirect
  Object.defineProperty(window, 'location', {
    value: { pathname: '/dashboard', assign: vi.fn() },
    writable: true,
  })
})

afterEach(() => {
  vi.useRealTimers()
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  delete (window as any).Clerk
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

  it('sets Authorization header from Clerk session', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { data: null }))
    await apiClient.get('/me')
    const init = fetchMock.mock.calls[0]![1] as RequestInit
    expect((init.headers as Headers).get('Authorization')).toBe('Bearer tok-123')
  })

  it('omits Authorization header when no Clerk session', async () => {
    setClerk(null)
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

// ── 401 handling (Clerk: sign out immediately, no retry) ─────────────────────
describe('apiClient — 401 handling', () => {
  it('signs out via Clerk and throws Unauthorized on 401', async () => {
    fetchMock.mockResolvedValue(emptyResponse(401))
    await expect(apiClient.get('/secure')).rejects.toThrow('Unauthorized')
    expect(clerkSignOut).toHaveBeenCalledOnce()
    expect(
      (window.location as unknown as { assign: ReturnType<typeof vi.fn> }).assign
    ).toHaveBeenCalledWith('/login')
  })

  it('does NOT retry on 401 — only one fetch call', async () => {
    fetchMock.mockResolvedValue(emptyResponse(401))
    await expect(apiClient.get('/secure')).rejects.toThrow('Unauthorized')
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('does not redirect when already on /login', async () => {
    Object.defineProperty(window, 'location', {
      value: { pathname: '/login', assign: vi.fn() },
      writable: true,
    })
    fetchMock.mockResolvedValue(emptyResponse(401))
    await expect(apiClient.get('/secure')).rejects.toThrow('Unauthorized')
    expect(
      (window.location as unknown as { assign: ReturnType<typeof vi.fn> }).assign
    ).not.toHaveBeenCalled()
  })

  it('does NOT trigger sign-out on 403', async () => {
    fetchMock.mockResolvedValue(jsonResponse(403, { error: { message: 'forbidden' } }))
    await expect(apiClient.get('/secure')).rejects.toThrow('forbidden')
    expect(clerkSignOut).not.toHaveBeenCalled()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('does not redirect when Clerk has not initialized yet (window.Clerk undefined)', async () => {
    // Simulate the race-condition window: Clerk JS hasn't bootstrapped yet so
    // window.Clerk is undefined. A 401 must NOT cause signOut or a /login
    // redirect — the user is not logged out, Clerk simply hasn't loaded.
    setClerk(null)
    fetchMock.mockResolvedValue(emptyResponse(401))
    await expect(apiClient.get('/secure')).rejects.toThrow('Unauthorized')
    expect(clerkSignOut).not.toHaveBeenCalled()
    expect(
      (window.location as unknown as { assign: ReturnType<typeof vi.fn> }).assign
    ).not.toHaveBeenCalled()
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

describe('apiClient.upload', () => {
  it('sends FormData without JSON.stringify and without forcing a Content-Type header', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ data: { id: 'r1' } }), { status: 201 })
    )
    const form = new FormData()
    form.append('file', new Blob(['data']), 'resume.pdf')

    await apiClient.upload('/resumes', form)

    const init = fetchMock.mock.calls[0]![1] as RequestInit
    expect(init?.body).toBe(form)
    const headers = new Headers(init?.headers)
    expect(headers.has('Content-Type')).toBe(false)
  })

  it('resolves the unwrapped response data', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ data: { id: 'r1' } }), { status: 201 })
    )
    const form = new FormData()
    form.append('file', new Blob(['data']), 'resume.pdf')

    const result = await apiClient.upload<{ id: string }>('/resumes', form)
    expect(result).toEqual({ id: 'r1' })
  })
})
