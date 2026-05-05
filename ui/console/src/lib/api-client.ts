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

async function signOutAndRedirect(): Promise<void> {
  try {
    await supabase.auth.signOut({ scope: 'global' })
  } catch {
    // ignore — we're redirecting anyway
  }
  if (typeof window !== 'undefined' && window.location.pathname !== '/login') {
    window.location.assign('/login')
  }
}

async function authHeader(): Promise<Record<string, string>> {
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

async function rawFetch(method: HttpMethod, path: string, init?: RequestInit): Promise<Response> {
  const headers = new Headers(init?.headers)
  const auth = await authHeader()
  for (const [k, v] of Object.entries(auth)) headers.set(k, v)
  if (init?.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  return fetch(buildUrl(path), { ...init, method, headers })
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
      await signOutAndRedirect()
      throw new Error('Unauthorized')
    }
    try {
      res = await rawFetch(method, path, init)
    } catch (err) {
      throw err instanceof Error ? err : new Error('Network error')
    }
    if (res.status === 401) {
      await signOutAndRedirect()
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
