import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { setJobStatus, setJobSaved, dismissJob, bulkDismissJobs } from '@/api/jobs'
import type { JobStatus } from '@/types'
import { JOB_STATUS } from '@/constants/jobStatus'

export function useJobActions() {
  const queryClient = useQueryClient()
  const invalidateJobs = () => queryClient.invalidateQueries({ queryKey: ['jobs'] })

  const setStatus = useMutation({
    mutationFn: ({ id, status }: { id: string; status: Exclude<JobStatus, 'dismissed'> }) => {
      // 'new' is synthetic — no row in user_job_state. Moving back to 'new'
      // requires deleting the row via setJobSaved(false), not setting status='new'.
      if (status === JOB_STATUS.NEW) return setJobSaved(id, false)
      return setJobStatus(id, status)
    },
    onSuccess: (_, { status }) => {
      toast.success(status === JOB_STATUS.NEW ? 'Moved back to New' : `Moved to ${status}`)
      invalidateJobs()
    },
    onError: () => toast.error('Failed to update status'),
  })

  const dismiss = useMutation({
    mutationFn: ({ id, reason, note }: { id: string; reason: string; note?: string }) =>
      dismissJob(id, reason, note),
    onSuccess: () => {
      toast.success('Job dismissed')
      invalidateJobs()
    },
    onError: () => toast.error('Failed to dismiss job'),
  })

  const bulkDismiss = useMutation({
    mutationFn: (ids: string[]) => bulkDismissJobs(ids),
    onSuccess: (result) => {
      toast.success(`${result.dismissed} job${result.dismissed !== 1 ? 's' : ''} dismissed`)
      invalidateJobs()
    },
    onError: () => toast.error('Failed to dismiss jobs'),
  })

  // ── Wave 2 stubs ─────────────────────────────────────────────────────────
  // The following are placeholders so JobsPage's keyboard handler can wire
  // up the bulk-save / bulk-applied / undo callbacks today. Wave 2 Agent G
  // replaces these with real mutations (bulk save -> PATCH per id or a new
  // /api/jobs/bulk-status endpoint; undo -> revert the most recent status
  // change with the prior status persisted in a local action history).
  //
  // TODO(Wave 2 Agent G): swap these stubs for real mutations.
  const bulkSave = {
    mutate: (ids: string[]) => {
      console.warn('bulkSave stub — Wave 2 Agent G', ids.length, 'jobs')
    },
    isPending: false,
  }

  const bulkApplied = {
    mutate: (ids: string[]) => {
      console.warn('bulkApplied stub — Wave 2 Agent G', ids.length, 'jobs')
    },
    isPending: false,
  }

  const undoLast = () => {
    console.warn('undoLast stub — Wave 2 Agent G')
  }

  return { setStatus, dismiss, bulkDismiss, bulkSave, bulkApplied, undoLast }
}
