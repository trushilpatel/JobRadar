import { useUser } from '@clerk/react'
import { Navigate, Outlet } from 'react-router-dom'

/**
 * Route guard: allows only users with publicMetadata.role === "admin".
 * The server enforces 403 on /admin/* regardless — this is a UX guard only.
 * Grant admin in Clerk dashboard: User → publicMetadata → { "role": "admin" }.
 */
export function RequireAdmin() {
  const { user, isLoaded } = useUser()

  if (!isLoaded) return null
  if (user?.publicMetadata?.role !== 'admin') return <Navigate to="/dashboard" replace />
  return <Outlet />
}
