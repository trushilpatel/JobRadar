import { useAuth } from '@clerk/react'
import { Navigate, Outlet } from 'react-router-dom'

export function RequireAuth() {
  const { isSignedIn, isLoaded } = useAuth()

  if (!isLoaded) return null
  if (!isSignedIn) return <Navigate to="/login" replace />
  return <Outlet />
}
