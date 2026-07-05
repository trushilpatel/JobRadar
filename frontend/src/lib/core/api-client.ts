import { toast } from 'sonner'
import { parseApiError } from './api-error'

type HttpMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'

const BASE = '/api'

// Clerk is loaded by ClerkProvider and exposes session on window.Clerk.
// We access it here (non-React context) without importing @clerk/react,
// which avoids a React hook call outside a component.
interface ClerkWindow {
  Clerk?: {
    session?: { getToken: (opts?: { template?: string }) => Promise<string | null> }
    signOut: () => Promise<void>
  }
}

function getClerkWindow(): ClerkWindow {
  return window as unknown as ClerkWindow
}

async function getClerkToken(): Promise<string | null> {
  return getClerkWindow().Clerk?.session?.getToken({ template: 'job-radar-jwt-template' }) ?? null
}

function signOutAndRedirect(): void {
  const clerk = getClerkWindow().Clerk
  // If Clerk hasn't initialized yet (e.g. a race-condition request fired before
  // ClerkProvider bootstrapped), skip the redirect entirely — the user is not
  // logged out; re-firing after init will re-auth correctly.
  if (!clerk) return
  clerk.signOut().catch(() => {})
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
  if (init?.body && !(init.body instanceof FormData) && !headers.has('Content-Type')) {
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

// fetchOk runs the shared request pipeline (auth, timeout, 401/429/error-status
// handling) and returns the raw ok Response — shared by request<T> (which
// JSON-unwraps it) and getBlob (which reads it as binary instead). Keeping
// this in one place means blob downloads never drift from the 401-redirect /
// rate-limit / timeout behaviour every other call gets.
async function fetchOk(method: HttpMethod, path: string, init?: RequestInit): Promise<Response> {
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

  return res
}

async function request<T>(method: HttpMethod, path: string, init?: RequestInit): Promise<T> {
  const res = await fetchOk(method, path, init)
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
  upload<T>(path: string, form: FormData, init?: RequestInit): Promise<T> {
    return request<T>('POST', path, { ...init, body: form })
  },
  // getBlob is for binary downloads (files) — unwrap()'s res.text() + JSON.parse
  // would corrupt binary bytes, so this skips it and returns the raw Blob.
  getBlob(path: string, init?: RequestInit): Promise<Blob> {
    return fetchOk('GET', path, init).then((res) => res.blob())
  },
}
