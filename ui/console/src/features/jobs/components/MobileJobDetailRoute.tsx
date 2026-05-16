import { useNavigate, useParams } from 'react-router-dom'
import { useState } from 'react'
import type { Job, JobStatus } from '@/types'
import { useJob } from '@/features/jobs/hooks/useJob'
import { useJobActions } from '@/features/jobs/hooks/useJobActions'
import { JobDetailPanel } from '@/features/jobs/components/JobDetailPanel'
import { DismissModal } from '@/features/jobs/components/DismissModal'

export function MobileJobDetailRoute() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const job = useJob(id)
  const actions = useJobActions()
  const [dismissTarget, setDismissTarget] = useState<Job | null>(null)

  function handleStatusChange(target: Job, status: Exclude<JobStatus, 'dismissed'>) {
    actions.setStatus.mutate({ id: target.id, status })
    navigate(-1)
  }

  function handleDismissConfirm(reason: string) {
    if (!dismissTarget) return
    actions.dismiss.mutate({ id: dismissTarget.id, reason })
    setDismissTarget(null)
    navigate(-1)
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
    return (
      <div className="flex flex-1 flex-col items-start gap-3 p-6">
        <p className="text-sm text-red-400">Could not load this job.</p>
        <button
          type="button"
          onClick={() => navigate(-1)}
          className="rounded-lg border border-white/10 bg-white/5 px-3 py-1.5 text-sm text-slate-300"
        >
          Back
        </button>
      </div>
    )
  }

  return (
    <>
      <JobDetailPanel
        job={job.data}
        mode="route"
        onClose={() => navigate(-1)}
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
