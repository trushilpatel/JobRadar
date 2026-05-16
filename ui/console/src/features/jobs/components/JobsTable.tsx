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
  isError?: boolean
  onRetry?: () => void
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
  isError,
  onRetry,
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
  const containerRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    if (focusedIndex === undefined || focusedIndex < 0) return
    const el = containerRef.current?.querySelector<HTMLElement>(
      `tbody tr:nth-child(${focusedIndex + 1})`
    )
    el?.scrollIntoView({ block: 'nearest' })
  }, [focusedIndex])

  // Compose a screen-reader-friendly row label.
  function rowLabel(job: Job): string {
    const company = job.company?.name ?? job.company_id ?? ''
    return `${job.title}${company ? ` at ${company}` : ''}, status ${job.status}`
  }

  return (
    <div ref={containerRef} className="flex flex-col flex-1 overflow-hidden min-w-0">
      <div className="flex-1 overflow-auto">
        <DataTable
          columns={columns}
          data={jobs}
          loading={loading}
          isError={isError}
          onRetry={onRetry}
          errorMessage="Couldn't load jobs"
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
              // Focused row: solid violet inset ring — strong and unmistakable
              // even when the row is also `isActive`.
              isFocused && 'ring-2 ring-violet-400 ring-inset',
              isSelected
                ? 'bg-violet-500/[0.06]'
                : isActive
                  ? 'bg-violet-500/[0.08]'
                  : 'hover:bg-white/[0.03]'
            )
          }}
          getRowProps={(job) => ({
            tabIndex: 0,
            role: 'button',
            'aria-label': rowLabel(job),
            'aria-selected': activeJobId === job.id ? true : undefined,
            onKeyDown: (e: React.KeyboardEvent<HTMLTableRowElement>) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                onRowClick(job)
              }
            },
            className:
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400 focus-visible:ring-inset',
          })}
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
