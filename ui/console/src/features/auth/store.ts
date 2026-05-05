import { create } from 'zustand'
import type { Session } from '@supabase/supabase-js'
import { supabase } from '@/lib/supabase'
import { fetchMe } from '@/api/me'
import type { Identity } from '@/types'

interface AuthState {
  session: Session | null
  identity: Identity | null
  loading: boolean
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
  initialized: false,
  isAdmin: false,

  bootstrap: async () => {
    if (get().initialized) {
      return () => {}
    }
    set({ initialized: true })
    const { data } = await supabase.auth.getSession()
    const session = data.session

    // Unblock rendering immediately — identity loads in background
    set({ session, loading: false })

    if (session) {
      loadIdentity().then((identity) => {
        if (get().session) set({ identity, isAdmin: isAdminIdentity(identity) })
      })
    }

    const { data: sub } = supabase.auth.onAuthStateChange(async (event, newSession) => {
      if (event === 'TOKEN_REFRESHED') {
        set({ session: newSession })
        return
      }
      let newIdentity: Identity | null = null
      if (newSession) {
        newIdentity = await loadIdentity()
      }
      set({ session: newSession, identity: newIdentity, loading: false, isAdmin: isAdminIdentity(newIdentity) })
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
    set({ session: null, identity: null, isAdmin: false })
  },

  refreshIdentity: async () => {
    const identity = await loadIdentity()
    set({ identity, isAdmin: isAdminIdentity(identity) })
  },
}))
