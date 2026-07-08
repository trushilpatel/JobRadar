import { useEffect, useMemo, useRef } from 'react'
import { DataTable, type SortingState } from '@/components/ui/data-table'
import type { Job } from '@/types'
import type { TabKey } from '../hooks/useJobs'
import { buildJobColumns } from './jobs-columns'
import { cn } from '@/lib/utils/utils'

// Matches the approved mockup's sentinel: begin fetching the next page once
// the sentinel is within 300px of entering the scroll viewport, so the next
// page is usually ready before the user reaches the bottom.
const SENTINEL_ROOT_MARGIN = '300px'

interface JobsTableProps {
  jobs: Job[]
  loading: boolean
  isFetchingMore?: boolean
  isError?: boolean
  onRetry?: () => void
  tab: TabKey
  hasNextPage: boolean
  hasActiveFilters?: boolean
  activeJobId: string | null
  /** Index of the keyboard-focused row. -1 disables the focus indicator. */
  focusedIndex?: number
  onRowClick: (job: Job) => void
  onToggleSave: (job: Job) => void
  onApply: (job: Job) => void
  /** Optional callback injected by the pages layer to provide cross-feature
   *  data (e.g. resume attachment state) without creating a cross-feature
   *  import inside features/jobs. */
  getAttachmentState?: (jobId: string) => string | null
  /** Fetches and appends the next page — fired by the infinite-scroll sentinel. */
  onLoadMore: () => void
  sorting: SortingState
  onSortingChange: (sorting: SortingState) => void
}

const EMPTY_MESSAGES: Record<TabKey, string> = {
  new: 'No new jobs — run a scan to populate the feed',
  saved: 'No saved jobs yet — bookmark a job to save it here',
  applied: 'No applied jobs yet',
  interviewing: 'No jobs in interviewing stage',
  offer: 'No jobs with offers',
  rejected: 'No rejected jobs yet',
  tracking: 'No jobs in the selected stages',
}

export function JobsTable({
  jobs,
  loading,
  isFetchingMore = false,
  isError,
  onRetry,
  tab,
  hasNextPage,
  hasActiveFilters = false,
  activeJobId,
  focusedIndex,
  onRowClick,
  onToggleSave,
  onApply,
  getAttachmentState,
  onLoadMore,
  sorting,
  onSortingChange,
}: JobsTableProps) {
  const columns = useMemo(
    () => buildJobColumns({ onToggleSave, onApply, getAttachmentState }),
    [onToggleSave, onApply, getAttachmentState]
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

  // Infinite-scroll sentinel — observes its own visibility within the
  // scrollable container (not the viewport) and fetches the next page once
  // it comes within SENTINEL_ROOT_MARGIN, matching the approved mockup.
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const sentinelRef = useRef<HTMLDivElement | null>(null)
  const onLoadMoreRef = useRef(onLoadMore)
  useEffect(() => {
    onLoadMoreRef.current = onLoadMore
  }, [onLoadMore])

  useEffect(() => {
    const sentinel = sentinelRef.current
    const root = scrollRef.current
    if (!sentinel || !root || !hasNextPage) return

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) onLoadMoreRef.current()
      },
      { root, rootMargin: SENTINEL_ROOT_MARGIN }
    )
    observer.observe(sentinel)
    return () => observer.disconnect()
  }, [hasNextPage, jobs.length])

  // Compose a screen-reader-friendly row label.
  function rowLabel(job: Job): string {
    const company = job.company?.name ?? job.company_id ?? ''
    return `${job.title}${company ? ` at ${company}` : ''}, status ${job.status}`
  }

  return (
    <div ref={containerRef} className="flex flex-col flex-1 overflow-hidden min-w-0">
      <div ref={scrollRef} className="flex-1 overflow-auto">
        <DataTable
          columns={columns}
          data={jobs}
          loading={loading && jobs.length === 0}
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
            // Avoid O(n) findIndex: derive focused job id directly from the
            // pre-computed focusedIndex so the comparison is O(1).
            const focusedJobId =
              focusedIndex !== undefined && focusedIndex >= 0
                ? (jobs[focusedIndex]?.id ?? null)
                : null
            const isFocused = focusedJobId !== null && job.id === focusedJobId
            const isActive = activeJobId === job.id
            return cn(
              isActive ? 'bg-[var(--accent-dim)]' : isFocused ? 'bg-hover' : 'hover:bg-hover'
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
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet focus-visible:ring-inset',
            // Reuses the `tr[data-has-edge]:hover` accent-bar utility already
            // established for the Companies table (index.css) — jobs have no
            // per-row hue source (unlike company adapter tints), so every row
            // gets the same brand-violet edge on hover.
            'data-has-edge': true,
            style: { '--row-edge': 'var(--violet)' } as React.CSSProperties,
          })}
          sorting={sorting}
          onSortingChange={onSortingChange}
        />

        {jobs.length > 0 && (hasNextPage || isFetchingMore) && (
          <div
            ref={sentinelRef}
            data-testid="jobs-table-sentinel"
            className="grid place-items-center py-5 text-xs text-muted"
          >
            {isFetchingMore && <span className="animate-pulse">Loading more roles…</span>}
          </div>
        )}
      </div>
    </div>
  )
}
