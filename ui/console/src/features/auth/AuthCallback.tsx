import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { supabase } from '@/lib/supabase'

type State = 'pending' | 'success' | 'error'

export function AuthCallback() {
  const navigate = useNavigate()
  const [state, setState] = useState<State>('pending')
  const [message, setMessage] = useState<string>('')

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const href = window.location.href
        const search = window.location.search
        const hasCode = /[?&]code=/.test(href)
        const hasHashToken = /access_token=|error=/.test(window.location.hash)

        if (hasCode) {
          const { error } = await supabase.auth.exchangeCodeForSession(search)
          if (error) throw new Error(error.message || 'Authentication failed')
        } else if (hasHashToken) {
          const { data, error } = await supabase.auth.getSession()
          if (error) throw new Error(error.message || 'Authentication failed')
          if (!data.session) throw new Error('No session returned')
        } else {
          const { data } = await supabase.auth.getSession()
          if (!data.session) throw new Error('Missing authorization code')
        }

        if (cancelled) return
        window.history.replaceState({}, '', '/')
        setState('success')
        navigate('/', { replace: true })
      } catch (err) {
        if (cancelled) return
        setState('error')
        setMessage(err instanceof Error ? err.message : 'Authentication failed')
      }
    })()
    return () => {
      cancelled = true
    }
  }, [navigate])

  return (
    <div className="flex h-screen w-full items-center justify-center bg-neutral-950 text-neutral-100">
      <div className="w-full max-w-sm rounded-xl border border-neutral-800 bg-neutral-900 p-8 text-center shadow-xl">
        {state === 'pending' && <p className="text-sm text-neutral-400">Signing you in…</p>}
        {state === 'success' && <p className="text-sm text-neutral-400">Redirecting…</p>}
        {state === 'error' && (
          <>
            <h1 className="text-lg font-semibold text-red-400">Sign-in failed</h1>
            <p role="alert" className="mt-3 text-sm text-neutral-400">
              {message || 'Something went wrong.'}
            </p>
            <Link
              to="/login"
              className="mt-6 inline-block text-sm text-neutral-200 underline hover:text-white"
            >
              Back to sign in
            </Link>
          </>
        )}
      </div>
    </div>
  )
}
