import { toast } from 'sonner'
import { supabase } from './supabase'
import { parseApiError } from './api-error'

type HttpMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'

const BASE = '/api'

let pendingRefresh: Promise<boolean> | null = null

async function refreshOnce(): Promise<boolean> {
  if (pendingRefresh) return pendingRefresh
  pendingRefresh = (async () => {
    try {
      const { data, error } = await supabase.auth.refreshSession()
      return !error && !!data.session
    } catch {
      return false
    } finally {
      setTimeout(() => {
        pendingRefresh = null
      }, 0)
    }
  })()
  return pendingRefresh
}

function signOutAndRedirect(): void {
  // Fire-and-forget — don't await signOut so the redirect is never blocked by a slow/hanging network call
  supabase.auth.signOut({ scope: 'global' }).catch(() => {})
  if (typeof window !== 'undefined' && window.location.pathname !== '/login') {
    window.location.assign('/login')
  }
}

async function authHeader(): Promise<Record<string, string>> {
  // Don't call refreshSession() here — Supabase's autoRefreshToken handles proactive scheduling.
  // Calling it manually races with the internal timer and can burn the refresh token.
  const { data } = await supabase.auth.getSession()
  const token = data.session?.access_token
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
  for (const [k, v] of Object.entries(auth)) headers.set(k, v)
  if (init?.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const controller = new AbortController()
  const timeoutId = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS)
  // Honour a caller-supplied signal by aborting on either trigger.
  if (init?.signal) {
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
      throw new Error('Request timed out')
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
    const refreshed = await refreshOnce()
    if (!refreshed) {
      signOutAndRedirect()
      throw new Error('Unauthorized')
    }
    try {
      res = await rawFetch(method, path, init)
    } catch (err) {
      throw err instanceof Error ? err : new Error('Network error')
    }
    if (res.status === 401) {
      signOutAndRedirect()
      throw new Error('Unauthorized')
    }
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

export const __testing = {
  resetPendingRefresh: () => {
    pendingRefresh = null
  },
}
