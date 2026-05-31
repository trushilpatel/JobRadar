import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '../store/auth.store'
import { sessionRef } from '@/lib/core/session-ref'

// ── Supabase mock ──────────────────────────────────────────────────────────────
const getSession = vi.fn()
const signInWithOtp = vi.fn()
const signOut = vi.fn()

let capturedAuthListener: ((event: string, session: unknown) => Promise<void>) | null = null

const onAuthStateChange = vi.fn((cb: (event: string, session: unknown) => Promise<void>) => {
  capturedAuthListener = cb
  return { data: { subscription: { unsubscribe: vi.fn() } } }
})

vi.mock('@/lib/core/supabase', () => ({
  supabase: {
    auth: {
      getSession: (...args: unknown[]) => getSession(...args),
      signInWithOtp: (...args: unknown[]) => signInWithOtp(...args),
      signOut: (...args: unknown[]) => signOut(...args),
      onAuthStateChange: (...args: unknown[]) =>
        onAuthStateChange(...(args as Parameters<typeof onAuthStateChange>)),
    },
  },
}))

// ── fetchMe mock ───────────────────────────────────────────────────────────────
const fetchMeMock = vi.fn()

vi.mock('@/features/auth/api/me.api', () => ({
  fetchMe: (...args: unknown[]) => fetchMeMock(...args),
}))

function fakeSession(token = 'tok') {
  return { access_token: token, user: { id: 'u1' } }
}

function adminIdentity() {
  return { user_id: 'u1', email: 'a@b.com', display_name: 'A', status: 'active', roles: ['admin'] }
}

function userIdentity() {
  return { user_id: 'u1', email: 'a@b.com', display_name: 'A', status: 'active', roles: ['user'] }
}

beforeEach(() => {
  vi.resetModules()
  getSession.mockReset()
  signInWithOtp.mockReset()
  signOut.mockReset()
  fetchMeMock.mockReset()
  onAuthStateChange.mockReset()
  capturedAuthListener = null
  onAuthStateChange.mockImplementation((cb) => {
    capturedAuthListener = cb
    return { data: { subscription: { unsubscribe: vi.fn() } } }
  })
  sessionRef.set(null)
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

// ── signInWithEmail ──────────────────────────────────────────────────────────
describe('signInWithEmail', () => {
  it('calls signInWithOtp with an emailRedirectTo pointing at /auth/callback', async () => {
    signInWithOtp.mockResolvedValue({ error: null })
    await useAuthStore.getState().signInWithEmail('a@b.com')
    expect(signInWithOtp).toHaveBeenCalledTimes(1)
    const arg = signInWithOtp.mock.calls[0]![0] as {
      email: string
      options: { emailRedirectTo: string }
    }
    expect(arg.email).toBe('a@b.com')
    expect(arg.options.emailRedirectTo).toContain('/auth/callback')
  })

  it('throws the supabase error when sign-in fails', async () => {
    signInWithOtp.mockResolvedValue({ error: new Error('otp failed') })
    await expect(useAuthStore.getState().signInWithEmail('a@b.com')).rejects.toThrow('otp failed')
  })
})

// ── signOut ──────────────────────────────────────────────────────────────────
describe('signOut', () => {
  it('clears session, identity, isAdmin and the sessionRef', async () => {
    sessionRef.set(fakeSession() as never)
    useAuthStore.setState({
      session: fakeSession() as never,
      identity: adminIdentity() as never,
      isAdmin: true,
      identityLoading: true,
    })
    signOut.mockResolvedValue({ error: null })

    await useAuthStore.getState().signOut()

    expect(signOut).toHaveBeenCalledWith({ scope: 'global' })
    const state = useAuthStore.getState()
    expect(state.session).toBeNull()
    expect(state.identity).toBeNull()
    expect(state.isAdmin).toBe(false)
    expect(state.identityLoading).toBe(false)
    expect(sessionRef.get()).toBeNull()
  })
})

// ── refreshIdentity ──────────────────────────────────────────────────────────
describe('refreshIdentity', () => {
  it('loads identity and derives isAdmin=true for an admin', async () => {
    fetchMeMock.mockResolvedValue(adminIdentity())
    await useAuthStore.getState().refreshIdentity()
    const state = useAuthStore.getState()
    expect(state.identity).toEqual(adminIdentity())
    expect(state.isAdmin).toBe(true)
    expect(state.identityLoading).toBe(false)
  })

  it('derives isAdmin=false for a non-admin identity', async () => {
    fetchMeMock.mockResolvedValue(userIdentity())
    await useAuthStore.getState().refreshIdentity()
    expect(useAuthStore.getState().isAdmin).toBe(false)
  })

  it('sets identity=null and isAdmin=false when fetchMe rejects (loadIdentity catch)', async () => {
    fetchMeMock.mockRejectedValue(new Error('401'))
    await useAuthStore.getState().refreshIdentity()
    const state = useAuthStore.getState()
    expect(state.identity).toBeNull()
    expect(state.isAdmin).toBe(false)
    expect(state.identityLoading).toBe(false)
  })
})

// ── bootstrap branch coverage ────────────────────────────────────────────────
describe('bootstrap branches', () => {
  it('returns a no-op and skips setup when already initialized', async () => {
    useAuthStore.setState({ initialized: true })
    const cleanup = await useAuthStore.getState().bootstrap()
    expect(getSession).not.toHaveBeenCalled()
    expect(onAuthStateChange).not.toHaveBeenCalled()
    expect(typeof cleanup).toBe('function')
    expect(() => cleanup()).not.toThrow()
  })

  it('returns a cleanup that unsubscribes the auth listener', async () => {
    const unsubscribe = vi.fn()
    onAuthStateChange.mockImplementation((cb) => {
      capturedAuthListener = cb
      return { data: { subscription: { unsubscribe } } }
    })
    getSession.mockResolvedValue({ data: { session: null } })

    const cleanup = await useAuthStore.getState().bootstrap()
    cleanup()
    expect(unsubscribe).toHaveBeenCalledTimes(1)
  })

  it('does not apply identity if the session was cleared before the fetch resolved', async () => {
    getSession.mockResolvedValue({ data: { session: fakeSession() } })

    let resolveIdentity!: (v: unknown) => void
    fetchMeMock.mockReturnValue(
      new Promise((res) => {
        resolveIdentity = res
      })
    )

    await useAuthStore.getState().bootstrap()
    // Simulate the user signing out while identity is still in-flight.
    useAuthStore.setState({ session: null })

    resolveIdentity(adminIdentity())
    await vi.waitFor(() => {
      expect(useAuthStore.getState().identityLoading).toBe(false)
    })
    // identity must NOT have been applied because session is gone
    expect(useAuthStore.getState().identity).toBeNull()
    expect(useAuthStore.getState().isAdmin).toBe(false)
  })

  it('does not load identity in the listener when newSession is null', async () => {
    getSession.mockResolvedValue({ data: { session: null } })
    await useAuthStore.getState().bootstrap()
    expect(capturedAuthListener).not.toBeNull()

    fetchMeMock.mockClear()
    await capturedAuthListener!('SIGNED_OUT', null)
    expect(fetchMeMock).not.toHaveBeenCalled()
    expect(useAuthStore.getState().isAdmin).toBe(false)
  })
})
