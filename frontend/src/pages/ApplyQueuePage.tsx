import { useState, type CSSProperties } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { SegmentedSwitch } from '@/components/ui/segmented-switch'
import { ResizeHandle } from '@/components/ui/resize-handle'
import { ErrorBoundary } from '@/components/shared/ErrorBoundary'
import {
  useApplyQueue,
  ApplyQueueTable,
  applyQueueQueryKeys,
  APPLY_QUEUE_FILTER,
  type ApplyQueueFilter,
  type ApplyQueueItem,
} from '@/features/apply-queue'
import { useResumes } from '@/features/resumes'
import { useJob, useJobActions, JobDetailPanel } from '@/features/jobs'
import type { Job, JobStatus } from '@/types'
import { useApplyQueueResumeMatch } from './apply-queue/useApplyQueueResumeMatch'

const DEFAULT_PANEL_WIDTH = 480

export function ApplyQueuePage() {
  const q = useApplyQueue()
  const { data: resumes } = useResumes()
  const queryClient = useQueryClient()
  const jobActions = useJobActions()
  const { getEffectiveResume, getMatchSummary, getResumeMatchOptions } = useApplyQueueResumeMatch(
    q.items,
    resumes ?? []
  )

  // Which job's detail panel is open. Local component state — this page has
  // no URL-state hook of its own (see @/features/jobs/hooks/useJobUrlState
  // for the pattern that exists on the Jobs page); a plain useState matches
  // the project's "component-local UI state -> useState" rule.
  const [selectedJobId, setSelectedJobId] = useState<string | null>(null)
  const [panelWidth, setPanelWidth] = useState(DEFAULT_PANEL_WIDTH)
  const selectedJob = useJob(selectedJobId ?? undefined)

  // The detail panel's actions (status/save) live in features/jobs's
  // useJobActions and only invalidate that feature's own query keys. A change
  // made from here can also move the job in or out of the Apply Queue list
  // (e.g. unsaving an otherwise-untouched job removes it) — so every
  // mutation here additionally busts the apply-queue cache.
  // Composed at this page level rather than inside useJobActions itself,
  // which must stay unaware of apply-queue's query keys (features/jobs may
  // not depend on another feature).
  function invalidateApplyQueue() {
    void queryClient.invalidateQueries({ queryKey: applyQueueQueryKeys.all() })
  }

  function handlePanelClose() {
    setSelectedJobId(null)
  }

  function handleRowClick(item: ApplyQueueItem) {
    setSelectedJobId(item.job_id)
  }

  function handleStatusChange(job: Job, status: Exclude<JobStatus, 'dismissed' | 'saved'>) {
    jobActions.setStatus.mutate({ id: job.id, status }, { onSuccess: invalidateApplyQueue })
  }

  function handleToggleSave(job: Job) {
    const currentlySaved = job.user_state?.saved_at != null
    jobActions.setSaved.mutate(
      { id: job.id, saved: !currentlySaved },
      { onSuccess: invalidateApplyQueue }
    )
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
        {/* Table + on-demand detail panel as flex siblings (not a portal/
            overlay) so the table genuinely reflows to make room — same
            pattern as CompaniesPage's drawer. */}
        <div className="flex-1 flex min-h-0 overflow-hidden">
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
            getEffectiveResume={getEffectiveResume}
            getMatchSummary={getMatchSummary}
            getResumeMatchOptions={getResumeMatchOptions}
            selectedJobId={selectedJobId}
            onRowClick={handleRowClick}
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

          {selectedJobId && (
            <>
              <ResizeHandle width={panelWidth} onChange={setPanelWidth} />
              <aside
                style={{ ['--panel-width' as string]: `${panelWidth}px` } as CSSProperties}
                className="flex flex-col shrink-0 min-h-0 h-full border-l border-[var(--border)] overflow-hidden shadow-[0_8px_24px_rgba(0,0,0,0.4)] [width:var(--panel-width)]"
              >
                {selectedJob.isLoading ? (
                  <div className="flex flex-1 flex-col gap-3 p-6">
                    <div className="h-6 w-2/3 animate-pulse rounded bg-subtle" />
                    <div className="h-4 w-1/3 animate-pulse rounded bg-subtle" />
                    <div className="mt-4 h-32 animate-pulse rounded bg-subtle" />
                  </div>
                ) : selectedJob.isError || !selectedJob.data ? (
                  <div className="flex flex-1 flex-col items-start gap-3 p-6">
                    <p className="text-sm text-red-400">Couldn't load this job.</p>
                    <button
                      type="button"
                      onClick={handlePanelClose}
                      className="rounded-lg border border-[var(--border-strong)] bg-subtle px-3 py-1.5 text-sm text-secondary"
                    >
                      Close
                    </button>
                  </div>
                ) : (
                  <JobDetailPanel
                    job={selectedJob.data}
                    onClose={handlePanelClose}
                    onStatusChange={handleStatusChange}
                    onToggleSave={handleToggleSave}
                  />
                )}
              </aside>
            </>
          )}
        </div>
      </ErrorBoundary>
    </div>
  )
}
