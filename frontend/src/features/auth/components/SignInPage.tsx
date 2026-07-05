import { useState } from 'react'
import { Navigate } from 'react-router-dom'
import { useAuth, useSignIn, useSignUp } from '@clerk/react'

/**
 * Extracts a user-safe message from a Clerk Future-API error. `longMessage`
 * is the field Clerk documents as safe to show end users; `message` is for
 * developers only and is used solely as a last-resort fallback. Duck-typed
 * rather than importing `ClerkError` from `@clerk/shared` — that package is
 * only a transitive dependency of `@clerk/react`, not a direct one.
 */
function clerkErrorMessage(error: { longMessage?: string; message?: string }, fallback: string) {
  return error.longMessage ?? error.message ?? fallback
}

export function SignInPage() {
  const { isSignedIn, isLoaded } = useAuth()
  const { signIn } = useSignIn()
  const { signUp } = useSignUp()
  const [mode, setMode] = useState<'signin' | 'create'>('signin')
  const [view, setView] = useState<'methods' | 'otp'>('methods')
  const [email, setEmail] = useState('')
  const [otp, setOtp] = useState('')
  const [authError, setAuthError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  if (isLoaded && isSignedIn) {
    return <Navigate to="/" replace />
  }

  function handleGoogleClick() {
    void signIn.sso({
      strategy: 'oauth_google',
      redirectUrl: '/sso-callback',
      redirectCallbackUrl: '/',
    })
  }

  async function handleEmailSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!email.trim()) return
    setAuthError(null)
    setLoading(true)
    try {
      if (mode === 'signin') {
        const { error } = await signIn.emailCode.sendCode({ emailAddress: email })
        if (error) {
          setAuthError(clerkErrorMessage(error, 'Something went wrong'))
          return
        }
      } else {
        const created = await signUp.create({ emailAddress: email })
        if (created.error) {
          setAuthError(clerkErrorMessage(created.error, 'Something went wrong'))
          return
        }
        const sent = await signUp.verifications.sendEmailCode()
        if (sent.error) {
          setAuthError(clerkErrorMessage(sent.error, 'Something went wrong'))
          return
        }
      }
      setView('otp')
    } finally {
      setLoading(false)
    }
  }

  async function handleOtpSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (otp.length < 6) return
    setAuthError(null)
    setLoading(true)
    try {
      if (mode === 'signin') {
        const { error } = await signIn.emailCode.verifyCode({ code: otp })
        if (error) {
          setAuthError(clerkErrorMessage(error, 'Incorrect code'))
          return
        }
        if (signIn.status === 'complete') {
          await signIn.finalize()
        }
      } else {
        const { error } = await signUp.verifications.verifyEmailCode({ code: otp })
        if (error) {
          setAuthError(clerkErrorMessage(error, 'Incorrect code'))
          return
        }
        if (signUp.status === 'complete') {
          await signUp.finalize()
        }
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div
      className="flex h-screen w-full"
      style={{
        background: [
          'radial-gradient(1200px 600px at 8% -10%, color-mix(in srgb, var(--violet) 12%, transparent), transparent 58%)',
          'radial-gradient(900px 500px at 100% 5%, color-mix(in srgb, var(--cyan) 8%, transparent), transparent 54%)',
          'var(--bg-page)',
        ].join(', '),
      }}
    >
      {/* Left: value panel (hidden on mobile) */}
      <aside className="hidden lg:flex flex-col justify-center px-16 flex-1 max-w-[560px]">
        <div
          className="mb-6 w-10 h-10 rounded-xl flex items-center justify-center text-[18px] font-bold"
          style={{
            background: 'linear-gradient(135deg, var(--violet), var(--cyan))',
            boxShadow: 'var(--glow-violet)',
            color: '#0b0b14',
          }}
        >
          ◎
        </div>
        <h1
          className="text-4xl font-bold tracking-tight leading-tight mb-3"
          style={{
            background:
              'linear-gradient(100deg, var(--text-primary) 50%, var(--violet-bright) 90%, var(--cyan))',
            WebkitBackgroundClip: 'text',
            WebkitTextFillColor: 'transparent',
            backgroundClip: 'text',
          }}
        >
          Every job that matters.
          <br />
          In five minutes a day.
        </h1>
        <p className="text-base text-muted mb-8">
          JobRadar watches 100+ company careers pages and surfaces the roles you actually want —
          automatically filtered, scored, and ready to act on.
        </p>
        <ul className="space-y-3">
          {(
            [
              ['◎', 'Monitors 100+ careers pages daily'],
              ['▤', 'LLM-filtered to match your preferences'],
              ['↗', 'One-click apply queue — no portal hunting'],
            ] as [string, string][]
          ).map(([icon, label]) => (
            <li key={label} className="flex items-center gap-3 text-sm text-secondary">
              <span className="font-mono" style={{ color: 'var(--violet-bright)' }}>
                {icon}
              </span>
              {label}
            </li>
          ))}
        </ul>
      </aside>

      {/* Right: auth card */}
      <div className="flex items-center justify-center flex-1 px-6" data-testid="sign-in-panel">
        <div
          className="w-full max-w-sm rounded-2xl p-8 border border-[var(--border-strong)]"
          style={{
            background: 'var(--bg-panel)',
            boxShadow: '0 24px 64px rgba(0,0,0,0.3)',
          }}
        >
          {view === 'otp' ? (
            <form onSubmit={handleOtpSubmit}>
              <button
                type="button"
                onClick={() => {
                  setView('methods')
                  setOtp('')
                  setAuthError(null)
                }}
                className="text-xs text-muted hover:text-primary transition-colors mb-4"
              >
                ← Use a different email
              </button>
              <p className="text-base font-semibold text-primary mb-1">Check your email</p>
              <p className="text-xs text-muted mb-4">
                Enter the 6-digit code sent to <strong>{email}</strong>.
              </p>
              <input
                type="text"
                inputMode="numeric"
                maxLength={6}
                placeholder="000000"
                value={otp}
                onChange={(e) => setOtp(e.target.value.replace(/\D/g, '').slice(0, 6))}
                className="w-full text-center text-2xl tracking-widest rounded-xl px-4 py-3 border border-[var(--border-strong)] focus:outline-none focus:border-[var(--violet)] focus:ring-2 focus:ring-[color-mix(in_srgb,var(--violet)_14%,transparent)] mb-3"
                style={{ background: 'var(--bg-subtle)', color: 'var(--text-primary)' }}
                autoFocus
              />
              {authError && <p className="text-xs text-red-400 mb-2">{authError}</p>}
              <button
                type="submit"
                disabled={otp.length < 6 || loading}
                className="w-full py-3 rounded-xl text-sm font-semibold disabled:opacity-50 transition-all"
                style={{
                  background:
                    'linear-gradient(135deg, var(--violet), color-mix(in srgb, var(--violet) 60%, var(--cyan)))',
                  color: '#0b0b14',
                  boxShadow: 'var(--glow-violet)',
                }}
              >
                {loading ? 'Verifying…' : 'Verify →'}
              </button>
            </form>
          ) : (
            <>
              {/* Mode toggle */}
              <div
                className="flex gap-1 p-1 rounded-full mb-6"
                role="group"
                aria-label="Sign in or create an account"
                style={{ background: 'var(--bg-subtle)' }}
              >
                {(['signin', 'create'] as const).map((m) => (
                  <button
                    key={m}
                    type="button"
                    onClick={() => setMode(m)}
                    aria-pressed={mode === m}
                    className="flex-1 py-2 rounded-full text-sm font-medium transition-all"
                    style={
                      mode === m
                        ? {
                            background: 'var(--bg-panel)',
                            color: 'var(--text-primary)',
                            boxShadow: '0 1px 4px rgba(0,0,0,0.2)',
                          }
                        : { color: 'var(--text-muted)' }
                    }
                  >
                    {m === 'signin' ? 'Sign in' : 'Create account'}
                  </button>
                ))}
              </div>

              {/* Google OAuth button */}
              <button
                type="button"
                className="w-full flex items-center justify-center gap-3 py-2.5 rounded-xl border border-[var(--border-strong)] text-sm font-medium text-primary hover:bg-[var(--bg-hover)] transition-colors mb-5"
                onClick={handleGoogleClick}
              >
                <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true">
                  <path
                    d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"
                    fill="#4285F4"
                  />
                  <path
                    d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"
                    fill="#34A853"
                  />
                  <path
                    d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l3.66-2.84z"
                    fill="#FBBC05"
                  />
                  <path
                    d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"
                    fill="#EA4335"
                  />
                </svg>
                Continue with Google
              </button>

              <div className="flex items-center gap-3 mb-5">
                <div className="flex-1 h-px bg-[var(--border)]" />
                <span className="text-xs text-muted">or with email</span>
                <div className="flex-1 h-px bg-[var(--border)]" />
              </div>

              <form onSubmit={handleEmailSubmit}>
                <input
                  type="email"
                  placeholder="you@example.com"
                  autoComplete="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  className="w-full rounded-xl px-4 py-3 text-sm text-primary placeholder:text-placeholder border border-[var(--border-strong)] focus:outline-none focus:border-[var(--violet)] focus:ring-2 focus:ring-[color-mix(in_srgb,var(--violet)_14%,transparent)] mb-3"
                  style={{ background: 'var(--bg-subtle)' }}
                />
                {authError && <p className="text-xs text-red-400 mb-2">{authError}</p>}
                <button
                  type="submit"
                  disabled={loading}
                  className="w-full py-3 rounded-xl text-sm font-semibold disabled:opacity-50 transition-all"
                  style={{
                    background:
                      'linear-gradient(135deg, var(--violet), color-mix(in srgb, var(--violet) 60%, var(--cyan)))',
                    color: '#0b0b14',
                    boxShadow: 'var(--glow-violet)',
                  }}
                >
                  {mode === 'create' ? 'Create account →' : 'Continue →'}
                </button>
              </form>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
