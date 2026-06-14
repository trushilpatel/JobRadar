import { SignIn, useAuth } from '@clerk/react'
import { Navigate } from 'react-router-dom'
import { LogoLockup } from '@/components/brand/Logo'

export function SignInPage() {
  const { isSignedIn, isLoaded } = useAuth()

  if (isLoaded && isSignedIn) {
    return <Navigate to="/" replace />
  }

  return (
    <div className="flex h-screen w-full items-center justify-center bg-page text-primary">
      <div className="flex flex-col items-center gap-6">
        <LogoLockup size="lg" />
        <SignIn redirectUrl="/" />
      </div>
    </div>
  )
}
