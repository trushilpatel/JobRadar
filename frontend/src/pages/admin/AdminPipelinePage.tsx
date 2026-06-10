import { ErrorBoundary } from '@/components/shared/ErrorBoundary'
import { AtsTable } from '@/features/settings'
import { useAts, useAtsActions } from '@/features/ats'

export function AdminPipelinePage() {
  const { data: atsList, isLoading, isError, refetch } = useAts()
  const mutation = useAtsActions()

  return (
    <div className="flex-1 overflow-y-auto p-6 max-w-4xl mx-auto w-full">
      <div className="mb-6">
        <h1 className="text-lg font-semibold text-primary">Pipeline Config</h1>
        <p className="text-sm text-muted mt-1">
          Per-adapter scan intervals and inter-company delays. Changes apply to all companies using
          that adapter unless a company-level override is set.
        </p>
      </div>
      <section>
        <div className="mb-4">
          <h2 className="text-sm font-semibold text-primary">Adapter Configuration</h2>
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
  )
}
