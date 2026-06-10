import { useState } from 'react'
import { Navigate } from 'react-router-dom'
import { useAuthStore } from '../store/auth.store'
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
    <div className="flex h-screen w-full items-center justify-center bg-page text-primary">
      <div className="w-full max-w-sm rounded-xl border border-[var(--border-strong)] bg-panel p-8 shadow-xl">
        <div className="mb-6 text-center">
          <h1 className="flex justify-center text-primary">
            <LogoLockup size="lg" />
          </h1>
          <p className="mt-2 text-sm text-muted">Sign in or create an account</p>
        </div>

        {sent ? (
          <div className="text-center">
            <p className="text-sm text-secondary">Sign-in link sent to</p>
            <p className="mt-1 text-sm font-medium text-primary">{email}</p>
            <p className="mt-3 text-xs text-muted">
              Check your inbox and click the link to continue.
            </p>
            <button
              type="button"
              onClick={() => setSent(false)}
              className="mt-5 text-xs text-muted underline hover:text-primary"
            >
              Resend
            </button>
          </div>
        ) : (
          <form onSubmit={handleSignIn} className="space-y-3">
            <div>
              <label htmlFor="email" className="sr-only">
                Email address
              </label>
              <input
                id="email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="you@example.com"
                required
                aria-describedby={error ? 'email-error' : undefined}
                className={cn(
                  'w-full rounded-md border border-[var(--border-strong)] bg-subtle px-4 py-2.5 text-sm text-primary',
                  'placeholder:text-placeholder focus:outline-none focus:ring-2 focus:ring-violet-500'
                )}
              />
            </div>
            <button
              type="submit"
              disabled={submitting || !email}
              className={cn(
                'w-full rounded-md border border-[var(--border-strong)] bg-subtle px-4 py-2.5 text-sm font-medium text-primary',
                'transition hover:bg-hover focus:outline-none focus:ring-2 focus:ring-violet-500',
                'disabled:cursor-not-allowed disabled:opacity-60'
              )}
            >
              {submitting ? 'Sending…' : 'Send sign-in link'}
            </button>
            {error && (
              <p id="email-error" role="alert" className="text-center text-sm text-red-400">
                {error}
              </p>
            )}
          </form>
        )}
      </div>
    </div>
  )
}
