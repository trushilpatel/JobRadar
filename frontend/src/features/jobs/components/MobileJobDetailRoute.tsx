import { useCallback, useEffect, useRef } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import type { Job, JobStatus } from '@/types'
import { useJob } from '../hooks/useJob'
import { useJobActions } from '../hooks/useJobActions'
import { JobDetailPanel } from './JobDetailPanel'
import { showDismissStripe } from '@/components/ui/dismiss-stripe'
import { HIDDEN_COMPANIES_CATEGORY } from '@/config/constants/hiddenItems'

export function MobileJobDetailRoute() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const location = useLocation()
  const job = useJob(id)
  const actions = useJobActions()

  // Track whether the user arrived here from inside the app (so navigate(-1)
  // is safe) or via deep-link / refresh (so we must fall back to /jobs).
  const enteredViaInAppNavRef = useRef<boolean>(false)
  useEffect(() => {
    const idx = (location.state as { idx?: number } | null)?.idx ?? 0
    if (idx > 0) enteredViaInAppNavRef.current = true
  }, [location.state])

  const safeBack = useCallback(() => {
    if (enteredViaInAppNavRef.current || window.history.length > 1) {
      navigate(-1)
    } else {
      navigate('/jobs')
    }
  }, [navigate])

  const handleStatusChange = useCallback(
    (target: Job, status: Exclude<JobStatus, 'dismissed' | 'saved'>) => {
      actions.setStatus.mutate({ id: target.id, status })
      safeBack()
    },
    [actions.setStatus, safeBack]
  )

  // Toggle the orthogonal saved flag without leaving the panel — saving a
  // job doesn't move it out of its workflow stage. Stays on the detail view
  // so the user can take another action.
  const handleToggleSave = useCallback(
    (target: Job) => {
      const currentlySaved = target.user_state?.saved_at != null
      actions.setSaved.mutate({ id: target.id, saved: !currentlySaved })
    },
    [actions.setSaved]
  )

  // Mobile dismiss mirrors the desktop flow — no modal, no reason picker,
  // optional "Don't show <Company> again" via DismissStripe.
  const handleDismiss = useCallback(
    (target: Job) => {
      const companyName = target.company?.name ?? ''
      actions.dismiss.mutate({ id: target.id, reason: 'user_dismissed' })
      showDismissStripe({
        message: 'Dismissed',
        submessage: `${target.title}${companyName ? ` at ${companyName}` : ''}`,
        onUndo: () => actions.undoLast(),
        hideTarget: companyName
          ? { category: HIDDEN_COMPANIES_CATEGORY, value: companyName, label: companyName }
          : undefined,
      })
      safeBack()
    },
    [actions, safeBack]
  )

  if (job.isLoading) {
    return (
      <div className="flex flex-1 flex-col gap-3 p-6">
        <div className="h-6 w-2/3 animate-pulse rounded bg-subtle" />
        <div className="h-4 w-1/3 animate-pulse rounded bg-subtle" />
        <div className="mt-4 h-32 animate-pulse rounded bg-subtle" />
      </div>
    )
  }

  if (job.isError || !job.data) {
    const msg = job.error instanceof Error ? job.error.message : ''
    const isNotFound = /404|not\s*found/i.test(msg)
    const isForbidden = /401|403|forbidden|unauthorized/i.test(msg)

    return (
      <div className="flex flex-1 flex-col items-start gap-3 p-6">
        <p className="text-sm text-red-400">
          {isNotFound
            ? "We couldn't find that job."
            : isForbidden
              ? "You don't have access to this job."
              : "Couldn't load this job."}
        </p>
        <div className="flex gap-2">
          {isNotFound ? (
            <button
              type="button"
              onClick={() => navigate('/jobs')}
              className="rounded-lg border border-[var(--border-strong)] bg-subtle px-3 py-1.5 text-sm text-secondary"
            >
              Browse all jobs
            </button>
          ) : !isForbidden ? (
            <button
              type="button"
              onClick={() => job.refetch()}
              className="rounded-lg border border-[var(--border-strong)] bg-subtle px-3 py-1.5 text-sm text-secondary"
            >
              Try again
            </button>
          ) : null}
          <button
            type="button"
            onClick={safeBack}
            className="rounded-lg border border-[var(--border-strong)] bg-subtle px-3 py-1.5 text-sm text-secondary"
          >
            Back
          </button>
        </div>
      </div>
    )
  }

  return (
    <JobDetailPanel
      job={job.data}
      mode="route"
      onClose={safeBack}
      onStatusChange={handleStatusChange}
      onToggleSave={handleToggleSave}
      onDismiss={handleDismiss}
    />
  )
}
