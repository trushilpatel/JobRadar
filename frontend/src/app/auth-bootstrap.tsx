import { useEffect } from 'react'
import { useAuthStore } from '@/features/auth'

export function AuthBootstrap({ children }: { children: React.ReactNode }) {
  const bootstrap = useAuthStore((s) => s.bootstrap)
  useEffect(() => {
    let unsubscribe: (() => void) | undefined
    bootstrap().then((fn) => {
      unsubscribe = fn
    })
    return () => {
      unsubscribe?.()
    }
  }, [bootstrap])
  return <>{children}</>
}
