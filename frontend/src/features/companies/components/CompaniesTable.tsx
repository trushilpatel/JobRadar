import { useMemo } from 'react'
import { DataTable } from '@/components/ui/data-table'
import type { Company } from '@/types'
import { buildCompanyColumns, type CompanyColumnActions } from './companies-columns'

interface CompaniesTableProps {
  companies: Company[]
  total: number
  loading: boolean
  page: number
  limit: number
  onPageChange: (page: number) => void
  onLimitChange: (limit: number) => void
  onRowClick: (c: Company) => void
  activeCompanyId?: string
  actions: CompanyColumnActions
  emptyMessage?: string
}

export function CompaniesTable({
  companies, total, loading,
  page, limit, onPageChange, onLimitChange,
  onRowClick, activeCompanyId,
  actions,
  emptyMessage = 'No companies match the current filters',
}: CompaniesTableProps) {
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const columns = useMemo(
    () => buildCompanyColumns(actions),
    [actions.scanning, actions.queueMap],
  )

  return (
    <div className="flex-1 overflow-auto min-w-0 px-6 py-3">
      <DataTable
        columns={columns}
        data={companies}
        loading={loading}
        emptyMessage={emptyMessage}
        getRowId={c => c.id}
        selectedId={activeCompanyId}
        onRowClick={onRowClick}
        serverPagination={{
          page, limit, total,
          onPageChange,
          onLimitChange,
        }}
      />
    </div>
  )
}
