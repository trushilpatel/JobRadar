import { useMemo } from 'react'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { DataTable, type SortingState } from '@/components/ui/data-table'
import type { Job } from '@/types'
import type { TabKey } from '@/features/jobs/hooks/useJobs'
import { buildJobColumns } from '@/features/jobs/components/jobs-columns'

interface JobsTableProps {
  jobs: Job[]
  loading: boolean
  tab: TabKey
  page: number
  limit: number
  hasNextPage: boolean
  hasPrevPage: boolean
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
}

const LIMIT_OPTIONS = [20, 50, 100]

export function JobsTable({
  jobs,
  loading,
  tab,
  page,
  limit,
  hasNextPage,
  hasPrevPage,
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
        />
      </div>

      {/* Cursor pagination controls */}
      <div className="flex items-center justify-between px-4 py-2.5 border-t border-white/[0.07] shrink-0">
        <div className="flex items-center gap-2">
          <span className="text-xs text-slate-500">Rows</span>
          <select
            value={limit}
            onChange={(e) => onLimitChange(Number(e.target.value))}
            className="bg-white/[0.04] border border-white/[0.08] text-xs text-slate-300 rounded-lg px-2 py-1.5 focus:outline-none focus:border-violet-500 transition-colors cursor-pointer"
          >
            {LIMIT_OPTIONS.map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
          </select>
        </div>
        <div className="flex items-center gap-1">
          <button
            onClick={onPrevPage}
            disabled={!hasPrevPage}
            className="flex items-center gap-1 px-2.5 py-1.5 text-xs text-slate-400 hover:text-white hover:bg-white/[0.06] rounded-lg transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
          >
            <ChevronLeft size={13} /> Prev
          </button>
          <span className="px-2 text-xs text-slate-500">Page {page}</span>
          <button
            onClick={onNextPage}
            disabled={!hasNextPage}
            className="flex items-center gap-1 px-2.5 py-1.5 text-xs text-slate-400 hover:text-white hover:bg-white/[0.06] rounded-lg transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
          >
            Next <ChevronRight size={13} />
          </button>
        </div>
      </div>
    </div>
  )
}
