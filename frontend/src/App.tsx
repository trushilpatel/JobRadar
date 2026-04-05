import { useState } from 'react'
import { Sidebar, type Page } from '@/components/Sidebar'
import { CompaniesPage } from '@/pages/CompaniesPage'

function Placeholder({ title }: { title: string }) {
  return (
    <div className="flex-1 flex items-center justify-center text-slate-500">
      {title} — coming soon
    </div>
  )
}

export default function App() {
  const [page, setPage] = useState<Page>('companies')

  return (
    <div className="flex w-full min-h-screen">
      <Sidebar current={page} onChange={setPage} />
      <main className="flex-1 flex flex-col min-h-screen overflow-hidden">
        {page === 'dashboard' && <Placeholder title="Dashboard" />}
        {page === 'companies' && <CompaniesPage />}
        {page === 'jobs' && <Placeholder title="Jobs" />}
        {page === 'applications' && <Placeholder title="Applications" />}
      </main>
    </div>
  )
}
