import { Navigate, Outlet } from 'react-router-dom'
import { useAuthStore } from './store'

/**
 * Route guard that redirects non-admin users to /dashboard.
 * The server enforces 403 for /admin/* regardless — this is a UX guard only.
 */
export function RequireAdmin() {
  const isAdmin = useAuthStore((s) => s.isAdmin)
  const loading = useAuthStore((s) => s.loading)
  const identityLoading = useAuthStore((s) => s.identityLoading)

  // Wait for identity before deciding — otherwise a refresh on /admin/* or /insights
  // redirects to /dashboard during the brief window where session is set but
  // isAdmin hasn't been resolved yet.
  if (loading || identityLoading) return null
  if (!isAdmin) return <Navigate to="/dashboard" replace />
  return <Outlet />
}
