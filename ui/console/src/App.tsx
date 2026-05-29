import { useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { NuqsAdapter } from 'nuqs/adapters/react-router/v7'
import { Toaster } from 'sonner'
import { queryClient } from '@/lib/core/queryClient'
import { Sidebar } from '@/app/Sidebar'
import { CompaniesPage } from '@/pages/CompaniesPage'
import { DashboardPage } from '@/pages/DashboardPage'
import { JobsPage } from '@/pages/JobsPage'
import { MobileJobDetailRoute } from '@/features/jobs'
import { InsightsPage } from '@/pages/InsightsPage'
import { LoginPage, AuthCallback, RequireAuth, useAuthStore } from '@/features/auth'
import { SettingsPage } from '@/pages/SettingsPage'
import { AccountPage } from '@/pages/AccountPage'
import { AdminUsersPage } from '@/pages/admin/AdminUsersPage'
import { AdminAuditPage } from '@/pages/admin/AdminAuditPage'
import { ResumeLibraryPage } from '@/pages/admin/ResumeLibraryPage'
import { ResumeDetailPage } from '@/pages/admin/ResumeDetailPage'
import { ResumeEditorPage } from '@/pages/admin/ResumeEditorPage'
import { ApplyQueuePage } from '@/pages/admin/ApplyQueuePage'
import { RequireAdmin } from '@/features/auth/RequireAdmin'
import { routes } from '@/lib/routes'

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
    <div className="flex h-dvh w-full overflow-hidden">
      <Sidebar />
      <main className="flex-1 flex flex-col min-h-0 overflow-hidden">
        <Routes>
          <Route path="/" element={<Navigate to={routes.dashboard()} replace />} />
          <Route path="/dashboard" element={<DashboardPage />} />
          <Route path="/jobs" element={<JobsPage />} />
          <Route path="/jobs/:id" element={<MobileJobDetailRoute />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/account" element={<AccountPage />} />
          {/* Admin routes — server also enforces 403 for non-admins */}
          <Route element={<RequireAdmin />}>
            <Route path="/admin/companies" element={<CompaniesPage />} />
            <Route path="/admin/users" element={<AdminUsersPage />} />
            <Route path="/admin/audit" element={<AdminAuditPage />} />
            <Route path="/admin/resumes" element={<ResumeLibraryPage />} />
            <Route path="/admin/resumes/:id" element={<ResumeDetailPage />} />
            <Route path="/admin/resumes/:id/edit" element={<ResumeEditorPage />} />
            <Route path="/admin/apply-queue" element={<ApplyQueuePage />} />
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
        <NuqsAdapter>
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
        </NuqsAdapter>
      </BrowserRouter>
    </QueryClientProvider>
  )
}
