import { useEffect, useMemo, useRef } from 'react'
import { DataTable, type SortingState } from '@/components/ui/data-table'
import { Pagination } from '@/components/ui/pagination'
import type { Job } from '@/types'
import type { TabKey } from '@/features/jobs/hooks/useJobs'
import { buildJobColumns } from '@/features/jobs/components/jobs-columns'
import { cn } from '@/lib/utils'

interface JobsTableProps {
  jobs: Job[]
  loading: boolean
  tab: TabKey
  page: number
  limit: number
  itemsOnPage: number
  itemsBeforePage: number
  hasNextPage: boolean
  hasPrevPage: boolean
  knownTotal?: number
  hasActiveFilters?: boolean
  selectedIds: Set<string>
  activeJobId: string | null
  /** Index of the keyboard-focused row. -1 disables the focus indicator. */
  focusedIndex?: number
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
  itemsBeforePage,
  hasNextPage,
  hasPrevPage,
  knownTotal,
  hasActiveFilters = false,
  selectedIds,
  activeJobId,
  focusedIndex,
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

  // Scroll the focused row into view whenever the focused index changes.
  // We can't add a data-attribute to DataTable rows without modifying that
  // shared primitive, so target rows by position (tbody tr:nth-child) — the
  // table renders rows in index order. Safe today; the day we virtualise,
  // we'll add a typed prop to DataTable.
  const containerRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    if (focusedIndex === undefined || focusedIndex < 0) return
    const el = containerRef.current?.querySelector<HTMLElement>(
      `tbody tr:nth-child(${focusedIndex + 1})`
    )
    el?.scrollIntoView({ block: 'nearest' })
  }, [focusedIndex])

  return (
    <div ref={containerRef} className="flex flex-col flex-1 overflow-hidden min-w-0">
      <div className="flex-1 overflow-auto">
        <DataTable
          columns={columns}
          data={jobs}
          loading={loading}
          emptyMessage={
            hasActiveFilters ? 'No results match your current filters' : EMPTY_MESSAGES[tab]
          }
          getRowId={(j) => j.id}
          selectedId={activeJobId ?? undefined}
          onRowClick={onRowClick}
          getRowClassName={(job) => {
            const idx = jobs.findIndex((j) => j.id === job.id)
            const isFocused = focusedIndex !== undefined && focusedIndex === idx
            const isSelected = selectedIds.has(job.id)
            const isActive = activeJobId === job.id
            return cn(
              // Focus indicator: a violet left-border bar distinct from the
              // selected/active row backgrounds. Pre-emptively reserve the
              // left ring slot so non-focused rows stay aligned.
              isFocused && 'shadow-[inset_3px_0_0_0_rgb(167,139,250)]',
              isSelected
                ? 'bg-violet-500/[0.06]'
                : isActive
                  ? 'bg-violet-500/[0.08]'
                  : 'hover:bg-white/[0.03]'
            )
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
          itemsBeforePage={itemsBeforePage}
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
