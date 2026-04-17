import { HashRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from 'sonner'
import { queryClient } from '@/lib/queryClient'
import { Sidebar } from '@/components/Sidebar'
import { CompaniesPage } from '@/pages/CompaniesPage'
import { DashboardPage } from '@/pages/DashboardPage'
import { JobsPage } from '@/pages/JobsPage'
import { InsightsPage } from '@/pages/InsightsPage'

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <HashRouter>
        <Toaster position="bottom-center" theme="dark" richColors />
        <div className="flex h-screen w-full overflow-hidden">
          <Sidebar />
          <main className="flex-1 flex flex-col min-h-0 overflow-hidden">
            <Routes>
              <Route path="/" element={<Navigate to="/dashboard" replace />} />
              <Route path="/dashboard" element={<DashboardPage />} />
              <Route path="/companies" element={<CompaniesPage />} />
              <Route path="/jobs" element={<JobsPage />} />
              <Route path="/insights" element={<InsightsPage />} />
            </Routes>
          </main>
        </div>
      </HashRouter>
    </QueryClientProvider>
  )
}
