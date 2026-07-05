import { useMemo } from 'react'
import { DataTable } from '@/components/ui/data-table'
import { Pagination } from '@/components/ui/pagination'
import type { ApplyQueueItem } from '../api/apply-queue.api'
import { buildApplyQueueColumns, type ApplyQueueColumnActions } from './apply-queue-columns'

interface ApplyQueueTableProps {
  items: ApplyQueueItem[]
  loading: boolean
  isError?: boolean
  onRetry?: () => void
  emptyMessage?: string

  // Resume picker wiring — injected by the pages layer (see
  // ResumePickerCell.tsx for why this feature never imports features/resumes
  // directly).
  resumeOptions: ApplyQueueColumnActions['resumeOptions']
  onResumeChange: ApplyQueueColumnActions['onResumeChange']
  isResumeUpdatePending?: ApplyQueueColumnActions['isResumeUpdatePending']

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

export function ApplyQueueTable({
  items,
  loading,
  isError,
  onRetry,
  emptyMessage = 'No jobs in your apply queue yet',
  resumeOptions,
  onResumeChange,
  isResumeUpdatePending,
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
    () => buildApplyQueueColumns({ resumeOptions, onResumeChange, isResumeUpdatePending }),
    [resumeOptions, onResumeChange, isResumeUpdatePending]
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
