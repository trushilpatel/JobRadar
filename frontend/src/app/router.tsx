import { Routes, Route, Navigate } from 'react-router-dom'
import { Sidebar } from './Sidebar'
import { CompaniesPage } from '@/pages/CompaniesPage'
import { DashboardPage } from '@/pages/DashboardPage'
import { JobsPage } from '@/pages/JobsPage'
import { InsightsPage } from '@/pages/InsightsPage'
import { AccountPage } from '@/pages/AccountPage'
import { AdminUsersPage } from '@/pages/admin/AdminUsersPage'
import { AdminAuditPage } from '@/pages/admin/AdminAuditPage'
import { AdminPipelinePage } from '@/pages/admin/AdminPipelinePage'
import { ResumeLibraryPage } from '@/pages/admin/ResumeLibraryPage'
import { ResumeDetailPage } from '@/pages/admin/ResumeDetailPage'
import { ResumeEditorPage } from '@/pages/admin/ResumeEditorPage'
import { ApplyQueuePage } from '@/pages/admin/ApplyQueuePage'
import { SignInPage, RequireAuth, RequireAdmin } from '@/features/auth'
import { MobileJobDetailRoute } from '@/features/jobs'
import { routes } from '@/config/routes'

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
          <Route path="/account" element={<AccountPage />} />
          <Route path="/insights" element={<InsightsPage />} />
          {/* Admin routes — server also enforces 403 for non-admins */}
          <Route element={<RequireAdmin />}>
            <Route path="/admin/companies" element={<CompaniesPage />} />
            <Route path="/admin/users" element={<AdminUsersPage />} />
            <Route path="/admin/audit" element={<AdminAuditPage />} />
            <Route path="/admin/resumes" element={<ResumeLibraryPage />} />
            <Route path="/admin/resumes/:id" element={<ResumeDetailPage />} />
            <Route path="/admin/resumes/:id/edit" element={<ResumeEditorPage />} />
            <Route path="/admin/apply-queue" element={<ApplyQueuePage />} />
            <Route path="/admin/pipeline" element={<AdminPipelinePage />} />
          </Route>
        </Routes>
      </main>
    </div>
  )
}

export function AppRouter() {
  return (
    <Routes>
      <Route path="/login" element={<SignInPage />} />
      <Route element={<RequireAuth />}>
        <Route path="/*" element={<ProtectedLayout />} />
      </Route>
    </Routes>
  )
}
