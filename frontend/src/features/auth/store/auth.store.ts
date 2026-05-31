import { create } from 'zustand'
import type { Session } from '@supabase/supabase-js'
import { supabase } from '@/lib/core/supabase'
import { sessionRef } from '@/lib/core/session-ref'
import { fetchMe } from '../api/me.api'
import type { Identity } from '@/types'

interface AuthState {
  session: Session | null
  identity: Identity | null
  loading: boolean
  identityLoading: boolean
  initialized: boolean
  isAdmin: boolean
  bootstrap: () => Promise<() => void>
  signInWithEmail: (email: string) => Promise<void>
  signOut: () => Promise<void>
  refreshIdentity: () => Promise<void>
}

async function loadIdentity(): Promise<Identity | null> {
  try {
    return await fetchMe()
  } catch {
    return null
  }
}

function isAdminIdentity(identity: Identity | null): boolean {
  return identity?.roles.includes('admin') ?? false
}

export const useAuthStore = create<AuthState>((set, get) => ({
  session: null,
  identity: null,
  loading: true,
  identityLoading: false,
  initialized: false,
  isAdmin: false,

  bootstrap: async () => {
    if (get().initialized) {
      return () => {}
    }
    set({ initialized: true })
    const { data } = await supabase.auth.getSession()
    const session = data.session

    // Unblock rendering immediately — identity loads in background.
    // identityLoading guards role-gated routes from redirecting before identity arrives.
    sessionRef.set(session)
    set({ session, loading: false, identityLoading: !!session })

    if (session) {
      loadIdentity().then((identity) => {
        if (get().session) {
          set({ identity, isAdmin: isAdminIdentity(identity), identityLoading: false })
        } else {
          set({ identityLoading: false })
        }
      })
    }

    const { data: sub } = supabase.auth.onAuthStateChange(async (event, newSession) => {
      sessionRef.set(newSession)
      if (event === 'TOKEN_REFRESHED') {
        set({ session: newSession })
        return
      }
      set({ identityLoading: !!newSession })
      let newIdentity: Identity | null = null
      if (newSession) {
        newIdentity = await loadIdentity()
      }
      set({
        session: newSession,
        identity: newIdentity,
        loading: false,
        identityLoading: false,
        isAdmin: isAdminIdentity(newIdentity),
      })
    })
    return () => {
      sub.subscription.unsubscribe()
    }
  },

  signInWithEmail: async (email: string) => {
    const redirectTo = `${window.location.origin}/auth/callback`
    const { error } = await supabase.auth.signInWithOtp({
      email,
      options: { emailRedirectTo: redirectTo },
    })
    if (error) throw error
  },

  signOut: async () => {
    await supabase.auth.signOut({ scope: 'global' })
    sessionRef.set(null)
    set({ session: null, identity: null, isAdmin: false, identityLoading: false })
  },

  refreshIdentity: async () => {
    set({ identityLoading: true })
    const identity = await loadIdentity()
    set({ identity, isAdmin: isAdminIdentity(identity), identityLoading: false })
  },
}))
