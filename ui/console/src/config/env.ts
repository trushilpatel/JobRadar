// Typed access to VITE_* environment variables.
// Components must import from here, never from import.meta.env directly.

const env = import.meta.env as Record<string, string | undefined>

function required(key: string): string {
  const value = env[key]
  if (!value) throw new Error(`Missing required env var: ${key}`)
  return value
}

export const ENV = {
  SUPABASE_URL: required('VITE_SUPABASE_URL'),
  SUPABASE_ANON_KEY: required('VITE_SUPABASE_ANON_KEY'),
} as const
