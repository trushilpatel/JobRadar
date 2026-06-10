import { ErrorBoundary } from '@/components/shared/ErrorBoundary'
import { AtsTable } from '@/features/settings'
import { useAts, useAtsActions } from '@/features/ats'

export function SettingsPage() {
  const { data: atsList, isLoading, isError, refetch } = useAts()
  const mutation = useAtsActions()

  return (
    <div className="flex flex-col h-full bg-panel overflow-y-auto">
      <div className="border-b border-[var(--border)] px-6 py-4 shrink-0">
        <h1 className="text-lg font-semibold text-primary">Settings</h1>
      </div>

      <div className="flex-1 px-6 py-6 space-y-8">
        <section>
          <div className="mb-4">
            <h2 className="text-sm font-semibold text-primary">Adapter Configuration</h2>
            <p className="text-sm text-muted mt-1">
              Per-adapter scan intervals and inter-company delays. Changes apply to all companies
              using that adapter unless a company-level override is set.
            </p>
          </div>
          <ErrorBoundary>
            <AtsTable
              atsList={atsList}
              isLoading={isLoading}
              isError={isError}
              refetch={refetch}
              onUpdate={(name, data) => mutation.mutateAsync({ name, data })}
            />
          </ErrorBoundary>
        </section>
      </div>
    </div>
  )
}
