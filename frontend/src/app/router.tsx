import { useState } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import { Menu } from 'lucide-react'
import { Sidebar, MobileSidebarNav } from './Sidebar'
import { Sheet, SheetContent, SheetTitle, SheetDescription } from '@/components/ui/sheet'
import { LogoLockup } from '@/components/brand/Logo'
import { CompaniesPage } from '@/pages/CompaniesPage'
import { JobsPage } from '@/pages/JobsPage'
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
  const [mobileNavOpen, setMobileNavOpen] = useState(false)

  return (
    <div className="flex h-dvh w-full overflow-hidden">
      <Sidebar />

      {/* Mobile drawer — Radix Dialog underneath gives overlay + focus trap + Esc */}
      <Sheet open={mobileNavOpen} onOpenChange={setMobileNavOpen}>
        <SheetContent
          side="left"
          className="w-64 max-w-[80%] bg-sidebar backdrop-blur-xl border-[var(--sidebar-border)] p-0"
        >
          <SheetTitle className="sr-only">Navigation</SheetTitle>
          <SheetDescription className="sr-only">Primary navigation menu</SheetDescription>
          <MobileSidebarNav onNavigate={() => setMobileNavOpen(false)} />
        </SheetContent>
      </Sheet>

      <main className="flex-1 flex flex-col min-h-0 overflow-hidden">
        {/* Mobile top bar — desktop rail is hidden below md */}
        <div className="md:hidden flex items-center gap-3 h-[57px] px-3 border-b border-[var(--sidebar-border)] bg-sidebar backdrop-blur-xl shrink-0">
          <button
            type="button"
            onClick={() => setMobileNavOpen(true)}
            className="p-1.5 text-muted hover:text-[var(--sidebar-fg)] hover:bg-[var(--sidebar-hover)] rounded-md transition-colors"
            aria-label="Open menu"
          >
            <Menu size={18} />
          </button>
          <LogoLockup className="text-sidebar-fg" />
        </div>

        <Routes>
          <Route path="/" element={<Navigate to={routes.jobs()} replace />} />
          <Route path="/jobs" element={<JobsPage />} />
          <Route path="/jobs/:id" element={<MobileJobDetailRoute />} />
          <Route path="/account" element={<AccountPage />} />
          {/* Résumés + Apply queue are frontend-only (no backend endpoints) —
              user-facing for every authenticated user, not admin-gated. Paths
              keep the /admin prefix to avoid a route rename. */}
          <Route path="/admin/resumes" element={<ResumeLibraryPage />} />
          <Route path="/admin/resumes/:id" element={<ResumeDetailPage />} />
          <Route path="/admin/resumes/:id/edit" element={<ResumeEditorPage />} />
          <Route path="/admin/apply-queue" element={<ApplyQueuePage />} />
          {/* Admin routes — server also enforces 403 for non-admins */}
          <Route element={<RequireAdmin />}>
            <Route path="/admin/companies" element={<CompaniesPage />} />
            <Route path="/admin/users" element={<AdminUsersPage />} />
            <Route path="/admin/audit" element={<AdminAuditPage />} />
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
