import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '../store'

// ── Supabase mock ──────────────────────────────────────────────────────────────
const getSession = vi.fn()
const refreshSession = vi.fn()
const signOut = vi.fn()

let capturedAuthListener: ((event: string, session: unknown) => Promise<void>) | null = null

const onAuthStateChange = vi.fn((cb: (event: string, session: unknown) => Promise<void>) => {
  capturedAuthListener = cb
  return { data: { subscription: { unsubscribe: vi.fn() } } }
})

vi.mock('@/lib/supabase', () => ({
  supabase: {
    auth: {
      getSession: (...args: unknown[]) => getSession(...args),
      refreshSession: (...args: unknown[]) => refreshSession(...args),
      signOut: (...args: unknown[]) => signOut(...args),
      onAuthStateChange: (...args: unknown[]) =>
        onAuthStateChange(...(args as Parameters<typeof onAuthStateChange>)),
    },
  },
}))

// ── fetchMe mock ───────────────────────────────────────────────────────────────
const fetchMeMock = vi.fn()

vi.mock('@/api/me', () => ({
  fetchMe: (...args: unknown[]) => fetchMeMock(...args),
}))

// ── helpers ────────────────────────────────────────────────────────────────────
function fakeSession(token = 'tok') {
  return { access_token: token, user: { id: 'u1' } }
}

function fakeIdentity() {
  return { id: 'u1', email: 'a@b.com', roles: ['user'] }
}

beforeEach(() => {
  vi.resetModules()
  getSession.mockReset()
  refreshSession.mockReset()
  signOut.mockReset()
  fetchMeMock.mockReset()
  onAuthStateChange.mockReset()
  capturedAuthListener = null
  onAuthStateChange.mockImplementation((cb) => {
    capturedAuthListener = cb
    return { data: { subscription: { unsubscribe: vi.fn() } } }
  })
  // Reset zustand store to initial state before each test
  useAuthStore.setState({
    session: null,
    identity: null,
    loading: true,
    identityLoading: false,
    initialized: false,
    isAdmin: false,
  })
})

afterEach(() => {
  vi.clearAllMocks()
})

// ── Fix 1: bootstrap unblocks loading before identity fetch ───────────────────
describe('bootstrap', () => {
  it('sets loading=false immediately after getSession, before identity fetch resolves', async () => {
    getSession.mockResolvedValue({ data: { session: fakeSession() } })

    let resolveIdentity!: (v: unknown) => void
    const identityPromise = new Promise((res) => {
      resolveIdentity = res
    })
    fetchMeMock.mockReturnValue(identityPromise)

    const store = useAuthStore.getState()
    const bootstrapPromise = store.bootstrap()

    // Give getSession time to resolve
    await vi.waitFor(() => {
      expect(useAuthStore.getState().loading).toBe(false)
    })

    // Identity should still be null — fetchMe hasn't resolved yet
    expect(useAuthStore.getState().identity).toBeNull()

    // Now resolve identity
    resolveIdentity(fakeIdentity())
    await bootstrapPromise

    await vi.waitFor(() => {
      expect(useAuthStore.getState().identity).toEqual(fakeIdentity())
    })
  })

  it('sets session immediately after getSession resolves', async () => {
    const session = fakeSession()
    getSession.mockResolvedValue({ data: { session } })
    fetchMeMock.mockResolvedValue(fakeIdentity())

    await useAuthStore.getState().bootstrap()

    expect(useAuthStore.getState().session).toEqual(session)
  })

  it('does not fetch identity when no session exists', async () => {
    getSession.mockResolvedValue({ data: { session: null } })

    await useAuthStore.getState().bootstrap()

    expect(fetchMeMock).not.toHaveBeenCalled()
    expect(useAuthStore.getState().loading).toBe(false)
    expect(useAuthStore.getState().identityLoading).toBe(false)
  })

  it('keeps identityLoading=true until identity fetch resolves', async () => {
    getSession.mockResolvedValue({ data: { session: fakeSession() } })

    let resolveIdentity!: (v: unknown) => void
    fetchMeMock.mockReturnValue(
      new Promise((res) => {
        resolveIdentity = res
      })
    )

    useAuthStore.getState().bootstrap()

    await vi.waitFor(() => {
      expect(useAuthStore.getState().loading).toBe(false)
    })
    // Session is resolved but identity is not — identityLoading must stay true so
    // role-gated routes don't redirect prematurely on refresh.
    expect(useAuthStore.getState().identityLoading).toBe(true)

    resolveIdentity(fakeIdentity())

    await vi.waitFor(() => {
      expect(useAuthStore.getState().identityLoading).toBe(false)
    })
    expect(useAuthStore.getState().identity).toEqual(fakeIdentity())
  })
})

// ── Fix 3: TOKEN_REFRESHED does not call fetchMe ──────────────────────────────
describe('onAuthStateChange', () => {
  it('does not call fetchMe on TOKEN_REFRESHED', async () => {
    getSession.mockResolvedValue({ data: { session: fakeSession() } })
    fetchMeMock.mockResolvedValue(fakeIdentity())

    await useAuthStore.getState().bootstrap()
    expect(capturedAuthListener).not.toBeNull()

    fetchMeMock.mockClear()

    const refreshedSession = fakeSession('new-token')
    await capturedAuthListener!('TOKEN_REFRESHED', refreshedSession)

    expect(fetchMeMock).not.toHaveBeenCalled()
    expect(useAuthStore.getState().session).toEqual(refreshedSession)
  })

  it('calls fetchMe on SIGNED_IN', async () => {
    getSession.mockResolvedValue({ data: { session: null } })

    await useAuthStore.getState().bootstrap()
    expect(capturedAuthListener).not.toBeNull()

    fetchMeMock.mockResolvedValue(fakeIdentity())

    await capturedAuthListener!('SIGNED_IN', fakeSession())

    expect(fetchMeMock).toHaveBeenCalledTimes(1)
    expect(useAuthStore.getState().identity).toEqual(fakeIdentity())
  })

  it('clears identity on SIGNED_OUT', async () => {
    getSession.mockResolvedValue({ data: { session: fakeSession() } })
    fetchMeMock.mockResolvedValue(fakeIdentity())
    await useAuthStore.getState().bootstrap()

    await capturedAuthListener!('SIGNED_OUT', null)

    expect(useAuthStore.getState().session).toBeNull()
    expect(useAuthStore.getState().identity).toBeNull()
  })
})
