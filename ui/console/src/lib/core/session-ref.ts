import type { Session } from '@supabase/supabase-js'

let current: Session | null = null

export const sessionRef = {
  get: (): Session | null => current,
  set: (s: Session | null): void => {
    current = s
  },
}
