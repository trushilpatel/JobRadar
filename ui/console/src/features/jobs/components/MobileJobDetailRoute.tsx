import { useCallback, useEffect, useRef, useState } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import type { Job, JobStatus } from '@/types'
import { useJob } from '@/features/jobs/hooks/useJob'
import { useJobActions } from '@/features/jobs/hooks/useJobActions'
import { JobDetailPanel } from '@/features/jobs/components/JobDetailPanel'
import { DismissModal, type DismissPayload } from '@/features/jobs/components/DismissModal'

export function MobileJobDetailRoute() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const location = useLocation()
  const job = useJob(id)
  const actions = useJobActions()
  const [dismissTarget, setDismissTarget] = useState<Job | null>(null)

  // Track whether the user arrived here from inside the app (so navigate(-1)
  // is safe) or via deep-link / refresh (so we must fall back to /jobs).
  // react-router tags entries with a numeric `idx` on its location state —
  // idx > 0 means we entered via an in-app push.
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

  function handleStatusChange(target: Job, status: Exclude<JobStatus, 'dismissed'>) {
    actions.setStatus.mutate({ id: target.id, status })
    safeBack()
  }

  function handleDismissConfirm(payload: DismissPayload) {
    if (!dismissTarget) return
    actions.dismiss.mutate({
      id: dismissTarget.id,
      reason: payload.reason,
      note: payload.note,
    })
    setDismissTarget(null)
    safeBack()
  }

  if (job.isLoading) {
    return (
      <div className="flex flex-1 flex-col gap-3 p-6">
        <div className="h-6 w-2/3 animate-pulse rounded bg-white/[0.05]" />
        <div className="h-4 w-1/3 animate-pulse rounded bg-white/[0.05]" />
        <div className="mt-4 h-32 animate-pulse rounded bg-white/[0.05]" />
      </div>
    )
  }

  if (job.isError || !job.data) {
    // The api-client throws generic Error instances without a status field, so
    // we can only distinguish broad categories from the message. 404-ish
    // messages get a dedicated "not found" affordance; everything else gets
    // Retry. See docs/superpowers/specs for the longer note.
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
              className="rounded-lg border border-white/10 bg-white/5 px-3 py-1.5 text-sm text-slate-300"
            >
              Browse all jobs
            </button>
          ) : !isForbidden ? (
            <button
              type="button"
              onClick={() => job.refetch()}
              className="rounded-lg border border-white/10 bg-white/5 px-3 py-1.5 text-sm text-slate-300"
            >
              Try again
            </button>
          ) : null}
          <button
            type="button"
            onClick={safeBack}
            className="rounded-lg border border-white/10 bg-white/5 px-3 py-1.5 text-sm text-slate-300"
          >
            Back
          </button>
        </div>
      </div>
    )
  }

  return (
    <>
      <JobDetailPanel
        job={job.data}
        mode="route"
        onClose={safeBack}
        onStatusChange={handleStatusChange}
        onDismiss={(target) => setDismissTarget(target)}
      />
      <DismissModal
        key={dismissTarget?.id ?? ''}
        open={!!dismissTarget}
        jobTitle={dismissTarget?.title ?? ''}
        onConfirm={handleDismissConfirm}
        onCancel={() => setDismissTarget(null)}
      />
    </>
  )
}
