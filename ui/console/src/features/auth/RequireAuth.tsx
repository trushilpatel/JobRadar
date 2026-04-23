import { Navigate, Outlet } from 'react-router-dom'
import { useAuthStore } from './store'

export function RequireAuth() {
  const session = useAuthStore((s) => s.session)
  const loading = useAuthStore((s) => s.loading)

  if (loading) {
    return null
  }
  if (!session) {
    return <Navigate to="/login" replace />
  }
  return <Outlet />
}
