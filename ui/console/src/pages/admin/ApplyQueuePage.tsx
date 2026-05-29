import { useState } from 'react'
import { Link } from 'react-router-dom'
import { cn } from '@/lib/utils/utils'
import { routes, type ApplyQueueFilter } from '@/config/routes'
import { useApplyQueue, ApplyQueueTable } from '@/features/resumes'
import { ErrorBoundary } from '@/components/shared/ErrorBoundary'

const FILTER_TABS: { key: ApplyQueueFilter; label: string }[] = [
  { key: 'all', label: 'All' },
  { key: 'needs-resume', label: 'Needs resume' },
  { key: 'draft', label: 'Draft' },
  { key: 'final', label: 'Final' },
  { key: 'submitted', label: 'Submitted' },
  { key: 'interviewing', label: 'Interviewing' },
]

const EMPTY_BY_TAB: Record<ApplyQueueFilter, string> = {
  all: 'No jobs in your apply queue yet. Save jobs from the Jobs feed to start.',
  'needs-resume': 'All saved jobs already have a resume attached.',
  draft: 'No drafts in progress.',
  final: 'No resumes marked final yet.',
  submitted: 'No submitted applications yet.',
  interviewing: 'No interviewing-stage jobs with an attached resume.',
}

export function ApplyQueuePage() {
  const [filter, setFilter] = useState<ApplyQueueFilter>('all')
  const { data, isLoading } = useApplyQueue(filter)

  return (
    <div className="flex-1 flex flex-col min-h-0 overflow-auto">
      <div className="px-6 py-6 w-full space-y-6">
        {/* Header */}
        <div className="flex items-start justify-between gap-6 flex-wrap">
          <div>
            <h1 className="text-2xl font-semibold text-white">Apply Queue</h1>
            <p className="text-sm text-slate-400 mt-1">
              Every job sitting between <em>saved</em> and <em>submitted</em>. Click a row to open
              the job on the Jobs page; pick a resume inline to attach it.
            </p>
          </div>
          <Link
            to={routes.resumeLibrary()}
            className="text-xs text-slate-400 hover:text-white px-2.5 py-1 rounded bg-white/[0.04] hover:bg-white/[0.08]"
          >
            ← Resume library
          </Link>
        </div>

        {/* Tabs */}
        <div className="flex items-center gap-1 border-b border-white/[0.07] overflow-x-auto">
          {FILTER_TABS.map((t) => (
            <button
              key={t.key}
              onClick={() => setFilter(t.key)}
              className={cn(
                'text-sm px-3 py-2 -mb-px border-b-2 transition-colors whitespace-nowrap',
                filter === t.key
                  ? 'border-violet-500 text-white'
                  : 'border-transparent text-slate-400 hover:text-white'
              )}
            >
              {t.label}
            </button>
          ))}
        </div>

        <ErrorBoundary>
          <ApplyQueueTable
            items={data ?? []}
            isLoading={isLoading}
            emptyMessage={EMPTY_BY_TAB[filter]}
          />
        </ErrorBoundary>
      </div>
    </div>
  )
}
