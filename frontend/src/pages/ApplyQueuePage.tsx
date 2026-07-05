import { SegmentedSwitch } from '@/components/ui/segmented-switch'
import { ErrorBoundary } from '@/components/shared/ErrorBoundary'
import {
  useApplyQueue,
  useSetApplyQueueResume,
  ApplyQueueTable,
  APPLY_QUEUE_FILTER,
  type ApplyQueueFilter,
  type ApplyQueueResumeOption,
} from '@/features/apply-queue'
import { useResumes } from '@/features/resumes'

export function ApplyQueuePage() {
  const q = useApplyQueue()
  const { data: resumes } = useResumes()
  const setResume = useSetApplyQueueResume()

  const resumeOptions: ApplyQueueResumeOption[] = (resumes ?? []).map((r) => ({
    id: r.id,
    label: r.label,
  }))

  function handleResumeChange(jobId: string, resumeId: string | null) {
    setResume.mutate({ jobId, resumeId })
  }

  function isResumeUpdatePending(jobId: string): boolean {
    return setResume.isPending && setResume.variables?.jobId === jobId
  }

  return (
    <div className="flex-1 flex flex-col min-h-0 overflow-hidden">
      <div className="flex items-center justify-between gap-4 px-6 py-5 border-b border-[var(--border)] shrink-0 flex-wrap">
        <div>
          <h1 className="text-2xl font-semibold text-primary">Apply Queue</h1>
          <p className="text-sm text-muted mt-1">
            Jobs you've saved or applied to — attach a resume to each before you apply.
          </p>
        </div>
        <SegmentedSwitch<ApplyQueueFilter>
          aria-label="Apply queue filter"
          value={q.filter}
          onChange={q.setFilter}
          options={[
            { value: APPLY_QUEUE_FILTER.ALL, label: 'All', accent: 'violet' },
            { value: APPLY_QUEUE_FILTER.SAVED, label: 'Saved', accent: 'violet' },
          ]}
        />
      </div>

      <ErrorBoundary>
        <ApplyQueueTable
          items={q.items}
          loading={q.isLoading}
          isError={q.isError}
          onRetry={q.refetch}
          emptyMessage={
            q.filter === APPLY_QUEUE_FILTER.SAVED
              ? 'No saved jobs yet'
              : 'No jobs in your apply queue yet'
          }
          resumeOptions={resumeOptions}
          onResumeChange={handleResumeChange}
          isResumeUpdatePending={isResumeUpdatePending}
          page={q.page}
          itemsOnPage={q.itemsOnPage}
          itemsBeforePage={q.itemsBeforePage}
          limit={q.limit}
          onLimitChange={q.setLimit}
          hasNextPage={q.hasNextPage}
          hasPrevPage={q.hasPrevPage}
          onNext={q.goToNextPage}
          onPrev={q.goToPrevPage}
          onFirst={q.goToFirstPage}
        />
      </ErrorBoundary>
    </div>
  )
}
