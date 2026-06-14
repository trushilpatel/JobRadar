import { toast } from 'sonner'
import { parseApiError } from './api-error'

type HttpMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'

const BASE = '/api'

// Clerk is loaded by ClerkProvider and exposes session on window.Clerk.
// We access it here (non-React context) without importing @clerk/react,
// which avoids a React hook call outside a component.
interface ClerkWindow {
  Clerk?: {
    session?: { getToken: () => Promise<string | null> }
    signOut: () => Promise<void>
  }
}

function getClerkWindow(): ClerkWindow {
  return window as unknown as ClerkWindow
}

async function getClerkToken(): Promise<string | null> {
  return getClerkWindow().Clerk?.session?.getToken() ?? null
}

function signOutAndRedirect(): void {
  getClerkWindow()
    .Clerk?.signOut()
    .catch(() => {})
  if (typeof window !== 'undefined' && window.location.pathname !== '/login') {
    window.location.assign('/login')
  }
}

async function authHeader(): Promise<Record<string, string>> {
  const token = await getClerkToken()
  return token ? { Authorization: `Bearer ${token}` } : {}
}

function buildUrl(path: string): string {
  if (path.startsWith('http://') || path.startsWith('https://')) return path
  if (path.startsWith('/')) return `${BASE}${path}`
  return `${BASE}/${path}`
}

async function unwrap<T>(res: Response): Promise<T> {
  const text = await res.text()
  if (!text) return undefined as T
  let body: unknown
  try {
    body = JSON.parse(text)
  } catch {
    return text as T
  }
  if (body && typeof body === 'object') {
    const obj = body as Record<string, unknown>
    // Paged envelope: {data, total, page, limit} — return full body so callers get pagination meta.
    if ('data' in obj && 'total' in obj && 'page' in obj && 'limit' in obj) {
      return body as T
    }
    if ('data' in obj) {
      return obj.data as T
    }
  }
  return body as T
}

// REQUEST_TIMEOUT_MS is how long rawFetch waits before aborting a stalled
// request. 15 seconds covers slow API starts without causing false positives
// on normal traffic. The spinner would otherwise hang indefinitely if the
// backend drops the TCP connection mid-response (e.g., pod crash, DB lock).
const REQUEST_TIMEOUT_MS = 15_000

async function rawFetch(method: HttpMethod, path: string, init?: RequestInit): Promise<Response> {
  const headers = new Headers(init?.headers)
  const auth = await authHeader()
  for (const [k, v] of Object.entries(auth)) {
    headers.set(k, v)
  }
  if (init?.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const controller = new AbortController()
  let timedOut = false
  const timeoutId = setTimeout(() => {
    timedOut = true
    controller.abort()
  }, REQUEST_TIMEOUT_MS)

  // Honour a caller-supplied signal by aborting on either trigger.
  if (init?.signal) {
    if (init.signal.aborted) {
      clearTimeout(timeoutId)
      throw new DOMException('Aborted', 'AbortError')
    }
    init.signal.addEventListener('abort', () => controller.abort())
  }

  try {
    return await fetch(buildUrl(path), {
      ...init,
      method,
      headers,
      signal: controller.signal,
    })
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') {
      // Only surface a user-visible timeout message when our timer fired.
      // Aborts triggered by the caller's signal (e.g. TanStack Query
      // cancellation) are re-thrown as-is so they propagate correctly.
      if (timedOut) throw new Error('Request timed out')
      throw err
    }
    throw err
  } finally {
    clearTimeout(timeoutId)
  }
}

async function request<T>(method: HttpMethod, path: string, init?: RequestInit): Promise<T> {
  let res: Response
  try {
    res = await rawFetch(method, path, init)
  } catch (err) {
    throw err instanceof Error ? err : new Error('Network error')
  }

  if (res.status === 401) {
    signOutAndRedirect()
    throw new Error('Unauthorized')
  }

  if (res.status === 429) {
    const retryAfter = res.headers.get('Retry-After')
    const seconds = retryAfter ? parseInt(retryAfter, 10) : null
    const msg = seconds
      ? `Rate limit exceeded. Try again in ${seconds}s.`
      : 'Rate limit exceeded. Please slow down.'
    toast.warning(msg)
    throw await parseApiError(res, msg)
  }

  if (!res.ok) {
    throw await parseApiError(res, `Request failed: ${res.status}`)
  }

  return unwrap<T>(res)
}

export const apiClient = {
  get<T>(path: string, init?: RequestInit): Promise<T> {
    return request<T>('GET', path, init)
  },
  post<T>(path: string, body?: unknown, init?: RequestInit): Promise<T> {
    return request<T>('POST', path, {
      ...init,
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  },
  put<T>(path: string, body?: unknown, init?: RequestInit): Promise<T> {
    return request<T>('PUT', path, {
      ...init,
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  },
  patch<T>(path: string, body?: unknown, init?: RequestInit): Promise<T> {
    return request<T>('PATCH', path, {
      ...init,
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  },
  delete<T>(path: string, body?: unknown, init?: RequestInit): Promise<T> {
    return request<T>('DELETE', path, {
      ...init,
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  },
}
