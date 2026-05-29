import { useState } from 'react'
import { Navigate } from 'react-router-dom'
import { useAuthStore } from './store'
import { cn } from '@/lib/utils/utils'
import { LogoLockup } from '@/components/brand/Logo'

export function LoginPage() {
  const session = useAuthStore((s) => s.session)
  const loading = useAuthStore((s) => s.loading)
  const signInWithEmail = useAuthStore((s) => s.signInWithEmail)
  const [email, setEmail] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [sent, setSent] = useState(false)
  const [error, setError] = useState<string | null>(null)

  if (!loading && session) {
    return <Navigate to="/" replace />
  }

  const handleSignIn = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      await signInWithEmail(email)
      setSent(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Sign-in failed')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex h-screen w-full items-center justify-center bg-neutral-950 text-neutral-100">
      <div className="w-full max-w-sm rounded-xl border border-neutral-800 bg-neutral-900 p-8 shadow-xl">
        <div className="mb-6 text-center">
          <h1 className="flex justify-center text-white">
            <LogoLockup size="lg" />
          </h1>
          <p className="mt-2 text-sm text-neutral-400">Sign in or create an account</p>
        </div>

        {sent ? (
          <div className="text-center">
            <p className="text-sm text-neutral-300">Sign-in link sent to</p>
            <p className="mt-1 text-sm font-medium text-white">{email}</p>
            <p className="mt-3 text-xs text-neutral-500">
              Check your inbox and click the link to continue.
            </p>
            <button
              type="button"
              onClick={() => setSent(false)}
              className="mt-5 text-xs text-neutral-500 underline hover:text-neutral-300"
            >
              Resend
            </button>
          </div>
        ) : (
          <form onSubmit={handleSignIn} className="space-y-3">
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@example.com"
              required
              className={cn(
                'w-full rounded-md border border-neutral-700 bg-neutral-800 px-4 py-2.5 text-sm text-neutral-100',
                'placeholder:text-neutral-500 focus:outline-none focus:ring-2 focus:ring-neutral-600'
              )}
            />
            <button
              type="submit"
              disabled={submitting || !email}
              className={cn(
                'w-full rounded-md border border-neutral-700 bg-neutral-800 px-4 py-2.5 text-sm font-medium',
                'transition hover:bg-neutral-700 focus:outline-none focus:ring-2 focus:ring-neutral-600',
                'disabled:cursor-not-allowed disabled:opacity-60'
              )}
            >
              {submitting ? 'Sending…' : 'Send sign-in link'}
            </button>
            {error && (
              <p role="alert" className="text-center text-sm text-red-400">
                {error}
              </p>
            )}
          </form>
        )}
      </div>
    </div>
  )
}
