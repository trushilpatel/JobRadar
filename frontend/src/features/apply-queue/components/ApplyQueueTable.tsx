import { useMemo, useState } from 'react'
import { DataTable } from '@/components/ui/data-table'
import { Pagination } from '@/components/ui/pagination'
import { cn } from '@/lib/utils/utils'
import type { ApplyQueueItem } from '../api/apply-queue.api'
import { useSetApplyQueueResume } from '../hooks/useSetApplyQueueResume'
import {
  buildApplyQueueColumns,
  type ApplyQueueColumnActions,
  type ResumeMatchOption,
} from './apply-queue-columns'
import { ResumePickerDialog } from './ResumePickerDialog'

interface ApplyQueueTableProps {
  items: ApplyQueueItem[]
  loading: boolean
  isError?: boolean
  onRetry?: () => void
  emptyMessage?: string

  // Resume match wiring — injected by the pages layer (this feature may not
  // import features/resumes directly; see useApplyQueueResumeMatch.ts). The
  // resume-attach mutation itself has no such dependency, so it's owned here.
  getEffectiveResume: ApplyQueueColumnActions['getEffectiveResume']
  getMatchSummary: ApplyQueueColumnActions['getMatchSummary']
  getResumeMatchOptions: (jobId: string) => ResumeMatchOption[]

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
  getEffectiveResume,
  getMatchSummary,
  getResumeMatchOptions,
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
  // Single shared dialog instance for the whole table, parameterized by
  // activeJobId — one row's trigger button only sets which job is active,
  // rather than every row owning its own Dialog/portal (see the resume-picker
  // dialog design: N rows must not mount N Radix Dialog instances).
  const [activeJobId, setActiveJobId] = useState<string | null>(null)
  const setResume = useSetApplyQueueResume()

  function isResumeUpdatePending(jobId: string): boolean {
    return setResume.isPending && setResume.variables?.jobId === jobId
  }

  const columns = useMemo(
    () =>
      buildApplyQueueColumns({
        isResumeUpdatePending,
        getEffectiveResume,
        getMatchSummary,
        onOpenResumePicker: setActiveJobId,
      }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [getEffectiveResume, getMatchSummary, setResume.isPending, setResume.variables?.jobId]
  )

  const activeItem = activeJobId ? (items.find((i) => i.job_id === activeJobId) ?? null) : null

  function handleConfirm(resumeId: string) {
    if (!activeJobId) return
    setResume.mutate({ jobId: activeJobId, resumeId }, { onSuccess: () => setActiveJobId(null) })
  }

  function handleClear() {
    if (!activeJobId) return
    setResume.mutate(
      { jobId: activeJobId, resumeId: null },
      { onSuccess: () => setActiveJobId(null) }
    )
  }

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

      {/* key forces a full remount on every job switch (and on close, since
          activeJobId -> null changes the key too) — the dialog always starts
          fresh from the server-committed resume_id, never a stale highlight
          or pending selection from a previous open. */}
      <ResumePickerDialog
        key={activeJobId ?? 'none'}
        open={activeJobId !== null}
        onOpenChange={(open) => {
          if (!open) setActiveJobId(null)
        }}
        jobTitle={activeItem?.job_title ?? ''}
        jobHasKeywords={(activeItem?.job_keywords.length ?? 0) > 0}
        currentResumeId={activeItem?.resume_id ?? null}
        options={activeJobId ? getResumeMatchOptions(activeJobId) : []}
        onConfirm={handleConfirm}
        onClear={handleClear}
        isPending={activeJobId ? isResumeUpdatePending(activeJobId) : false}
      />
    </div>
  )
}
