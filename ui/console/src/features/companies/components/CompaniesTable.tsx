import { useMemo } from 'react'
import { DataTable, type SortingState } from '@/components/ui/data-table'
import { Pagination } from '@/components/ui/pagination'
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
  sorting: SortingState
  onSortingChange: (sorting: SortingState) => void
}

export function CompaniesTable({
  companies, total, loading,
  page, limit, onPageChange, onLimitChange,
  onRowClick, activeCompanyId,
  actions,
  emptyMessage = 'No companies match the current filters',
  sorting,
  onSortingChange,
}: CompaniesTableProps) {
  const columns = useMemo(
    () => buildCompanyColumns(actions),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [actions.scanning, actions.queueMap],
  )

  return (
    <div className="flex flex-col flex-1 overflow-hidden min-w-0">
      <div className="flex-1 overflow-auto px-6">
        <DataTable
          columns={columns}
          data={companies}
          loading={loading}
          emptyMessage={emptyMessage}
          getRowId={c => c.id}
          selectedId={activeCompanyId}
          onRowClick={onRowClick}
          sorting={sorting}
          onSortingChange={onSortingChange}
        />
      </div>

      <div className="border-t border-white/[0.07] shrink-0">
        <Pagination
          page={page}
          total={total}
          limit={limit}
          onPageChange={onPageChange}
          onLimitChange={onLimitChange}
        />
      </div>
    </div>
  )
}
