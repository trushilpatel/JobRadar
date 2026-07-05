import { Navigate } from 'react-router-dom'
import { useAuth, SignIn } from '@clerk/react'
import { Globe, Sparkles, Zap } from 'lucide-react'
import { LogoMark } from '@/components/brand/Logo'

export function SignInPage() {
  const { isSignedIn, isLoaded } = useAuth()

  if (isLoaded && isSignedIn) {
    return <Navigate to="/" replace />
  }

  return (
    <div
      className="flex flex-col h-screen w-full"
      style={{
        background: [
          'radial-gradient(1200px 600px at 8% -10%, color-mix(in srgb, var(--violet) 12%, transparent), transparent 58%)',
          'radial-gradient(900px 500px at 100% 5%, color-mix(in srgb, var(--cyan) 8%, transparent), transparent 54%)',
          'var(--bg-page)',
        ].join(', '),
      }}
    >
      {/* Top bar with logo */}
      <header className="flex-none flex items-center px-6 lg:px-10 h-14 border-b border-[var(--border)]">
        <div className="flex items-center gap-2.5">
          <div
            className="w-7 h-7 rounded-lg flex items-center justify-center shrink-0"
            style={{
              background: 'linear-gradient(135deg, var(--violet), var(--cyan))',
              boxShadow: 'var(--glow-violet)',
            }}
          >
            <LogoMark className="w-4 h-4 text-white" />
          </div>
          <span className="text-sm font-bold tracking-tight text-primary">JobRadar</span>
        </div>
      </header>

      {/* Main two-column area */}
      <div className="flex flex-1 min-h-0">
        {/* Left: value panel — hidden on mobile */}
        <aside className="hidden lg:flex flex-col justify-center px-16 flex-1 max-w-[660px]">
          <h1
            className="text-6xl font-bold tracking-tight leading-tight mb-5"
            style={{
              background:
                'linear-gradient(100deg, var(--text-primary) 50%, var(--violet-bright) 90%, var(--cyan))',
              WebkitBackgroundClip: 'text',
              WebkitTextFillColor: 'transparent',
              backgroundClip: 'text',
            }}
          >
            Every job that matters to you.
            <br />
            Surfaced in five minutes a day.
          </h1>
          <p className="text-xl text-muted mb-10 leading-relaxed">
            JobRadar watches 100+ company careers pages and surfaces the roles you actually want —
            automatically filtered, scored, and ready to act on.
          </p>
          <ul className="space-y-4">
            {(
              [
                [<Globe key="globe" size={16} />, 'Monitors 100+ careers pages daily'],
                [<Sparkles key="sparkles" size={16} />, 'LLM-filtered to match your preferences'],
                [<Zap key="zap" size={16} />, 'One-click apply queue — no portal hunting'],
              ] as [React.ReactNode, string][]
            ).map(([icon, label]) => (
              <li key={label} className="flex items-center gap-3 text-lg text-secondary">
                <span className="shrink-0" style={{ color: 'var(--violet-bright)' }}>
                  {icon}
                </span>
                {label}
              </li>
            ))}
          </ul>
        </aside>

        {/* Right: Clerk managed sign-in */}
        <div className="flex items-center justify-center flex-1 px-6" data-testid="sign-in-panel">
          <SignIn routing="hash" fallbackRedirectUrl="/" />
        </div>
      </div>
    </div>
  )
}
