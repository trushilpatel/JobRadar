import { useMemo } from 'react'
import { DataTable, type SortingState } from '@/components/ui/data-table'
import type { Job } from '@/types'
import type { TabKey } from '@/features/jobs/hooks/useJobs'
import { buildJobColumns } from '@/features/jobs/components/jobs-columns'

interface JobsTableProps {
  jobs: Job[]
  total: number
  loading: boolean
  tab: TabKey
  page: number
  limit: number
  selectedIds: Set<string>
  activeJobId: string | null
  onRowClick: (job: Job) => void
  onToggleSelect: (id: string, checked: boolean) => void
  onSelectAll: (checked: boolean) => void
  onPageChange: (page: number) => void
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
}

export function JobsTable({
  jobs,
  total,
  loading,
  tab,
  page,
  limit,
  selectedIds,
  activeJobId,
  onRowClick,
  onToggleSelect,
  onSelectAll,
  onPageChange,
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
    <div className="flex-1 overflow-auto min-w-0">
      <DataTable
        columns={columns}
        data={jobs}
        loading={loading}
        emptyMessage={EMPTY_MESSAGES[tab]}
        getRowId={(j) => j.id}
        onRowClick={onRowClick}
        getRowClassName={(job) => {
          if (selectedIds.has(job.id)) return 'bg-violet-500/[0.06]'
          if (activeJobId === job.id) return 'bg-violet-500/[0.08]'
          return 'hover:bg-white/[0.03]'
        }}
        sorting={sorting}
        onSortingChange={onSortingChange}
        serverPagination={{
          page,
          limit,
          total,
          onPageChange,
          onLimitChange,
        }}
      />
    </div>
  )
}
