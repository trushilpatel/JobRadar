import { useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from 'sonner'
import { queryClient } from '@/lib/queryClient'
import { Sidebar } from '@/components/Sidebar'
import { CompaniesPage } from '@/pages/CompaniesPage'
import { DashboardPage } from '@/pages/DashboardPage'
import { JobsPage } from '@/pages/JobsPage'
import { InsightsPage } from '@/pages/InsightsPage'
import { LoginPage, AuthCallback, RequireAuth, useAuthStore } from '@/features/auth'
import { SettingsPage } from '@/pages/SettingsPage'
import { AccountPage } from '@/pages/AccountPage'
import { AdminUsersPage } from '@/pages/admin/AdminUsersPage'
import { AdminAuditPage } from '@/pages/admin/AdminAuditPage'
import { AdminPipelinePage } from '@/pages/admin/AdminPipelinePage'
import { RequireAdmin } from '@/features/auth/RequireAdmin'

function AuthBootstrap({ children }: { children: React.ReactNode }) {
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

function ProtectedLayout() {
  return (
    <div className="flex h-screen w-full overflow-hidden">
      <Sidebar />
      <main className="flex-1 flex flex-col min-h-0 overflow-hidden">
        <Routes>
          <Route path="/" element={<Navigate to="/dashboard" replace />} />
          <Route path="/dashboard" element={<DashboardPage />} />
          <Route path="/companies" element={<CompaniesPage />} />
          <Route path="/jobs" element={<JobsPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/account" element={<AccountPage />} />
          {/* Admin routes — server also enforces 403 for non-admins */}
          <Route element={<RequireAdmin />}>
            <Route path="/admin/pipeline" element={<AdminPipelinePage />} />
            <Route path="/admin/users" element={<AdminUsersPage />} />
            <Route path="/admin/audit" element={<AdminAuditPage />} />
            <Route path="/insights" element={<InsightsPage />} />
          </Route>
        </Routes>
      </main>
    </div>
  )
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <AuthBootstrap>
          <Toaster position="bottom-center" theme="dark" richColors />
          <Routes>
            <Route path="/login" element={<LoginPage />} />
            <Route path="/auth/callback" element={<AuthCallback />} />
            <Route element={<RequireAuth />}>
              <Route path="/*" element={<ProtectedLayout />} />
            </Route>
          </Routes>
        </AuthBootstrap>
      </BrowserRouter>
    </QueryClientProvider>
  )
}
