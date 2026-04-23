import { create } from 'zustand'
import type { Session } from '@supabase/supabase-js'
import { supabase } from '@/lib/supabase'

interface AuthState {
  session: Session | null
  loading: boolean
  initialized: boolean
  bootstrap: () => Promise<() => void>
  signInWithEmail: (email: string) => Promise<void>
  signOut: () => Promise<void>
}

export const useAuthStore = create<AuthState>((set, get) => ({
  session: null,
  loading: true,
  initialized: false,

  bootstrap: async () => {
    if (get().initialized) {
      return () => {}
    }
    set({ initialized: true })
    const { data } = await supabase.auth.getSession()
    set({ session: data.session, loading: false })
    const { data: sub } = supabase.auth.onAuthStateChange((_event, session) => {
      set({ session, loading: false })
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
    set({ session: null })
  },
}))
