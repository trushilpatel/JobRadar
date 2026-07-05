import { useMemo } from 'react'
import { DataTable } from '@/components/ui/data-table'
import { Pagination } from '@/components/ui/pagination'
import { cn } from '@/lib/utils/utils'
import type { ApplyQueueItem } from '../api/apply-queue.api'
import { buildApplyQueueColumns, type ApplyQueueColumnActions } from './apply-queue-columns'

interface ApplyQueueTableProps {
  items: ApplyQueueItem[]
  loading: boolean
  isError?: boolean
  onRetry?: () => void
  emptyMessage?: string

  // Resume picker + match wiring — injected by the pages layer (see
  // ResumePickerCell.tsx for why this feature never imports features/resumes
  // directly).
  resumeOptions: ApplyQueueColumnActions['resumeOptions']
  onResumeChange: ApplyQueueColumnActions['onResumeChange']
  isResumeUpdatePending?: ApplyQueueColumnActions['isResumeUpdatePending']
  getEffectiveResume: ApplyQueueColumnActions['getEffectiveResume']
  getMatchSummary: ApplyQueueColumnActions['getMatchSummary']

  // Row-click wiring — opens the job detail panel at the pages layer. Mirrors
  // features/jobs/components/JobsTable.tsx's onRowClick/getRowProps pattern
  // (keyboard Enter/Space activation, aria-selected on the open row). Optional
  // so this table stays usable without a detail panel wired up.
  selectedJobId?: string | null
  onRowClick?: (item: ApplyQueueItem) => void

  // Cursor pagination — prev/next only (not infinite scroll).
  page: number
  itemsOnPage: number
  itemsBeforePage: number
  limit: number
  onLimitChange: (limit: number) => void
  hasNextPage: boolean
  hasPrevPage: boolean
  onNext: () => void
  onPrev: () => void
  onFirst?: () => void
}

// Composes a screen-reader-friendly row label — mirrors JobsTable's rowLabel.
function rowLabel(item: ApplyQueueItem): string {
  return `${item.job_title}${item.company ? ` at ${item.company}` : ''}, status ${item.status}`
}

export function ApplyQueueTable({
  items,
  loading,
  isError,
  onRetry,
  emptyMessage = 'No jobs in your apply queue yet',
  resumeOptions,
  onResumeChange,
  isResumeUpdatePending,
  getEffectiveResume,
  getMatchSummary,
  selectedJobId,
  onRowClick,
  page,
  itemsOnPage,
  itemsBeforePage,
  limit,
  onLimitChange,
  hasNextPage,
  hasPrevPage,
  onNext,
  onPrev,
  onFirst,
}: ApplyQueueTableProps) {
  const columns = useMemo(
    () =>
      buildApplyQueueColumns({
        resumeOptions,
        onResumeChange,
        isResumeUpdatePending,
        getEffectiveResume,
        getMatchSummary,
      }),
    [resumeOptions, onResumeChange, isResumeUpdatePending, getEffectiveResume, getMatchSummary]
  )

  return (
    <div className="flex flex-col flex-1 overflow-hidden min-w-0">
      <div className="flex-1 overflow-auto px-6">
        <DataTable
          columns={columns}
          data={items}
          loading={loading}
          isError={isError}
          onRetry={onRetry}
          errorMessage="Couldn't load your apply queue"
          emptyMessage={emptyMessage}
          getRowId={(item) => item.job_id}
          selectedId={selectedJobId ?? undefined}
          onRowClick={onRowClick}
          getRowClassName={(item) => {
            const isActive = selectedJobId === item.job_id
            return cn('group/row', isActive ? 'bg-[var(--accent-dim)]' : 'hover:bg-hover')
          }}
          getRowProps={
            onRowClick
              ? (item) => ({
                  tabIndex: 0,
                  role: 'button',
                  'aria-label': rowLabel(item),
                  'aria-selected': selectedJobId === item.job_id ? true : undefined,
                  onKeyDown: (e: React.KeyboardEvent<HTMLTableRowElement>) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      onRowClick(item)
                    }
                  },
                  className:
                    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet focus-visible:ring-inset',
                })
              : undefined
          }
        />
      </div>

      <div className="border-t border-[var(--border)] shrink-0">
        <Pagination
          mode="cursor"
          page={page}
          itemsOnPage={itemsOnPage}
          itemsBeforePage={itemsBeforePage}
          limit={limit}
          onLimitChange={onLimitChange}
          hasNext={hasNextPage}
          hasPrev={hasPrevPage}
          onNext={onNext}
          onPrev={onPrev}
          onFirst={onFirst}
          loading={loading}
        />
      </div>
    </div>
  )
}
