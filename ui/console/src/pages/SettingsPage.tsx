import { ErrorBoundary } from '@/components/shared/ErrorBoundary'
import { AdapterConfigsTable } from '@/features/settings'

export function SettingsPage() {
  return (
    <div className="flex flex-col h-full bg-[#0d0d14] overflow-y-auto">
      <div className="border-b border-white/[0.07] px-6 py-4 shrink-0">
        <h1 className="text-lg font-semibold text-white">Settings</h1>
      </div>

      <div className="flex-1 px-6 py-6 space-y-8">
        <section>
          <div className="mb-4">
            <h2 className="text-sm font-semibold text-white">Adapter Configuration</h2>
            <p className="text-sm text-white/50 mt-1">
              Per-adapter scan intervals and inter-company delays. Changes apply to all companies
              using that adapter unless a company-level override is set.
            </p>
          </div>
          <ErrorBoundary>
            <AdapterConfigsTable />
          </ErrorBoundary>
        </section>
      </div>
    </div>
  )
}
