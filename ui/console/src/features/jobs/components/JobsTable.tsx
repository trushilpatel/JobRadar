import { useMemo } from 'react'
import { DataTable, type SortingState } from '@/components/ui/data-table'
import { Pagination } from '@/components/ui/pagination'
import type { Job } from '@/types'
import type { TabKey } from '@/features/jobs/hooks/useJobs'
import { buildJobColumns } from '@/features/jobs/components/jobs-columns'

interface JobsTableProps {
  jobs: Job[]
  loading: boolean
  tab: TabKey
  page: number
  limit: number
  itemsOnPage: number
  hasNextPage: boolean
  hasPrevPage: boolean
  knownTotal?: number
  hasActiveFilters?: boolean
  selectedIds: Set<string>
  activeJobId: string | null
  onRowClick: (job: Job) => void
  onToggleSelect: (id: string, checked: boolean) => void
  onSelectAll: (checked: boolean) => void
  onNextPage: () => void
  onPrevPage: () => void
  onLimitChange: (limit: number) => void
  sorting: SortingState
  onSortingChange: (sorting: SortingState) => void
}

const EMPTY_MESSAGES: Record<TabKey, string> = {
  new: 'No new jobs — run a scan to populate the feed',
  saved: 'No saved jobs yet',
  applied: 'No applied jobs yet',
  interviewing: 'No jobs in interviewing stage',
  offer: 'No jobs with offers',
  rejected: 'No rejected jobs yet',
}

export function JobsTable({
  jobs,
  loading,
  tab,
  page,
  limit,
  itemsOnPage,
  hasNextPage,
  hasPrevPage,
  knownTotal,
  hasActiveFilters = false,
  selectedIds,
  activeJobId,
  onRowClick,
  onToggleSelect,
  onSelectAll,
  onNextPage,
  onPrevPage,
  onLimitChange,
  sorting,
  onSortingChange,
}: JobsTableProps) {
  const columns = useMemo(
    () =>
      buildJobColumns({
        selectedIds,
        filteredIds: jobs.map((j) => j.id),
        onToggleSelect,
        onSelectAll,
      }),
    [selectedIds, jobs, onToggleSelect, onSelectAll]
  )

  return (
    <div className="flex flex-col flex-1 overflow-hidden min-w-0">
      <div className="flex-1 overflow-auto">
        <DataTable
          columns={columns}
          data={jobs}
          loading={loading}
          emptyMessage={
            hasActiveFilters
              ? 'No results match your current filters'
              : EMPTY_MESSAGES[tab]
          }
          getRowId={(j) => j.id}
          onRowClick={onRowClick}
          getRowClassName={(job) => {
            if (selectedIds.has(job.id)) return 'bg-violet-500/[0.06]'
            if (activeJobId === job.id) return 'bg-violet-500/[0.08]'
            return 'hover:bg-white/[0.03]'
          }}
          sorting={sorting}
          onSortingChange={onSortingChange}
        />
      </div>

      <div className="border-t border-white/[0.07] shrink-0">
        <Pagination
          mode="cursor"
          page={page}
          limit={limit}
          itemsOnPage={itemsOnPage}
          hasNext={hasNextPage}
          hasPrev={hasPrevPage}
          knownTotal={knownTotal}
          onNext={onNextPage}
          onPrev={onPrevPage}
          onLimitChange={onLimitChange}
          loading={loading}
        />
      </div>
    </div>
  )
}
